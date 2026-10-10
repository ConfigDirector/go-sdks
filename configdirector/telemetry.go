package configdirector

import (
	"reflect"

	"github.com/ConfigDirector/go-sdks/configdirector/internal/telemetry"
)

type telemetryCollector interface {
	Record(evaluation telemetry.Evaluation, context telemetry.Context)
	Close()
}

func startTelemetry(settings clientSettings) *telemetry.Collector {
	return telemetry.Start(telemetry.Options{
		Transport:       settings.transportOptions(),
		EventQueueLimit: settings.eventQueueLimit,
		FlushInterval:   settings.flushInterval,
	})
}

type readKind int

const (
	typedRead readKind = iota
	documentRead
)

func (kind readKind) reportsAsDocument(configType ConfigType) bool {
	return kind == documentRead || configType == ConfigTypeJSON
}

type configRead struct {
	kind       readKind
	evaluation Evaluation
	configType ConfigType
}

func (read configRead) reportsAsDocument() bool {
	return read.kind.reportsAsDocument(read.configType)
}

func (client *Client) reportRead(read configRead, defaultValue telemetry.Value, requestedType reflect.Type) {
	client.telemetry.Record(telemetry.Evaluation{
		Key:              read.evaluation.Key,
		ConfigType:       string(read.configType),
		DefaultValue:     defaultValue,
		RequestedType:    requestedTypeNameOf(requestedType),
		EvaluatedValue:   telemetry.ValueOf(read.evaluation.Value, read.reportsAsDocument(), read.evaluation.ValueID),
		EvaluatedValueID: read.evaluation.ValueID,
		UsedDefault:      read.evaluation.UsedDefault,
		Reason:           string(read.evaluation.Reason),
	}, telemetryContextOf(read.evaluation.UserContext))
	client.reportEvaluation(read.evaluation)
}

func telemetryContextOf(userContext UserContext) telemetry.Context {
	return telemetry.Context{
		ID:        userContext.ID,
		Name:      userContext.Name,
		Traits:    userContext.Traits,
		Anonymous: userContext.Anonymous,
	}
}

func requestedTypeNameOf(requestedType reflect.Type) string {
	if name := requestedType.Name(); name != "" {
		return name
	}
	return requestedType.String()
}
