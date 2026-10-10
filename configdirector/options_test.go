package configdirector_test

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/stretchr/testify/require"
)

type recordingHandler struct {
	records *[]slog.Record
	mutex   *sync.Mutex
	attrs   []slog.Attr
}

func newLogger() (*slog.Logger, func() []slog.Record) {
	handler := &recordingHandler{records: &[]slog.Record{}, mutex: &sync.Mutex{}}
	return slog.New(handler), func() []slog.Record {
		handler.mutex.Lock()
		defer handler.mutex.Unlock()
		return append([]slog.Record{}, *handler.records...)
	}
}

func (*recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (handler *recordingHandler) Handle(_ context.Context, record slog.Record) error {
	handler.mutex.Lock()
	defer handler.mutex.Unlock()
	record.AddAttrs(handler.attrs...)
	*handler.records = append(*handler.records, record)
	return nil
}

func (handler *recordingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	inherited := slices.Concat(handler.attrs, attrs)
	return &recordingHandler{records: handler.records, mutex: handler.mutex, attrs: inherited}
}

func (handler *recordingHandler) WithGroup(string) slog.Handler { return handler }

func recordsAt(level slog.Level, records []slog.Record) []slog.Record {
	var matching []slog.Record
	for _, record := range records {
		if record.Level == level {
			matching = append(matching, record)
		}
	}
	return matching
}

func attributesOf(record slog.Record) map[string]any {
	attributes := map[string]any{}
	record.Attrs(func(attr slog.Attr) bool {
		attributes[attr.Key] = attr.Value.Any()
		return true
	})
	return attributes
}

func pollingEvery(server *sdkServer, interval time.Duration, logger *slog.Logger) configdirector.Options {
	options := server.options()
	options.Connection.Mode = configdirector.ConnectionModePolling
	options.Connection.PollingInterval = interval
	options.Logger = logger
	return options
}

func TestNewClientAcceptsEveryOption(t *testing.T) {
	t.Parallel()
	logger, records := newLogger()
	server := startSDKServer(t, standardPayload().json())

	client := newClient(t, configdirector.Options{
		SDKKey:   testSDKKey,
		Metadata: configdirector.Metadata{AppName: "my-app", AppVersion: "1.2.3"},
		Connection: configdirector.ConnectionOptions{
			Mode:            configdirector.ConnectionModePolling,
			PollingInterval: 90 * time.Second,
			Timeout:         5 * time.Second,
			URL:             server.URL,
			HTTPClient:      &http.Client{},
		},
		Telemetry: configdirector.TelemetryOptions{EventQueueLimit: 100, FlushInterval: 10 * time.Second},
		Logger:    logger,
		Hooks:     configdirector.Hooks{},
	})

	waitForReady(t, client)
	require.Empty(t, recordsAt(slog.LevelWarn, records()))
}

func TestNewClientRejectsABlankSDKKey(t *testing.T) {
	t.Parallel()

	client, err := configdirector.NewClient(configdirector.Options{SDKKey: ""})

	require.Error(t, err)
	require.Nil(t, client)
}

func TestNewClientRejectsAWhitespaceSDKKey(t *testing.T) {
	t.Parallel()

	client, err := configdirector.NewClient(configdirector.Options{SDKKey: "   "})

	require.Error(t, err)
	require.Nil(t, client)
}

func TestNewClientRejectsAURLWithoutAScheme(t *testing.T) {
	t.Parallel()

	client, err := configdirector.NewClient(configdirector.Options{
		SDKKey:     testSDKKey,
		Connection: configdirector.ConnectionOptions{URL: "proxy.example.com/sdk"},
	})

	require.Error(t, err)
	require.Nil(t, client)
}

func TestNewClientRejectsAURLWithoutAHost(t *testing.T) {
	t.Parallel()

	client, err := configdirector.NewClient(configdirector.Options{
		SDKKey:     testSDKKey,
		Connection: configdirector.ConnectionOptions{URL: "https://"},
	})

	require.Error(t, err)
	require.Nil(t, client)
}

func TestNewClientRejectsAnUnknownConnectionMode(t *testing.T) {
	t.Parallel()

	client, err := configdirector.NewClient(configdirector.Options{
		SDKKey:     testSDKKey,
		Connection: configdirector.ConnectionOptions{Mode: "websocket"},
	})

	require.Error(t, err)
	require.Nil(t, client)
}

func TestNewClientRejectsANegativeTimeout(t *testing.T) {
	t.Parallel()

	client, err := configdirector.NewClient(configdirector.Options{
		SDKKey:     testSDKKey,
		Connection: configdirector.ConnectionOptions{Timeout: -time.Second},
	})

	require.Error(t, err)
	require.Nil(t, client)
}

func TestNewClientRejectsAnEventQueueLimitBelowTheMinimum(t *testing.T) {
	t.Parallel()

	client, err := configdirector.NewClient(configdirector.Options{
		SDKKey:    testSDKKey,
		Telemetry: configdirector.TelemetryOptions{EventQueueLimit: 99},
	})

	require.Error(t, err)
	require.Nil(t, client)
}

func TestNewClientRejectsAnEventQueueLimitAboveTheMaximum(t *testing.T) {
	t.Parallel()

	client, err := configdirector.NewClient(configdirector.Options{
		SDKKey:    testSDKKey,
		Telemetry: configdirector.TelemetryOptions{EventQueueLimit: 100_001},
	})

	require.Error(t, err)
	require.Nil(t, client)
}

func TestNewClientRejectsANegativeFlushInterval(t *testing.T) {
	t.Parallel()

	client, err := configdirector.NewClient(configdirector.Options{
		SDKKey:    testSDKKey,
		Telemetry: configdirector.TelemetryOptions{FlushInterval: -time.Second},
	})

	require.Error(t, err)
	require.Nil(t, client)
}

func TestNewClientRaisesAPollingIntervalBelowTheMinimumWithOneWarning(t *testing.T) {
	t.Parallel()
	logger, records := newLogger()

	newClient(t, pollingEvery(startSDKServer(t), 30*time.Second, logger))

	warnings := recordsAt(slog.LevelWarn, records())
	require.Len(t, warnings, 1)
	require.Equal(t, 30*time.Second, attributesOf(warnings[0])["configured"])
	require.Equal(t, time.Minute, attributesOf(warnings[0])["used"])
}

func TestNewClientTagsEveryLogRecordWithTheComponent(t *testing.T) {
	t.Parallel()
	logger, records := newLogger()

	newClient(t, pollingEvery(startSDKServer(t), 30*time.Second, logger))

	require.Equal(t, "configdirector", attributesOf(recordsAt(slog.LevelWarn, records())[0])["component"])
}

func TestNewClientKeepsAPollingIntervalAtTheMinimumWithoutAWarning(t *testing.T) {
	t.Parallel()
	logger, records := newLogger()

	newClient(t, pollingEvery(startSDKServer(t), time.Minute, logger))

	require.Empty(t, recordsAt(slog.LevelWarn, records()))
}

func TestNewClientIgnoresAShortPollingIntervalWhenStreaming(t *testing.T) {
	t.Parallel()
	logger, records := newLogger()

	options := startSDKServer(t).options()
	options.Connection.PollingInterval = 30 * time.Second
	options.Logger = logger
	newClient(t, options)

	require.Empty(t, recordsAt(slog.LevelWarn, records()))
}

func TestAWaitThatEndsBeforeTheFirstConfigStateIsLoggedAsAWarning(t *testing.T) {
	t.Parallel()
	logger, records := newLogger()
	options := startSDKServer(t).options()
	options.Logger = logger
	client := newClient(t, options)
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	require.Error(t, client.WaitForReady(ctx))

	require.Len(t, recordsAt(slog.LevelWarn, records()), 1)
}
