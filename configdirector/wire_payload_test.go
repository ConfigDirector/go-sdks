package configdirector_test

import (
	"encoding/json"
	"maps"
)

const (
	environmentID        = "6f1d2a3b-0000-4000-8000-00000000e001"
	projectID            = "6f1d2a3b-0000-4000-8000-00000000a001"
	betaTestersSegmentID = "segment-beta-testers"
)

type payloadBuilder struct {
	fields map[string]any
}

func fullPayload(configs ...map[string]any) payloadBuilder {
	return payloadOfKind("full", configs)
}

func deltaPayload(configs ...map[string]any) payloadBuilder {
	return payloadOfKind("delta", configs)
}

func payloadOfKind(kind string, configs []map[string]any) payloadBuilder {
	byKey := map[string]any{}
	for _, config := range configs {
		byKey[config["key"].(string)] = config
	}
	return payloadBuilder{fields: map[string]any{
		"payloadVersion": 1,
		"environmentId":  environmentID,
		"projectId":      projectID,
		"kind":           kind,
		"configs":        byKey,
		"segments":       map[string]any{},
	}}
}

func (payload payloadBuilder) withSegment(segmentID string, plan string) payloadBuilder {
	payload.fields["segments"].(map[string]any)[segmentID] = map[string]any{
		"groups": []any{[]any{traitEquals("/plan", plan)}},
	}
	return payload
}

func (payload payloadBuilder) withSegmentDefinition(segmentID string, definition any) payloadBuilder {
	payload.fields["segments"].(map[string]any)[segmentID] = definition
	return payload
}

func (payload payloadBuilder) withConfig(key string, config any) payloadBuilder {
	payload.fields["configs"].(map[string]any)[key] = config
	return payload
}

func (payload payloadBuilder) withField(name string, value any) payloadBuilder {
	payload.fields[name] = value
	return payload
}

func (payload payloadBuilder) json() string {
	encoded, err := json.Marshal(payload.fields)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func configOf(key string, configType string, defaultValue string, rules ...map[string]any) map[string]any {
	if rules == nil {
		rules = []map[string]any{}
	}
	return map[string]any{
		"id":              "config-" + key,
		"key":             key,
		"type":            configType,
		"typeConstraints": nil,
		"variations":      []any{},
		"target": map[string]any{
			"environmentId":  environmentID,
			"rules":          rules,
			"defaultValue":   defaultValue,
			"defaultValueId": "value-" + key + "-default",
		},
	}
}

func textConfig(key string, value string) map[string]any {
	return configOf(key, "string", value)
}

func boolConfig(key string, value string) map[string]any {
	return configOf(key, "boolean", value)
}

func intConfig(key string, value string) map[string]any {
	return configOf(key, "integer", value)
}

func floatConfig(key string, value string) map[string]any {
	return configOf(key, "float", value)
}

func jsonConfig(key string, value string) map[string]any {
	return configOf(key, "json", value)
}

func boolConfigTrueInSegment(key string, segmentID string) map[string]any {
	return configOf(key, "boolean", "false", valueRule(key+"-in-segment", 0, "true", inSegment(segmentID)))
}

func boolConfigTrueWhen(key string, condition map[string]any) map[string]any {
	return configOf(key, "boolean", "false", valueRule(key+"-matches", 0, "true", condition))
}

func valueRule(id string, order int, value string, conditions ...map[string]any) map[string]any {
	return map[string]any{
		"id":          id,
		"order":       order,
		"type":        "conditional",
		"target":      "value",
		"value":       value,
		"valueId":     "value-" + id,
		"conditions":  conditions,
		"percentages": nil,
	}
}

func identifierEquals(identifier string) map[string]any {
	return attributeCondition("identifier", nil, "=", "text", identifier)
}

func appNameEquals(appName string) map[string]any {
	return attributeCondition("appName", nil, "=", "text", appName)
}

func traitEquals(pointer string, value string) map[string]any {
	return traitCondition(pointer, "=", "text", value)
}

func traitCondition(pointer string, operator string, targetType string, target string) map[string]any {
	return attributeCondition("traits", pointer, operator, targetType, target)
}

func attributeCondition(attribute string, trait any, operator string, targetType string, targets ...string) map[string]any {
	return map[string]any{
		"id":           "condition-" + attribute,
		"kind":         "attribute",
		"attribute":    attribute,
		"trait":        trait,
		"operator":     operator,
		"targetType":   targetType,
		"targetValues": targets,
	}
}

func contextKindIsOneOf(contextKinds ...string) map[string]any {
	return attributeCondition("kind", nil, "is one of", "text", contextKinds...)
}

func evenSplitConfig(key string) map[string]any {
	rolledOut := configOf(key, "string", "NONE", percentageRule(key+"-split", bucket("b0", 50, "0"), bucket("b1", 50, "1")))
	return withField(rolledOut, "id", "11111111-1111-4111-8111-111111111111")
}

func percentageRule(id string, buckets ...map[string]any) map[string]any {
	return map[string]any{
		"id":          id,
		"order":       0,
		"type":        "percentage",
		"target":      "percentage",
		"value":       nil,
		"valueId":     nil,
		"conditions":  []map[string]any{},
		"percentages": buckets,
	}
}

func bucket(id string, percentage float64, value string) map[string]any {
	return map[string]any{"id": id, "percentage": percentage, "value": value, "valueId": "value-" + id}
}

func inSegment(segmentID string) map[string]any {
	return map[string]any{
		"id":        "condition-" + segmentID,
		"kind":      "segment",
		"operator":  "in",
		"segmentId": segmentID,
	}
}

func withField(object map[string]any, name string, value any) map[string]any {
	extended := maps.Clone(object)
	extended[name] = value
	return extended
}

func standardPayload() payloadBuilder {
	return fullPayload(
		boolConfig("new-checkout", "true"),
		boolConfig("upper-case-flag", "TRUE"),
		boolConfig("not-a-boolean", "yes"),
		textConfig("welcome-message", "Welcome"),
		textConfig("empty-message", ""),
		intConfig("max-items", "26"),
		intConfig("retry-limit", "26.0"),
		intConfig("not-a-number", "26abc"),
		floatConfig("price-multiplier", "1.5"),
		jsonConfig("limits", `{"max":20,"min":1}`),
		jsonConfig("allowed-regions", `["eu","us"]`),
		jsonConfig("broken-json", "{not json"),
		configOf("greeting", "string", "Hello", valueRule("greeting-pro", 0, "Hello, pro", identifierEquals("u-pro"))),
		boolConfigTrueInSegment("beta-banner", betaTestersSegmentID),
		configOf("maintenance-banner", "boolean", "false", valueRule("maintenance-banner-ops", 0, "true", appNameEquals("ops-console"))),
	).withSegment(betaTestersSegmentID, "beta")
}
