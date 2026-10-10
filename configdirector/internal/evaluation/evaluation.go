// Package evaluation serves a config's value for a context by its targeting rules, following the
// targeting rules contract every ConfigDirector SDK implements.
package evaluation

import (
	"cmp"
	"math"
	"slices"
)

// RuleKind is how a targeting rule decides whether it applies.
type RuleKind string

const (
	// RuleKindConditional is a rule that applies when every one of its conditions holds.
	RuleKindConditional RuleKind = "conditional"

	// RuleKindPercentage is a rule that applies to the share of contexts its percentages name.
	RuleKindPercentage RuleKind = "percentage"
)

// RuleTarget is what a conditional rule serves once it applies.
type RuleTarget string

const (
	// RuleTargetValue serves the rule's own value.
	RuleTargetValue RuleTarget = "value"

	// RuleTargetPercentage serves the value of the percentage bucket the context falls in.
	RuleTargetPercentage RuleTarget = "percentage"
)

// Percentage is one bucket of a percentage rollout: the share of contexts it covers and the
// value it serves them.
type Percentage struct {
	ID         string
	Percentage float64
	Value      string
	ValueID    string
}

// Rule is one targeting rule of a config, as the SDK server sends it.
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

// TargetingRules are a config's rules and the default targeting rule value served when none
// applies.
type TargetingRules struct {
	DefaultValue   string
	DefaultValueID string
	Rules          []Rule
}

// Config is a config as the evaluator sees it: its id, which seeds percentage rollouts, and its
// targeting rules.
type Config struct {
	ID             string
	TargetingRules TargetingRules
}

// ServedValue is the value the targeting rules served, as text, and whether a rule served it
// rather than the default targeting rule value.
type ServedValue struct {
	Text        string
	ValueID     string
	RuleMatched bool
}

// Evaluate serves config's value for context: the value of the first rule in evaluation order
// that applies, or the default targeting rule value. A segment condition is looked up in
// segments.
func Evaluate(config Config, context Context, segments Segments) ServedValue {
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

func (rule Rule) serve(configID string, context Context, segments Segments) (ServedValue, bool) {
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

func (rule Rule) conditionsHold(context Context, segments Segments) bool {
	for _, condition := range rule.Conditions {
		if !condition.holdsFor(context, segments) {
			return false
		}
	}
	return true
}

func (rule Rule) serveFromBucket(configID string, context Context) (ServedValue, bool) {
	bucket, found := bucketFor(rule.Percentages, assignedPercentage(configID, context.Identifier))
	if !found {
		return ServedValue{}, false
	}
	return ServedValue{Text: bucket.Value, ValueID: bucket.ValueID, RuleMatched: true}, true
}
