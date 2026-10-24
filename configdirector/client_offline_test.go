package configdirector_test

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/stretchr/testify/require"
)

func offlineValues(values map[string]any) configdirector.Options {
	return configdirector.Options{Connection: configdirector.ConnectionOptions{Mode: configdirector.ConnectionModeOffline, Values: values}}
}

func TestOfflineModeServesInlineValuesTypedByTheGoValueToEveryContext(t *testing.T) {
	t.Parallel()

	client := newClient(t, offlineValues(map[string]any{
		"new-checkout": true,
		"max-items":    20,
		"ratio":        0.5,
		"greeting":     "hello",
		"theme":        map[string]any{"color": "blue"},
	}))

	require.True(t, client.BoolValue("new-checkout", false, noUser))
	require.Equal(t, int64(20), client.IntValue("max-items", 1, configdirector.UserContext{ID: "u-pro", Kind: "device"}))
	require.InDelta(t, 0.5, client.FloatValue("ratio", 0, noUser), 0)
	require.Equal(t, "hello", client.StringValue("greeting", "fallback", noUser))
	var decoded struct {
		Color string `json:"color"`
	}
	require.NoError(t, client.JSONValue("theme", &decoded, noUser))
	require.Equal(t, "blue", decoded.Color)
	require.Equal(t, []configdirector.ConfigState{
		{ID: "offline:max-items", Key: "max-items", Type: configdirector.ConfigTypeInteger, Value: "20", ValueID: ""},
		{ID: "offline:ratio", Key: "ratio", Type: configdirector.ConfigTypeFloat, Value: "0.5", ValueID: ""},
	}, client.AllConfigs(noUser, "max-items", "ratio"))
}

func TestAnOfflineValueEvaluatesAsAFoundMatchWithoutAValueID(t *testing.T) {
	t.Parallel()
	client := newClient(t, offlineValues(map[string]any{"new-checkout": true}))

	value, details := client.BoolValueDetails("new-checkout", false, noUser)

	require.True(t, value)
	require.Equal(t, foundMatch(""), details)
}

func offlineFile(t *testing.T, document string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "flags.json")
	require.NoError(t, os.WriteFile(path, []byte(document), 0o600))
	return path
}

func offlineFromFile(path string) configdirector.Options {
	return configdirector.Options{Connection: configdirector.ConnectionOptions{Mode: configdirector.ConnectionModeOffline, File: path}}
}

func TestOfflineModeServesTypedValuesFromAJSONFile(t *testing.T) {
	t.Parallel()
	path := offlineFile(t, `{
		"new-checkout": true,
		"max-items": 20,
		"ratio": 0.5,
		"greeting": "hello",
		"theme": {"color": "blue"},
		"regions": ["eu", "us"]
	}`)

	client := newClient(t, offlineFromFile(path))

	require.True(t, client.BoolValue("new-checkout", false, noUser))
	require.Equal(t, int64(20), client.IntValue("max-items", 1, configdirector.UserContext{ID: "u-pro"}))
	require.InDelta(t, 0.5, client.FloatValue("ratio", 0, noUser), 0)
	require.Equal(t, "hello", client.StringValue("greeting", "fallback", noUser))
	var regions []string
	require.NoError(t, client.JSONValue("regions", &regions, noUser))
	require.Equal(t, []string{"eu", "us"}, regions)
	require.Equal(t, []configdirector.ConfigState{
		{ID: "offline:max-items", Key: "max-items", Type: configdirector.ConfigTypeInteger, Value: "20", ValueID: ""},
		{ID: "offline:ratio", Key: "ratio", Type: configdirector.ConfigTypeFloat, Value: "0.5", ValueID: ""},
		{ID: "offline:theme", Key: "theme", Type: configdirector.ConfigTypeJSON, Value: `{"color":"blue"}`, ValueID: ""},
	}, client.AllConfigs(noUser, "max-items", "ratio", "theme"))
}

func TestAWholeNumberWithADecimalPointInTheFileIsAnIntegerConfig(t *testing.T) {
	t.Parallel()
	client := newClient(t, offlineFromFile(offlineFile(t, `{"max-items": 20.0}`)))

	states := client.AllConfigs(noUser, "max-items")

	require.Equal(t, configdirector.ConfigTypeInteger, states[0].Type)
	require.Equal(t, "20", states[0].Value)
}

