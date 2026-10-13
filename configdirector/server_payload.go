package configdirector

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ConfigDirector/go-sdks/configdirector/internal/evaluation"
)

const supportedPayloadVersion = 1

type payloadKind string

const (
	payloadKindFull  payloadKind = "full"
	payloadKindDelta payloadKind = "delta"
)

type configDefinition struct {
	id             string
	key            string
	configType     ConfigType
	targetingRules evaluation.TargetingRules
}

type serverPayload struct {
	kind               payloadKind
	declaredVersion    json.RawMessage
	configs            map[string]configDefinition
	segments           evaluation.Segments
	unreadableConfigs  []unreadableConfig
	unreadableSegments []unreadableSegment
}

type unreadableConfig struct {
	key   string
	cause error
}

type unreadableSegment struct {
	id    string
	cause error
}

func (payload serverPayload) declaresASupportedVersion() bool {
	if payload.declaredVersion == nil {
		return true
	}
	var version float64
	return json.Unmarshal(payload.declaredVersion, &version) == nil && version == supportedPayloadVersion
}

func (payload serverPayload) unreadableConfigKeys() []string {
	keys := make([]string, 0, len(payload.unreadableConfigs))
	for _, config := range payload.unreadableConfigs {
		keys = append(keys, config.key)
	}
	return keys
}

func (payload serverPayload) unreadableSegmentIDs() []string {
	segmentIDs := make([]string, 0, len(payload.unreadableSegments))
	for _, segment := range payload.unreadableSegments {
		segmentIDs = append(segmentIDs, segment.id)
	}
	return segmentIDs
}

type wirePayload struct {
	PayloadVersion json.RawMessage            `json:"payloadVersion"`
	Kind           string                     `json:"kind"`
	Configs        map[string]json.RawMessage `json:"configs"`
	Segments       map[string]json.RawMessage `json:"segments"`
}

type wireConfig struct {
	ID     string      `json:"id"`
	Key    string      `json:"key"`
	Type   string      `json:"type"`
	Target *wireTarget `json:"target"`
}

type wireTarget struct {
	Rules          []wireRule      `json:"rules"`
	DefaultValue   json.RawMessage `json:"defaultValue"`
	DefaultValueID string          `json:"defaultValueId"`
}

type wireRule struct {
	ID          string            `json:"id"`
	Order       *int              `json:"order"`
	Type        string            `json:"type"`
	Target      string            `json:"target"`
	Value       json.RawMessage   `json:"value"`
	ValueID     string            `json:"valueId"`
	Conditions  []json.RawMessage `json:"conditions"`
	Percentages []wirePercentage  `json:"percentages"`
}

type wirePercentage struct {
	ID         string          `json:"id"`
	Percentage float64         `json:"percentage"`
	Value      json.RawMessage `json:"value"`
	ValueID    string          `json:"valueId"`
}

type wireCondition struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Attribute    string   `json:"attribute"`
	Trait        string   `json:"trait"`
	Operator     string   `json:"operator"`
	TargetType   string   `json:"targetType"`
	TargetValues []string `json:"targetValues"`
	SegmentID    string   `json:"segmentId"`
}

type wireSegment struct {
	Groups [][]json.RawMessage `json:"groups"`
}

const (
	attributeConditionKind = "attribute"
	segmentConditionKind   = "segment"
)

func parseServerPayload(document string) (serverPayload, error) {
	var wire wirePayload
	if err := json.Unmarshal([]byte(document), &wire); err != nil {
		return serverPayload{}, fmt.Errorf("the message is not a JSON object: %w", err)
	}
	if wire.Configs == nil {
		return serverPayload{}, errors.New("the message carries no configs object")
	}

	payload := serverPayload{
		kind:            payloadKindOf(wire.Kind),
		declaredVersion: wire.PayloadVersion,
		configs:         make(map[string]configDefinition, len(wire.Configs)),
		segments:        make(evaluation.Segments, len(wire.Segments)),
	}
	for key, raw := range wire.Configs {
		definition, err := parseConfig(raw)
		if err != nil {
			payload.unreadableConfigs = append(payload.unreadableConfigs, unreadableConfig{key: key, cause: err})
			continue
		}
		payload.configs[key] = definition
	}
	for segmentID, raw := range wire.Segments {
		segment, err := parseSegment(raw)
		if err != nil {
			payload.unreadableSegments = append(payload.unreadableSegments, unreadableSegment{id: segmentID, cause: err})
			continue
		}
		payload.segments[segmentID] = segment
	}
	return payload, nil
}

func payloadKindOf(declared string) payloadKind {
	if declared == string(payloadKindDelta) {
		return payloadKindDelta
	}
	return payloadKindFull
}

