package configdirector

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector/internal/identityseam"
	"github.com/ConfigDirector/go-sdks/configdirector/internal/servedvalues"
)

// Options configures a Client. Every field is optional, a zero value standing for the default
// described on the field, except SDKKey, which is required unless Connection.Mode is
// ConnectionModeOffline.
type Options struct {
	// SDKKey is your ConfigDirector server SDK key. It is a secret: read it from the environment
	// rather than committing it to source control. Not needed in ConnectionModeOffline.
	SDKKey string

	// Metadata describes the calling application. Supplying it lets targeting rules reference the
	// application's name and version.
	Metadata Metadata

	// Connection controls how the client reaches ConfigDirector.
	Connection ConnectionOptions

	// Telemetry tunes what the client reports back about the configs it evaluated.
	Telemetry TelemetryOptions

	// Logger receives the client's log records. Defaults to slog.Default(). Every record carries
	// the attribute component=configdirector.
	Logger *slog.Logger

	// Hooks holds event handlers attached before the client can emit any event.
	Hooks Hooks
}

// Metadata describes the application a client runs in. Targeting rules can match on both
// fields.
type Metadata struct {
	// AppName is the application's name.
	AppName string

	// AppVersion is the version of the application that is running.
	AppVersion string
}

// ConnectionMode is how the client retrieves config state from ConfigDirector.
type ConnectionMode string

const (
	// ConnectionModeStreaming keeps a connection open and receives updates whenever config state
	// changes in the ConfigDirector dashboard. It is the default.
	ConnectionModeStreaming ConnectionMode = "streaming"

	// ConnectionModePolling retrieves config state once when connecting and then again every
	// polling interval.
	ConnectionModePolling ConnectionMode = "polling"

	// ConnectionModeOffline never connects to ConfigDirector: the client serves the values in
	// ConnectionOptions.File or ConnectionOptions.Values to every context, is ready as soon as
	// NewClient returns, and sends no telemetry, so the application is invisible to the
	// ConfigDirector dashboard. With neither File nor Values, every read returns its in-code
	// default value. No SDK key is needed.
	ConnectionModeOffline ConnectionMode = "offline"
)

// ConnectionOptions controls how a client connects to ConfigDirector.
type ConnectionOptions struct {
	// Mode is the connection mode. Defaults to ConnectionModeStreaming.
	Mode ConnectionMode

	// File is the path of a JSON file holding the values to serve in ConnectionModeOffline,
	// resolved against the working directory: one JSON object with one value per config key.
	// Each value's config type follows from the value: true or false is a boolean, a number
	// without a fractional part is an integer, any other number is a float, a string is a string,
	// and an object or an array is a JSON config. NewClient returns an error when the file is
	// missing, cannot be read, is not a JSON object or holds a null, when Values is given too,
	// or when File is given in another mode.
	File string

	// Values holds the values to serve in ConnectionModeOffline, keyed by config key, typed by
	// the Go value exactly as configdirectortest.TestClient.SetValue types them: a bool, a
	// string, any integer kind, a float32 or float64, or a map, slice, array or struct that
	// encodes as JSON. NewClient returns an error for a nil value, a value of any other kind,
	// when File is given too, or when Values is given in another mode.
	Values map[string]any

	// PollingInterval is how long the client waits between polls in polling mode. Defaults to 5
	// minutes. The minimum is 1 minute: a shorter interval is raised to the minimum and a warning
	// is logged. Ignored in every other mode.
	PollingInterval time.Duration

	// Timeout bounds how long Client.WaitForReady waits for the first config state. Defaults to
	// 3 seconds. The client keeps connecting after the timeout elapses. Ignored in
	// ConnectionModeOffline.
	Timeout time.Duration

	// URL is the base URL of the ConfigDirector SDK server. Only needed when routing through a
	// proxy. Defaults to https://server-sdk-api.configdirector.com. Ignored in
	// ConnectionModeOffline.
	URL string

	// HTTPClient makes every request the client sends. Defaults to http.DefaultClient. Ignored
	// in ConnectionModeOffline.
	HTTPClient *http.Client
}

// TelemetryOptions tunes the telemetry a client reports. It is unlikely these settings need
// adjusting; they let an application that evaluates configs at a very high rate trade memory
// against how often telemetry requests are sent. ConfigDirector relies on telemetry to power
// insights about the configs in use.
type TelemetryOptions struct {
	// EventQueueLimit is the size limit of the telemetry event queue. When it is reached before
	// events are flushed, older events are dropped and ConfigDirector counts the drops. Between
	// 100 and 100,000; defaults to 5,000.
	EventQueueLimit int

	// FlushInterval is how often queued events are sent. Defaults to 30 seconds.
	FlushInterval time.Duration
}

const (
	defaultURL               = "https://server-sdk-api.configdirector.com"
	defaultPollingInterval   = 5 * time.Minute
	minimumPollingInterval   = time.Minute
	defaultTimeout           = 3 * time.Second
	defaultEventQueueLimit   = 5_000
	minimumEventQueueLimit   = 100
	maximumEventQueueLimit   = 100_000
	defaultFlushInterval     = 30 * time.Second
	loggerComponentAttribute = "configdirector"
)

type clientSettings struct {
	identity        identityseam.Identity
	sdkKey          string
	metadata        Metadata
	mode            ConnectionMode
	offlineConfigs  map[string]servedvalues.Config
	pollingInterval time.Duration
	timeout         time.Duration
	baseURL         string
	httpClient      *http.Client
	eventQueueLimit int
	flushInterval   time.Duration
	logger          *slog.Logger
}

