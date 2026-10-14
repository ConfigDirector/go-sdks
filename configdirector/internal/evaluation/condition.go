package evaluation

import "strings"

// Context is what targeting rules match on: the context a config is evaluated for and the
// application asking. Kind is the context kind as supplied, already validated by the client;
// empty means user.
type Context struct {
	Identifier string
	Kind       string
	Name       string
	Traits     map[string]any
	AppName    string
	AppVersion string
}

// UserContextKind is the effective context kind of a context whose kind was omitted, and of an
// evaluation with no context at all.
const UserContextKind = "user"

func (context Context) effectiveContextKind() string {
	return EffectiveContextKindOf(context.Kind)
}

// EffectiveContextKindOf is the context kind targeting rules and percentage rollouts see for a
// context whose kind is contextKind: contextKind itself when one was supplied, otherwise
// UserContextKind.
func EffectiveContextKindOf(contextKind string) string {
	if contextKind == "" {
		return UserContextKind
	}
	return contextKind
}

// Condition is one condition of a conditional rule. The two kinds are AttributeCondition and
// SegmentCondition.
type Condition interface {
	holdsFor(context Context, segments Segments) bool
}

// AttributeCondition compares one attribute of the context, or one of its traits, with the
// condition's target values.
type AttributeCondition struct {
	ID           string
	Attribute    string
	Trait        string
	Operator     string
	TargetType   string
	TargetValues []string
}

// SegmentCondition holds when the context is in, or not in, a segment.
type SegmentCondition struct {
	ID        string
	Operator  string
	SegmentID string
}

// Segment is a set of contexts: those for which every condition of at least one group holds.
type Segment struct {
	Groups [][]AttributeCondition
}

// Segments are the segments a payload carries, by id.
type Segments map[string]Segment

func (condition AttributeCondition) holdsFor(context Context, _ Segments) bool {
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

func (condition AttributeCondition) attributeValue(context Context) (any, bool) {
	switch condition.Attribute {
	case "identifier":
		return context.Identifier, true
	case "kind":
		return context.effectiveContextKind(), true
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

func (condition SegmentCondition) holdsFor(context Context, segments Segments) bool {
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

func (segment Segment) contains(context Context) bool {
	for _, group := range segment.Groups {
		if everyConditionHolds(group, context) {
			return true
		}
	}
	return false
}

func everyConditionHolds(group []AttributeCondition, context Context) bool {
	for _, condition := range group {
		if !condition.holdsFor(context, nil) {
			return false
		}
	}
	return true
}
