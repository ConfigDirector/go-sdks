package configdirector

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Options configures a Client. Every field except SDKKey is optional: a zero value stands for
// the default described on the field.
type Options struct {
	// SDKKey is your ConfigDirector server SDK key. It is a secret: read it from the environment
	// rather than committing it to source control.
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
)

// ConnectionOptions controls how a client connects to ConfigDirector.
type ConnectionOptions struct {
	// Mode is the connection mode. Defaults to ConnectionModeStreaming.
	Mode ConnectionMode

	// PollingInterval is how long the client waits between polls in polling mode. Defaults to 5
	// minutes. The minimum is 1 minute: a shorter interval is raised to the minimum and a warning
	// is logged. Ignored in streaming mode.
	PollingInterval time.Duration

	// Timeout bounds how long Client.WaitForReady waits for the first config state. Defaults to
	// 3 seconds. The client keeps connecting after the timeout elapses.
	Timeout time.Duration

	// URL is the base URL of the ConfigDirector SDK server. Only needed when routing through a
	// proxy. Defaults to https://server-sdk-api.configdirector.com.
	URL string

	// HTTPClient makes every request the client sends. Defaults to http.DefaultClient.
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
	sdkKey          string
	metadata        Metadata
	mode            ConnectionMode
	pollingInterval time.Duration
	timeout         time.Duration
	baseURL         string
	httpClient      *http.Client
	eventQueueLimit int
	flushInterval   time.Duration
	logger          *slog.Logger
}

func settingsFrom(options Options) (clientSettings, error) {
	if strings.TrimSpace(options.SDKKey) == "" {
		return clientSettings{}, errors.New("configdirector: the SDK key is blank; a client needs a server SDK key")
	}

	mode, err := connectionModeFrom(options.Connection.Mode)
	if err != nil {
		return clientSettings{}, err
	}
	if options.Connection.Timeout < 0 {
		return clientSettings{}, fmt.Errorf("configdirector: the connection timeout %s is negative; it must be a positive duration", options.Connection.Timeout)
	}
	baseURL, err := baseURLFrom(options.Connection.URL)
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

	pollingInterval := pollingIntervalFor(mode, options.Connection.PollingInterval)
	if pollingInterval.raisedToMinimum {
		logger.Warn(
			"The polling interval is below the minimum, the minimum is used instead",
			slog.Duration("configured", options.Connection.PollingInterval),
			slog.Duration("used", pollingInterval.interval),
		)
	}

	return clientSettings{
		sdkKey:          options.SDKKey,
		metadata:        options.Metadata,
		mode:            mode,
		pollingInterval: pollingInterval.interval,
		timeout:         durationOrDefault(options.Connection.Timeout, defaultTimeout),
		baseURL:         baseURL,
		httpClient:      httpClientOrDefault(options.Connection.HTTPClient),
		eventQueueLimit: eventQueueLimit,
		flushInterval:   durationOrDefault(options.Telemetry.FlushInterval, defaultFlushInterval),
		logger:          logger,
	}, nil
}

func connectionModeFrom(configured ConnectionMode) (ConnectionMode, error) {
	switch configured {
	case "", ConnectionModeStreaming:
		return ConnectionModeStreaming, nil
	case ConnectionModePolling:
		return ConnectionModePolling, nil
	default:
		return "", fmt.Errorf("configdirector: unknown connection mode %q; use ConnectionModeStreaming or ConnectionModePolling", configured)
	}
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
