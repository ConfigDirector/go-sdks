package configdirector

import (
	"log/slog"
	"reflect"

	"github.com/ConfigDirector/go-sdks/configdirector/internal/evaluation"
	"github.com/ConfigDirector/go-sdks/configdirector/internal/telemetry"
)

type telemetryCollector interface {
	Record(evaluated telemetry.Evaluation, context telemetry.Context)
	RecordInvalidContext(contextKindSample string) telemetry.InvalidContextRecording
	RecordTrackedEvent(admission telemetry.TrackedEventAdmission, userContext UserContext)
	Close()
}

type reportingTelemetry struct {
	*telemetry.Collector
}

func (reporting reportingTelemetry) RecordTrackedEvent(admission telemetry.TrackedEventAdmission, _ UserContext) {
	reporting.Collector.RecordTrackedEvent(admission)
}

type discardingTelemetry struct {
	telemetry.Discarding
}

func (discardingTelemetry) RecordTrackedEvent(telemetry.TrackedEventAdmission, UserContext) {}

type trackedEventRecordingTelemetry struct {
	telemetry.Discarding
	logger *slog.Logger
	record func(admission telemetry.TrackedEventAdmission, userContext UserContext)
}

func (recording trackedEventRecordingTelemetry) RecordTrackedEvent(admission telemetry.TrackedEventAdmission, userContext UserContext) {
	for _, warning := range admission.Warnings {
		recording.logger.Warn(string(warning))
	}
	if admission.Admitted {
		recording.record(admission, userContext)
	}
}

func startTelemetry(settings clientSettings) reportingTelemetry {
	return reportingTelemetry{telemetry.Start(telemetry.Options{
		Transport:       settings.transportOptions(),
		EventQueueLimit: settings.eventQueueLimit,
		FlushInterval:   settings.flushInterval,
	})}
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
	ruleID     string
	shareID    string
}

func servedRead(kind readKind, key string, value any, config servedConfig, userContext UserContext) configRead {
	return configRead{
		kind:       kind,
		evaluation: servedEvaluation(key, value, config, userContext),
		configType: config.state.Type,
		ruleID:     config.state.RuleID,
		shareID:    config.state.ShareID,
	}
}

func inCodeDefaultRead(kind readKind, key string, defaultValue any, config servedConfig, reason EvaluationReason, userContext UserContext) configRead {
	return configRead{kind: kind, evaluation: defaultEvaluation(key, defaultValue, reason, userContext), configType: config.state.Type}
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
		RuleID:           read.ruleID,
		ShareID:          read.shareID,
		VariationID:      variationIDOf(read.evaluation),
	}, telemetryContextOf(read.evaluation.UserContext))
	client.reportEvaluation(read.evaluation)
}

func variationIDOf(evaluated Evaluation) string {
	if evaluated.Variation == nil {
		return ""
	}
	return evaluated.Variation.ID
}

func telemetryContextOf(userContext UserContext) telemetry.Context {
	return telemetry.Context{
		ID:        userContext.ID,
		Kind:      evaluation.EffectiveContextKindOf(userContext.Kind),
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
