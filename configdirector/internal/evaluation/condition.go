package evaluation

import "strings"

type EvaluationContext struct {
	Identifier string
	Name       string
	Traits     map[string]any
	AppName    string
	AppVersion string
}

type Condition interface {
	holdsFor(context EvaluationContext, segments Segments) bool
}

type AttributeCondition struct {
	ID           string
	Attribute    string
	Trait        string
	Operator     string
	TargetType   string
	TargetValues []string
}

type SegmentCondition struct {
	ID        string
	Operator  string
	SegmentID string
}

type Segment struct {
	Groups [][]AttributeCondition
}

type Segments map[string]Segment

func (condition AttributeCondition) holdsFor(context EvaluationContext, _ Segments) bool {
	value, known := condition.attributeValue(context)
	if !known {
		return false
	}

	operator := strings.ToLower(condition.Operator)
	text, _ := textOf(value)
	switch condition.TargetType {
	case "text":
		return textMatches(text, operator, condition.TargetValues)
	case "number":
		return numberMatches(value, operator, condition.TargetValues)
	case "semver":
		return semverMatches(text, operator, condition.TargetValues)
	case "datetime":
		return datetimeMatches(text, operator, condition.TargetValues)
	case "array":
		return arrayMatches(value, operator, condition.TargetValues)
	default:
		return false
	}
}

func (condition AttributeCondition) attributeValue(context EvaluationContext) (any, bool) {
	switch condition.Attribute {
	case "identifier":
		return context.Identifier, true
	case "name":
		return context.Name, true
	case "appName":
		return context.AppName, true
	case "appVersion":
		return context.AppVersion, true
	case "traits":
		if condition.Trait == "" {
			return nil, true
		}
		return traitAtPointer(context.Traits, condition.Trait), true
	default:
		return nil, false
	}
}

func (condition SegmentCondition) holdsFor(context EvaluationContext, segments Segments) bool {
	segment, found := segments[condition.SegmentID]
	if !found {
		return false
	}

	switch strings.ToLower(condition.Operator) {
	case "in":
		return segment.contains(context)
	case "not in":
		return !segment.contains(context)
	default:
		return false
	}
}

func (segment Segment) contains(context EvaluationContext) bool {
	for _, group := range segment.Groups {
		if everyConditionHolds(group, context) {
			return true
		}
	}
	return false
}

func everyConditionHolds(group []AttributeCondition, context EvaluationContext) bool {
	for _, condition := range group {
		if !condition.holdsFor(context, nil) {
			return false
		}
	}
	return true
}
