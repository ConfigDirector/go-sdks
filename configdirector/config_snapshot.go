package configdirector

import (
	"maps"
	"slices"

	"github.com/ConfigDirector/go-sdks/configdirector/internal/evaluation"
)

type configSnapshot struct {
	configs  map[string]configDefinition
	segments evaluation.Segments
}

func (snapshot *configSnapshot) withPayload(payload serverPayload) *configSnapshot {
	if snapshot == nil || payload.kind == payloadKindFull {
		return &configSnapshot{
			configs:  maps.Clone(payload.configs),
			segments: maps.Clone(payload.segments),
		}
	}

	merged := &configSnapshot{
		configs:  maps.Clone(snapshot.configs),
		segments: maps.Clone(snapshot.segments),
	}
	maps.Copy(merged.configs, payload.configs)
	maps.Copy(merged.segments, payload.segments)
	return merged
}

func (snapshot *configSnapshot) stateOf(key string, context evaluation.EvaluationContext) (ConfigState, bool) {
	definition, found := snapshot.configs[key]
	if !found {
		return ConfigState{}, false
	}
	return definition.evaluatedFor(context, snapshot.segments), true
}

func (snapshot *configSnapshot) statesOf(keys []string, context evaluation.EvaluationContext) []ConfigState {
	states := make([]ConfigState, 0, len(keys))
	for _, key := range keys {
		if state, found := snapshot.stateOf(key, context); found {
			states = append(states, state)
		}
	}
	return states
}

func (snapshot *configSnapshot) keys() []string {
	return slices.Sorted(maps.Keys(snapshot.configs))
}

func (snapshot *configSnapshot) changesFrom(previous *configSnapshot, payload serverPayload) ConfigsUpdated {
	updated := ConfigsUpdated{Keys: []string{}, RemovedKeys: []string{}}
	for _, key := range snapshot.keys() {
		_, carried := payload.configs[key]
		if carried || snapshot.configs[key].usesAnySegment(payload.segments) {
			updated.Keys = append(updated.Keys, key)
		}
	}
	if previous != nil {
		for _, key := range previous.keys() {
			if _, held := snapshot.configs[key]; !held {
				updated.RemovedKeys = append(updated.RemovedKeys, key)
			}
		}
	}
	return updated
}

func (definition configDefinition) evaluatedFor(context evaluation.EvaluationContext, segments evaluation.Segments) ConfigState {
	config := evaluation.Config{ID: definition.id, TargetingRules: definition.targetingRules}
	served := evaluation.Evaluate(config, context, segments)
	return ConfigState{
		ID:      definition.id,
		Key:     definition.key,
		Type:    definition.configType,
		Value:   served.Text,
		ValueID: served.ValueID,
	}
}

func (definition configDefinition) usesAnySegment(segments evaluation.Segments) bool {
	for _, rule := range definition.targetingRules.Rules {
		for _, condition := range rule.Conditions {
			if segmentCondition, isSegment := condition.(evaluation.SegmentCondition); isSegment {
				if _, carried := segments[segmentCondition.SegmentID]; carried {
					return true
				}
			}
		}
	}
	return false
}