func TestAWholeNumberBeyondInt64InTheFileIsAFloatConfig(t *testing.T) {
	t.Parallel()
	client := newClient(t, offlineFromFile(offlineFile(t, `{"population": 1e20}`)))

	states := client.AllConfigs(noUser, "population")

	require.Equal(t, configdirector.ConfigTypeFloat, states[0].Type)
	require.Equal(t, "100000000000000000000", states[0].Value)
}

func TestNewClientRejectsAMissingOfflineFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "missing.json")

	client, err := configdirector.NewClient(offlineFromFile(path))

	require.ErrorContains(t, err, path)
	require.ErrorIs(t, err, fs.ErrNotExist)
	require.Nil(t, client)
}

func TestNewClientRejectsAnOfflineFileItCannotRead(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()

	client, err := configdirector.NewClient(offlineFromFile(directory))

	require.ErrorContains(t, err, directory)
	require.Nil(t, client)
}

func TestNewClientRejectsAnOfflineFileThatIsNotAJSONObject(t *testing.T) {
	t.Parallel()
	path := offlineFile(t, `["new-checkout"]`)

	client, err := configdirector.NewClient(offlineFromFile(path))

	require.ErrorContains(t, err, path)
	require.Nil(t, client)
}

func TestNewClientRejectsANullValueInTheOfflineFile(t *testing.T) {
	t.Parallel()
	path := offlineFile(t, `{"new-checkout": true, "max-items": null}`)

	client, err := configdirector.NewClient(offlineFromFile(path))

	require.ErrorContains(t, err, path)
	require.ErrorContains(t, err, `"max-items"`)
	require.Nil(t, client)
}

func TestNewClientRejectsANilInlineOfflineValue(t *testing.T) {
	t.Parallel()

	client, err := configdirector.NewClient(offlineValues(map[string]any{"new-checkout": true, "max-items": nil}))

	require.ErrorContains(t, err, `"max-items"`)
	require.Nil(t, client)
}

func TestNewClientRejectsAnInlineOfflineValueOfAnUnservedKind(t *testing.T) {
	t.Parallel()

	client, err := configdirector.NewClient(offlineValues(map[string]any{"updates": make(chan int)}))

	require.ErrorContains(t, err, `"updates"`)
	require.ErrorContains(t, err, "chan int")
	require.Nil(t, client)
}

func TestNewClientRejectsAnOfflineFileAndValuesTogether(t *testing.T) {
	t.Parallel()
	options := offlineFromFile(offlineFile(t, `{"new-checkout": true}`))
	options.Connection.Values = map[string]any{"max-items": 20}

	client, err := configdirector.NewClient(options)

	require.Error(t, err)
	require.Nil(t, client)
}

func TestNewClientRejectsAnOfflineFileInStreamingMode(t *testing.T) {
	t.Parallel()
	path := offlineFile(t, `{"new-checkout": true}`)

	client, err := configdirector.NewClient(configdirector.Options{
		SDKKey:     testSDKKey,
		Connection: configdirector.ConnectionOptions{File: path},
	})

	require.Error(t, err)
	require.Nil(t, client)
}

func TestNewClientRejectsOfflineValuesInPollingMode(t *testing.T) {
	t.Parallel()

	client, err := configdirector.NewClient(configdirector.Options{
		SDKKey:     testSDKKey,
		Connection: configdirector.ConnectionOptions{Mode: configdirector.ConnectionModePolling, Values: map[string]any{"new-checkout": true}},
	})

	require.Error(t, err)
	require.Nil(t, client)
}

func TestOfflineModeWithNoSourceIsReadyAndServesInCodeDefaultsAsConfigStateMissing(t *testing.T) {
	t.Parallel()

	client := newClient(t, configdirector.Options{Connection: configdirector.ConnectionOptions{Mode: configdirector.ConnectionModeOffline}})
	value, details := client.BoolValueDetails("new-checkout", true, noUser)

	require.True(t, client.Ready())
	require.True(t, value)
	require.Equal(t, usedDefault(configdirector.EvaluationReasonConfigStateMissing), details)
	require.Empty(t, client.AllConfigs(noUser))
}