func settingsFrom(options Options) (clientSettings, error) {
	mode, err := connectionModeFrom(options.Connection.Mode)
	if err != nil {
		return clientSettings{}, err
	}
	eventQueueLimit, err := eventQueueLimitFrom(options.Telemetry.EventQueueLimit)
	if err != nil {
		return clientSettings{}, err
	}
	if options.Telemetry.FlushInterval < 0 {
		return clientSettings{}, fmt.Errorf("configdirector: the telemetry flush interval %s is negative; it must be a positive duration", options.Telemetry.FlushInterval)
	}

	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With(slog.String("component", loggerComponentAttribute))

	settings := clientSettings{
		identity:        ownIdentity,
		metadata:        options.Metadata,
		mode:            mode,
		eventQueueLimit: eventQueueLimit,
		flushInterval:   durationOrDefault(options.Telemetry.FlushInterval, defaultFlushInterval),
		logger:          logger,
	}
	switch mode.configStateSource() {
	case configStateFromConfigDirector:
		return settings.connectingToConfigDirectorWith(options)
	case configStateFromOfflineValues:
		return settings.servingOfflineValuesFrom(options.Connection)
	}
	panic(fmt.Sprintf("configdirector: the connection mode %q passed validation without a config state source", mode))
}

func (settings clientSettings) connectingToConfigDirectorWith(options Options) (clientSettings, error) {
	if strings.TrimSpace(options.SDKKey) == "" {
		return clientSettings{}, errors.New("configdirector: the SDK key is blank; a client needs a server SDK key")
	}
	switch offlineSourceOf(options.Connection) {
	case offlineSourceNone:
	case offlineSourceFile, offlineSourceInlineValues, offlineSourceFileAndValuesTogether:
		return clientSettings{}, fmt.Errorf("configdirector: the connection mode is %q but File or Values is given; offline values are served in ConnectionModeOffline only", settings.mode)
	}
	if options.Connection.Timeout < 0 {
		return clientSettings{}, fmt.Errorf("configdirector: the connection timeout %s is negative; it must be a positive duration", options.Connection.Timeout)
	}
	baseURL, err := baseURLFrom(options.Connection.URL)
	if err != nil {
		return clientSettings{}, err
	}

	pollingInterval := pollingIntervalFor(settings.mode, options.Connection.PollingInterval)
	if pollingInterval.raisedToMinimum {
		settings.logger.Warn(
			"The polling interval is below the minimum, the minimum is used instead",
			slog.Duration("configured", options.Connection.PollingInterval),
			slog.Duration("used", pollingInterval.interval),
		)
	}

	settings.sdkKey = options.SDKKey
	settings.pollingInterval = pollingInterval.interval
	settings.timeout = durationOrDefault(options.Connection.Timeout, defaultTimeout)
	settings.baseURL = baseURL
	settings.httpClient = httpClientOrDefault(options.Connection.HTTPClient)
	return settings, nil
}

func (settings clientSettings) servingOfflineValuesFrom(connection ConnectionOptions) (clientSettings, error) {
	configs, err := offlineConfigsFrom(connection)
	if err != nil {
		return clientSettings{}, err
	}
	settings.offlineConfigs = configs
	return settings, nil
}

func connectionModeFrom(configured ConnectionMode) (ConnectionMode, error) {
	switch configured {
	case "", ConnectionModeStreaming:
		return ConnectionModeStreaming, nil
	case ConnectionModePolling:
		return ConnectionModePolling, nil
	case ConnectionModeOffline:
		return ConnectionModeOffline, nil
	default:
		return "", fmt.Errorf("configdirector: unknown connection mode %q; use ConnectionModeStreaming, ConnectionModePolling or ConnectionModeOffline", configured)
	}
}

type configStateSource int

const (
	configStateFromConfigDirector configStateSource = iota
	configStateFromOfflineValues
)

func (mode ConnectionMode) configStateSource() configStateSource {
	switch mode {
	case ConnectionModeStreaming, ConnectionModePolling:
		return configStateFromConfigDirector
	case ConnectionModeOffline:
		return configStateFromOfflineValues
	}
	panic(fmt.Sprintf("configdirector: the connection mode %q passed validation without a config state source", mode))
}

type pollingIntervalDecision struct {
	interval        time.Duration
	raisedToMinimum bool
}

func pollingIntervalFor(mode ConnectionMode, configured time.Duration) pollingIntervalDecision {
	if configured == 0 {
		return pollingIntervalDecision{interval: defaultPollingInterval}
	}
	if mode == ConnectionModePolling && configured < minimumPollingInterval {
		return pollingIntervalDecision{interval: minimumPollingInterval, raisedToMinimum: true}
	}
	return pollingIntervalDecision{interval: configured}
}

func baseURLFrom(configured string) (string, error) {
	if configured == "" {
		return defaultURL, nil
	}
	parsed, err := url.Parse(configured)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("configdirector: the connection URL %q is not usable; it must be absolute, with a scheme and a host", configured)
	}
	return configured, nil
}

func eventQueueLimitFrom(configured int) (int, error) {
	if configured == 0 {
		return defaultEventQueueLimit, nil
	}
	if configured < minimumEventQueueLimit || configured > maximumEventQueueLimit {
		return 0, fmt.Errorf("configdirector: the telemetry event queue limit %d is out of range; it must be between %d and %d", configured, minimumEventQueueLimit, maximumEventQueueLimit)
	}
	return configured, nil
}

func durationOrDefault(configured time.Duration, fallback time.Duration) time.Duration {
	if configured == 0 {
		return fallback
	}
	return configured
}

func httpClientOrDefault(configured *http.Client) *http.Client {
	if configured == nil {
		return http.DefaultClient
	}
	return configured
}
