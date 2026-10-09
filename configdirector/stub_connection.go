package configdirector

import (
	"sync"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector/internal/evaluation"
)

const stubConnectionDelay = 200 * time.Millisecond

type stubConnection struct {
	closing   chan struct{}
	delivered chan struct{}
	closeOnce sync.Once
}

func newStubConnection() *stubConnection {
	return &stubConnection{closing: make(chan struct{}), delivered: make(chan struct{})}
}

func (stub *stubConnection) open(receiver payloadReceiver) {
	go func() {
		defer close(stub.delivered)
		timer := time.NewTimer(stubConnectionDelay)
		defer timer.Stop()
		select {
		case <-timer.C:
			receiver.receive(stubPayload())
		case <-stub.closing:
		}
	}()
}

func (stub *stubConnection) close() error {
	stub.closeOnce.Do(func() { close(stub.closing) })
	<-stub.delivered
	return nil
}

func stubPayload() serverPayload {
	return serverPayload{
		kind: payloadKindFull,
		configs: definitionsByKey(
			stubConfig("new-checkout", ConfigTypeBoolean, "true"),
			stubConfig("upper-case-flag", ConfigTypeBoolean, "TRUE"),
			stubConfig("not-a-boolean", ConfigTypeBoolean, "yes"),
			stubConfig("welcome-message", ConfigTypeString, "Welcome"),
			stubConfig("empty-message", ConfigTypeString, ""),
			stubConfig("max-items", ConfigTypeInteger, "26"),
			stubConfig("retry-limit", ConfigTypeInteger, "26.0"),
			stubConfig("not-a-number", ConfigTypeInteger, "26abc"),
			stubConfig("price-multiplier", ConfigTypeFloat, "1.5"),
			stubConfig("limits", ConfigTypeJSON, `{"max":20,"min":1}`),
			stubConfig("allowed-regions", ConfigTypeJSON, `["eu","us"]`),
			stubConfig("broken-json", ConfigTypeJSON, "{not json"),
			stubConfigWithRules("greeting", ConfigTypeString, "Hello", []evaluation.Rule{
				valueRule("greeting-pro", 0, "Hello, pro", identifierEquals("u-pro")),
			}),
			stubConfigWithRules("beta-banner", ConfigTypeBoolean, "false", []evaluation.Rule{
				valueRule("beta-banner-testers", 0, "true", inSegment(betaTestersSegmentID)),
			}),
			stubConfigWithRules("maintenance-banner", ConfigTypeBoolean, "false", []evaluation.Rule{
				valueRule("maintenance-banner-ops", 0, "true", appNameEquals("ops-console")),
			}),
		),
		segments: evaluation.Segments{
			betaTestersSegmentID: {Groups: [][]evaluation.AttributeCondition{{traitEquals("/plan", "beta")}}},
		},
	}
}

const betaTestersSegmentID = "segment-beta-testers"

func definitionsByKey(definitions ...configDefinition) map[string]configDefinition {
	byKey := make(map[string]configDefinition, len(definitions))
	for _, definition := range definitions {
		byKey[definition.key] = definition
	}
	return byKey
}

func stubConfig(key string, configType ConfigType, value string) configDefinition {
	return stubConfigWithRules(key, configType, value, nil)
}

func stubConfigWithRules(key string, configType ConfigType, defaultValue string, rules []evaluation.Rule) configDefinition {
	return configDefinition{
		id:         "config-" + key,
		key:        key,
		configType: configType,
		targetingRules: evaluation.TargetingRules{
			DefaultValue:   defaultValue,
			DefaultValueID: "value-" + key + "-default",
			Rules:          rules,
		},
	}
}

func valueRule(id string, order int, value string, conditions ...evaluation.Condition) evaluation.Rule {
	return evaluation.Rule{
		ID:         id,
		Order:      &order,
		Kind:       evaluation.RuleKindConditional,
		Target:     evaluation.RuleTargetValue,
		Value:      value,
		ValueID:    "value-" + id,
		Conditions: conditions,
	}
}

func identifierEquals(identifier string) evaluation.AttributeCondition {
	return textCondition("identifier", "", identifier)
}

func appNameEquals(appName string) evaluation.AttributeCondition {
	return textCondition("appName", "", appName)
}

func traitEquals(pointer string, value string) evaluation.AttributeCondition {
	return textCondition("traits", pointer, value)
}

func textCondition(attribute string, trait string, target string) evaluation.AttributeCondition {
	return evaluation.AttributeCondition{
		ID:           "condition-" + attribute + trait,
		Attribute:    attribute,
		Trait:        trait,
		Operator:     "equals",
		TargetType:   "text",
		TargetValues: []string{target},
	}
}

func inSegment(segmentID string) evaluation.SegmentCondition {
	return evaluation.SegmentCondition{ID: "condition-" + segmentID, Operator: "in", SegmentID: segmentID}
}