func TestAnOfflineClientIsReadyWhenNewClientReturnsAndFiresTheReadyEventOnce(t *testing.T) {
	t.Parallel()
	readyEvents := 0
	updates := []configdirector.ConfigsUpdated{}
	options := offlineValues(map[string]any{"new-checkout": true})
	options.Hooks = configdirector.Hooks{
		OnReady:          func() { readyEvents++ },
		OnConfigsUpdated: func(update configdirector.ConfigsUpdated) { updates = append(updates, update) },
	}

	client := newClient(t, options)

	require.True(t, client.Ready())
	require.Equal(t, 1, readyEvents)
	require.Equal(t, []configdirector.ConfigsUpdated{{Keys: []string{"new-checkout"}, RemovedKeys: []string{}}}, updates)
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	require.NoError(t, client.WaitForReady(ctx))
	require.Equal(t, 1, readyEvents)
}

func TestAWatchOnAnOfflineClientNeverFires(t *testing.T) {
	t.Parallel()
	client := newClient(t, offlineValues(map[string]any{"new-checkout": true}))
	fired := []bool{}

	client.WatchBool("new-checkout", false, noUser, func(value bool) { fired = append(fired, value) })
	served := client.BoolValue("new-checkout", false, noUser)
	require.NoError(t, client.Close())

	require.True(t, served)
	require.Empty(t, fired)
}

func TestAnOfflineValueOfAnotherTypeServesTheInCodeDefaultValue(t *testing.T) {
	t.Parallel()
	client := newClient(t, offlineValues(map[string]any{"new-checkout": true}))

	value, details := client.StringValueDetails("new-checkout", "fallback", noUser)

	require.Equal(t, "fallback", value)
	require.Equal(t, usedDefault(configdirector.EvaluationReasonTypeMismatch), details)
}

func TestAnOfflineClientOpensNoConnectionAndSendsNoTelemetry(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	transport := &countingTransport{next: server.Client().Transport}
	options := offlineValues(map[string]any{"new-checkout": true})
	options.SDKKey = testSDKKey
	options.Connection.URL = server.URL
	options.Connection.HTTPClient = &http.Client{Transport: transport}

	client := newClient(t, options)
	for range 10 {
		client.BoolValue("new-checkout", false, configdirector.UserContext{ID: "u-1"})
	}
	require.NoError(t, client.Close())

	require.Zero(t, transport.requests.Load())
	require.Empty(t, server.requests)
}

func TestAnOfflineClientStartsNoGoroutine(t *testing.T) {
	before := sdkGoroutines()

	client := newClient(t, offlineValues(map[string]any{"new-checkout": true}))
	require.True(t, client.BoolValue("new-checkout", false, noUser))

	require.Equal(t, before, sdkGoroutines())
	require.NoError(t, client.Close())
	require.Equal(t, before, sdkGoroutines())
}

func TestNewClientAcceptsAnAbsentSDKKeyInOfflineMode(t *testing.T) {
	t.Parallel()

	client, err := configdirector.NewClient(configdirector.Options{Connection: configdirector.ConnectionOptions{Mode: configdirector.ConnectionModeOffline}})

	require.NoError(t, err)
	require.NoError(t, client.Close())
}

func TestNewClientStillRejectsAnAbsentSDKKeyInPollingMode(t *testing.T) {
	t.Parallel()

	client, err := configdirector.NewClient(configdirector.Options{Connection: configdirector.ConnectionOptions{Mode: configdirector.ConnectionModePolling}})

	require.Error(t, err)
	require.Nil(t, client)
}

func TestOfflineModeIgnoresThePollingIntervalTimeoutAndURLWithoutAnError(t *testing.T) {
	t.Parallel()
	logger, records := newLogger()
	options := offlineValues(map[string]any{"new-checkout": true})
	options.Connection.PollingInterval = time.Second
	options.Connection.Timeout = -time.Second
	options.Connection.URL = "not a url"
	options.Logger = logger

	client := newClient(t, options)

	require.True(t, client.BoolValue("new-checkout", false, noUser))
	require.Empty(t, recordsAt(slog.LevelWarn, records()))
}

func TestNewClientRejectsAnOfflineFileThatIsNull(t *testing.T) {
	t.Parallel()
	path := offlineFile(t, `null`)

	client, err := configdirector.NewClient(offlineFromFile(path))

	require.ErrorContains(t, err, path)
	require.Nil(t, client)
}

func TestNewClientRejectsAnOfflineFileThatContinuesAfterTheObject(t *testing.T) {
	t.Parallel()
	path := offlineFile(t, `{"new-checkout": true} {"max-items": 20}`)

	client, err := configdirector.NewClient(offlineFromFile(path))

	require.ErrorContains(t, err, path)
	require.Nil(t, client)
}
