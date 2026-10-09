package evaluation

import (
	"cmp"
	"math"
	"slices"
)

type RuleKind string

const (
	RuleKindConditional RuleKind = "conditional"
	RuleKindPercentage  RuleKind = "percentage"
)

type RuleTarget string

const (
	RuleTargetValue      RuleTarget = "value"
	RuleTargetPercentage RuleTarget = "percentage"
)

type Percentage struct {
	ID         string
	Percentage float64
	Value      string
	ValueID    string
}

type Rule struct {
	ID          string
	Order       *int
	Kind        RuleKind
	Target      RuleTarget
	Value       string
	ValueID     string
	Conditions  []Condition
	Percentages []Percentage
}

type TargetingRules struct {
	DefaultValue   string
	DefaultValueID string
	Rules          []Rule
}

type Config struct {
	ID             string
	TargetingRules TargetingRules
}

type ServedValue struct {
	Text        string
	ValueID     string
	RuleMatched bool
}

func Evaluate(config Config, context EvaluationContext, segments Segments) ServedValue {
	for _, rule := range inEvaluationOrder(config.TargetingRules.Rules) {
		if served, applies := rule.serve(config.ID, context, segments); applies {
			return served
		}
	}
	return ServedValue{
		Text:        config.TargetingRules.DefaultValue,
		ValueID:     config.TargetingRules.DefaultValueID,
		RuleMatched: false,
	}
}

func inEvaluationOrder(rules []Rule) []Rule {
	if slices.IsSortedFunc(rules, byOrder) {
		return rules
	}
	ordered := slices.Clone(rules)
	slices.SortStableFunc(ordered, byOrder)
	return ordered
}

func byOrder(left Rule, right Rule) int {
	return cmp.Compare(left.orderOrLast(), right.orderOrLast())
}

func (rule Rule) orderOrLast() int {
	if rule.Order == nil {
		return math.MaxInt
	}
	return *rule.Order
}

func (rule Rule) serve(configID string, context EvaluationContext, segments Segments) (ServedValue, bool) {
	switch rule.Kind {
	case RuleKindPercentage:
		return rule.serveFromBucket(configID, context)
	case RuleKindConditional:
		if !rule.conditionsHold(context, segments) {
			return ServedValue{}, false
		}
		switch rule.Target {
		case RuleTargetValue:
			return ServedValue{Text: rule.Value, ValueID: rule.ValueID, RuleMatched: true}, true
		case RuleTargetPercentage:
			return rule.serveFromBucket(configID, context)
		default:
			return ServedValue{}, false
		}
	default:
		return ServedValue{}, false
	}
}

func (rule Rule) conditionsHold(context EvaluationContext, segments Segments) bool {
	for _, condition := range rule.Conditions {
		if !condition.holdsFor(context, segments) {
			return false
		}
	}
	return true
}

func (rule Rule) serveFromBucket(configID string, context EvaluationContext) (ServedValue, bool) {
	bucket, found := bucketFor(rule.Percentages, assignedPercentage(configID, context.Identifier))
	if !found {
		return ServedValue{}, false
	}
	return ServedValue{Text: bucket.Value, ValueID: bucket.ValueID, RuleMatched: true}, true
}