func parseConfig(raw json.RawMessage) (configDefinition, error) {
	var wire wireConfig
	if err := json.Unmarshal(raw, &wire); err != nil {
		return configDefinition{}, err
	}
	switch {
	case wire.ID == "":
		return configDefinition{}, errors.New("the config has no id")
	case wire.Key == "":
		return configDefinition{}, errors.New("the config has no key")
	case wire.Type == "":
		return configDefinition{}, errors.New("the config has no type")
	case wire.Target == nil:
		return configDefinition{}, errors.New("the config has no target")
	}

	rules := make([]evaluation.Rule, 0, len(wire.Target.Rules))
	for _, wireRule := range wire.Target.Rules {
		rule, err := parseRule(wireRule)
		if err != nil {
			return configDefinition{}, err
		}
		rules = append(rules, rule)
	}
	return configDefinition{
		id:         wire.ID,
		key:        wire.Key,
		configType: ConfigType(wire.Type),
		targetingRules: evaluation.TargetingRules{
			DefaultValue:   textOf(wire.Target.DefaultValue),
			DefaultValueID: wire.Target.DefaultValueID,
			Rules:          rules,
		},
	}, nil
}

func parseRule(wire wireRule) (evaluation.Rule, error) {
	if wire.ID == "" {
		return evaluation.Rule{}, errors.New("a rule has no id")
	}
	conditions := make([]evaluation.Condition, 0, len(wire.Conditions))
	for _, raw := range wire.Conditions {
		condition, err := parseCondition(raw)
		if err != nil {
			return evaluation.Rule{}, fmt.Errorf("rule %s: %w", wire.ID, err)
		}
		conditions = append(conditions, condition)
	}
	percentages := make([]evaluation.Percentage, 0, len(wire.Percentages))
	for _, bucket := range wire.Percentages {
		percentages = append(percentages, evaluation.Percentage{
			ID:         bucket.ID,
			Percentage: bucket.Percentage,
			Value:      textOf(bucket.Value),
			ValueID:    bucket.ValueID,
		})
	}
	return evaluation.Rule{
		ID:          wire.ID,
		Order:       wire.Order,
		Kind:        evaluation.RuleKind(wire.Type),
		Target:      evaluation.RuleTarget(wire.Target),
		Value:       textOf(wire.Value),
		ValueID:     wire.ValueID,
		Conditions:  conditions,
		Percentages: percentages,
	}, nil
}

func parseCondition(raw json.RawMessage) (evaluation.Condition, error) {
	var wire wireCondition
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, err
	}
	switch wire.Kind {
	case "", attributeConditionKind:
		return attributeConditionOf(wire)
	case segmentConditionKind:
		return segmentConditionOf(wire)
	default:
		return nil, fmt.Errorf("condition %s has the unknown kind %q", wire.ID, wire.Kind)
	}
}

func attributeConditionOf(wire wireCondition) (evaluation.AttributeCondition, error) {
	switch {
	case wire.Attribute == "":
		return evaluation.AttributeCondition{}, fmt.Errorf("condition %s has no attribute", wire.ID)
	case wire.Operator == "":
		return evaluation.AttributeCondition{}, fmt.Errorf("condition %s has no operator", wire.ID)
	case wire.TargetType == "":
		return evaluation.AttributeCondition{}, fmt.Errorf("condition %s has no target type", wire.ID)
	}
	return evaluation.AttributeCondition{
		ID:           wire.ID,
		Attribute:    wire.Attribute,
		Trait:        wire.Trait,
		Operator:     wire.Operator,
		TargetType:   wire.TargetType,
		TargetValues: wire.TargetValues,
	}, nil
}

func segmentConditionOf(wire wireCondition) (evaluation.SegmentCondition, error) {
	switch {
	case wire.Operator == "":
		return evaluation.SegmentCondition{}, fmt.Errorf("condition %s has no operator", wire.ID)
	case wire.SegmentID == "":
		return evaluation.SegmentCondition{}, fmt.Errorf("condition %s has no segment id", wire.ID)
	}
	return evaluation.SegmentCondition{ID: wire.ID, Operator: wire.Operator, SegmentID: wire.SegmentID}, nil
}

func parseSegment(raw json.RawMessage) (evaluation.Segment, error) {
	var wire wireSegment
	if err := json.Unmarshal(raw, &wire); err != nil {
		return evaluation.Segment{}, err
	}
	if wire.Groups == nil {
		return evaluation.Segment{}, errors.New("the segment has no groups")
	}

	groups := make([][]evaluation.AttributeCondition, 0, len(wire.Groups))
	for _, wireGroup := range wire.Groups {
		group := make([]evaluation.AttributeCondition, 0, len(wireGroup))
		for _, rawCondition := range wireGroup {
			condition, err := parseCondition(rawCondition)
			if err != nil {
				return evaluation.Segment{}, err
			}
			attributeCondition, isAttribute := condition.(evaluation.AttributeCondition)
			if !isAttribute {
				return evaluation.Segment{}, errors.New("a segment group holds attribute conditions only")
			}
			group = append(group, attributeCondition)
		}
		groups = append(groups, group)
	}
	return evaluation.Segment{Groups: groups}, nil
}

func textOf(raw json.RawMessage) string {
	if raw == nil || bytes.Equal(raw, []byte("null")) {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var compact bytes.Buffer
	if json.Compact(&compact, raw) != nil {
		return string(raw)
	}
	return compact.String()
}
