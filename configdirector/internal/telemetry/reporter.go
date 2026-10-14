package telemetry

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector/internal/transport"
)

const (
	telemetryPath  = "server/telemetry/v1"
	requestTimeout = 5 * time.Second
)

type eventReport struct {
	startTime          time.Time
	endTime            time.Time
	evaluations        []aggregatedEvent
	droppedEvaluations int
	contexts           []Context
	droppedContexts    int
	invalidContexts    invalidContextBatch
}

func (report eventReport) isEmpty() bool {
	return len(report.evaluations) == 0 && len(report.contexts) == 0 && report.droppedEvaluations == 0 && report.droppedContexts == 0 && len(report.invalidContexts.entries) == 0
}

func (report eventReport) wire(serverSDKKey string, metaContext transport.MetaContext) wireReport {
	wire := wireReport{
		ServerSDKKey:   serverSDKKey,
		MetaContext:    metaContext,
		DiscreteEvents: wireDiscreteEvents{CapturedContexts: make([]wireContext, 0, len(report.contexts))},
		AggregatedEvents: wireAggregatedEvents{
			EvaluatedConfig: make([]wireAggregatedEvent, 0, len(report.evaluations)),
			InvalidContext:  make([]wireAggregatedInvalidContext, 0, len(report.invalidContexts.entries)),
		},
		DroppedEvents: wireDroppedEvents{EvaluatedConfig: report.droppedEvaluations, CapturedContexts: report.droppedContexts},
	}
	for _, captured := range report.contexts {
		wire.DiscreteEvents.CapturedContexts = append(wire.DiscreteEvents.CapturedContexts, captured.wire())
	}
	for _, aggregated := range report.evaluations {
		wire.AggregatedEvents.EvaluatedConfig = append(wire.AggregatedEvents.EvaluatedConfig, wireAggregatedEvent{
			StartTime: wireTime(report.startTime),
			EndTime:   wireTime(report.endTime),
			Count:     aggregated.count,
			Event:     aggregated.event.wire(),
		})
	}
	for _, entry := range report.invalidContexts.entries {
		wire.AggregatedEvents.InvalidContext = append(wire.AggregatedEvents.InvalidContext, wireAggregatedInvalidContext{
			StartTime: wireTime(report.invalidContexts.startTime),
			EndTime:   wireTime(report.invalidContexts.endTime),
			Count:     entry.count,
			Event:     entry.wire(),
		})
	}
	return wire
}

type wireReport struct {
	ServerSDKKey     string                `json:"serverSdkKey"`
	MetaContext      transport.MetaContext `json:"metaContext"`
	DiscreteEvents   wireDiscreteEvents    `json:"discreteEvents"`
	AggregatedEvents wireAggregatedEvents  `json:"aggregatedEvents"`
	DroppedEvents    wireDroppedEvents     `json:"droppedEvents"`
}

type wireDiscreteEvents struct {
	CapturedContexts []wireContext `json:"capturedContexts"`
}

type wireAggregatedEvents struct {
	EvaluatedConfig []wireAggregatedEvent          `json:"evaluatedConfig"`
	InvalidContext  []wireAggregatedInvalidContext `json:"invalidContext"`
}

type wireDroppedEvents struct {
	EvaluatedConfig  int `json:"evaluatedConfig"`
	CapturedContexts int `json:"capturedContexts"`
}

type wireAggregatedEvent struct {
	StartTime wireTime                 `json:"startTime"`
	EndTime   wireTime                 `json:"endTime"`
	Count     int                      `json:"count"`
	Event     wireEvaluatedConfigEvent `json:"event"`
}

type wireAggregatedInvalidContext struct {
	StartTime wireTime                `json:"startTime"`
	EndTime   wireTime                `json:"endTime"`
	Count     int                     `json:"count"`
	Event     wireInvalidContextEvent `json:"event"`
}

type wireTime time.Time

// MarshalJSON writes the moment as RFC 3339 in UTC with milliseconds and a trailing Z, the
// spelling the other SDKs send and the SDK server parses.
func (moment wireTime) MarshalJSON() ([]byte, error) {
	return []byte(`"` + time.Time(moment).UTC().Format("2006-01-02T15:04:05.000Z") + `"`), nil
}

type reportOutcome int

const (
	reportSkippedEmpty reportOutcome = iota
	reportSent
	reportFailed
	reportRefusedForGood
)

func outcomeOfStatus(status int) reportOutcome {
	switch {
	case status >= 200 && status < 300:
		return reportSent
	case transport.IsFatalStatus(status):
		return reportRefusedForGood
	default:
		return reportFailed
	}
}

type httpReporter struct {
	options     transport.Options
	endpoint    string
	endpointErr error
}

func newHTTPReporter(options transport.Options) *httpReporter {
	endpoint, err := transport.EndpointURL(options.BaseURL, telemetryPath)
	return &httpReporter{options: options, endpoint: endpoint, endpointErr: err}
}

func (reporter *httpReporter) send(report eventReport) reportOutcome {
	logger := reporter.options.Logger
	if reporter.endpointErr != nil {
		logger.Error("The telemetry URL is not usable; no telemetry is sent", slog.Any("cause", reporter.endpointErr))
		return reportRefusedForGood
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	status, err := reporter.options.PostJSON(ctx, reporter.endpoint, report.wire(reporter.options.SDKKey, reporter.options.MetaContext))
	if err != nil {
		logger.Warn("Sending the telemetry report failed; the events in it are discarded", slog.Any("cause", err))
		return reportFailed
	}

	outcome := outcomeOfStatus(status)
	switch outcome {
	case reportSent:
		logger.Debug("Sent a telemetry report", slog.Int("evaluations", len(report.evaluations)), slog.Int("contexts", len(report.contexts)), slog.Int("invalidContexts", len(report.invalidContexts.entries)))
	case reportRefusedForGood:
		logger.Error("The SDK server refused the telemetry report; no more telemetry is sent", slog.Int("status", status), slog.String("detail", http.StatusText(status)))
	case reportFailed:
		logger.Warn("The SDK server answered the telemetry report with an error; the events in it are discarded", slog.Int("status", status))
	case reportSkippedEmpty:
	}
	return outcome
}
