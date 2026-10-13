package configdirector

import (
	"testing"

	"github.com/ConfigDirector/go-sdks/configdirector/internal/evaluation"
	"github.com/stretchr/testify/require"
)

func textConfig(key string, value string) configDefinition {
	return configDefinition{
		id:             "config-" + key,
		key:            key,
		configType:     ConfigTypeString,
		targetingRules: evaluation.TargetingRules{DefaultValue: value, DefaultValueID: "value-" + key + "-default"},
	}
}

func payload(kind payloadKind, definitions ...configDefinition) serverPayload {
	byKey := make(map[string]configDefinition, len(definitions))
	for _, definition := range definitions {
		byKey[definition.key] = definition
	}
	return serverPayload{kind: kind, configs: byKey, segments: evaluation.Segments{}}
}

func withUnreadableConfigs(payload serverPayload, keys ...string) serverPayload {
	for _, key := range keys {
		payload.unreadableConfigs = append(payload.unreadableConfigs, unreadableConfig{key: key})
	}
	return payload
}

func withUnreadableSegments(payload serverPayload, segmentIDs ...string) serverPayload {
	for _, segmentID := range segmentIDs {
		payload.unreadableSegments = append(payload.unreadableSegments, unreadableSegment{id: segmentID})
	}
	return payload
}

func segmentMatchingIdentifier(identifier string) evaluation.Segment {
	return evaluation.Segment{Groups: [][]evaluation.AttributeCondition{{
		{ID: "condition-" + identifier, Attribute: "identifier", Operator: "=", TargetType: "text", TargetValues: []string{identifier}},
	}}}
}

func valuesOf(snapshot *configSnapshot) map[string]string {
	values := map[string]string{}
	for _, state := range snapshot.statesOf(snapshot.keys(), evaluation.Context{}) {
		values[state.Key] = state.Value
	}
	return values
}

func TestTheFirstPayloadBecomesTheSnapshotWhateverItsKind(t *testing.T) {
	var none *configSnapshot

	snapshot := none.withPayload(payload(payloadKindDelta, textConfig("a", "1")))

	require.Equal(t, map[string]string{"a": "1"}, valuesOf(snapshot))
}

func TestAFullPayloadReplacesThePreviousSnapshot(t *testing.T) {
	var none *configSnapshot
	previous := none.withPayload(payload(payloadKindFull, textConfig("a", "1"), textConfig("b", "2")))

	snapshot := previous.withPayload(payload(payloadKindFull, textConfig("b", "3"), textConfig("c", "4")))

	require.Equal(t, map[string]string{"b": "3", "c": "4"}, valuesOf(snapshot))
}

func TestAFullPayloadKeepsThePreviousDefinitionOfAConfigItCouldNotRead(t *testing.T) {
	var none *configSnapshot
	previous := none.withPayload(payload(payloadKindFull, textConfig("a", "1"), textConfig("b", "2")))

	snapshot := previous.withPayload(withUnreadableConfigs(payload(payloadKindFull, textConfig("a", "10")), "b"))

	require.Equal(t, map[string]string{"a": "10", "b": "2"}, valuesOf(snapshot))
}

func TestAFullPayloadKeepsThePreviousDefinitionOfASegmentItCouldNotRead(t *testing.T) {
	var none *configSnapshot
	previous := none.withPayload(serverPayload{kind: payloadKindFull, segments: evaluation.Segments{
		"s-1": segmentMatchingIdentifier("u-1"),
		"s-2": segmentMatchingIdentifier("u-2"),
	}})

	snapshot := previous.withPayload(withUnreadableSegments(
		serverPayload{kind: payloadKindFull, segments: evaluation.Segments{"s-1": segmentMatchingIdentifier("u-10")}},
		"s-2",
	))

	require.Equal(t, evaluation.Segments{"s-1": segmentMatchingIdentifier("u-10"), "s-2": segmentMatchingIdentifier("u-2")}, snapshot.segments)
}

func TestAFullPayloadHoldsNothingForASegmentItCouldNotReadThatWasNotHeld(t *testing.T) {
	var none *configSnapshot
	previous := none.withPayload(serverPayload{kind: payloadKindFull, segments: evaluation.Segments{"s-1": segmentMatchingIdentifier("u-1")}})

	snapshot := previous.withPayload(withUnreadableSegments(
		serverPayload{kind: payloadKindFull, segments: evaluation.Segments{"s-1": segmentMatchingIdentifier("u-10")}},
		"s-2",
	))

	require.Equal(t, evaluation.Segments{"s-1": segmentMatchingIdentifier("u-10")}, snapshot.segments)
}

func TestAFirstPayloadHoldsNothingForAConfigItCouldNotRead(t *testing.T) {
	var none *configSnapshot

	snapshot := none.withPayload(withUnreadableConfigs(payload(payloadKindFull, textConfig("a", "1")), "b"))

	require.Equal(t, map[string]string{"a": "1"}, valuesOf(snapshot))
}

func TestADeltaPayloadMergesIntoThePreviousSnapshot(t *testing.T) {
	var none *configSnapshot
	previous := none.withPayload(payload(payloadKindFull, textConfig("a", "1"), textConfig("b", "2")))

	snapshot := previous.withPayload(payload(payloadKindDelta, textConfig("b", "3"), textConfig("c", "4")))

	require.Equal(t, map[string]string{"a": "1", "b": "3", "c": "4"}, valuesOf(snapshot))
}

func TestADeltaPayloadMergesSegments(t *testing.T) {
	var none *configSnapshot
	previous := none.withPayload(serverPayload{kind: payloadKindFull, segments: evaluation.Segments{"s-1": {}}})

	snapshot := previous.withPayload(serverPayload{kind: payloadKindDelta, segments: evaluation.Segments{"s-2": {}}})

	require.Len(t, snapshot.segments, 2)
}

func TestAPayloadLeavesThePreviousSnapshotUntouched(t *testing.T) {
	var none *configSnapshot
	previous := none.withPayload(payload(payloadKindFull, textConfig("a", "1")))

	previous.withPayload(payload(payloadKindDelta, textConfig("b", "2")))

	require.Equal(t, map[string]string{"a": "1"}, valuesOf(previous))
}

func TestAPayloadWithoutConfigsYieldsAnEmptyReadySnapshot(t *testing.T) {
	var none *configSnapshot

	snapshot := none.withPayload(serverPayload{kind: payloadKindFull})

	require.NotNil(t, snapshot)
	require.Empty(t, snapshot.keys())
}
