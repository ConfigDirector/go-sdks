package evaluation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const contractConfigID = "11111111-1111-4111-8111-111111111111"

func valueRule(id string, position *int, value string, conditions ...Condition) Rule {
	return Rule{
		ID:         id,
		Order:      position,
		Kind:       RuleKindConditional,
		Target:     RuleTargetValue,
		Value:      value,
		ValueID:    id + "-value",
		Conditions: conditions,
	}
}

func identifierIs(identifier string) AttributeCondition {
	return textCondition("identifier", "", "equals", identifier)
}

func share(id string, width float64, value string) Percentage {
	return Percentage{ID: id, Percentage: width, Value: value, ValueID: id + "-value"}
}

func percentageRule(shares ...Percentage) Rule {
	return Rule{ID: "rollout", Order: new(0), Kind: RuleKindPercentage, Target: RuleTargetPercentage, Percentages: shares}
}

func configWith(rules ...Rule) Config {
	return Config{
		ID: contractConfigID,
		TargetingRules: TargetingRules{
			DefaultValue:   "NONE",
			DefaultValueID: "default-value",
			Rules:          rules,
		},
	}
}

func TestAConfigWithoutRulesServesTheDefaultTargetingRuleValue(t *testing.T) {
	served := Evaluate(configWith(), Context{Identifier: "u1"}, nil)

	require.Equal(t, ServedValue{Text: "NONE", ValueID: "default-value", RuleMatched: false}, served)
}

func TestTheFirstRuleThatAppliesServesItsValue(t *testing.T) {
	config := configWith(
		valueRule("first", new(0), "A", identifierIs("u2")),
		valueRule("second", new(1), "B", identifierIs("u1")),
		valueRule("third", new(2), "C", identifierIs("u1")),
	)

	served := Evaluate(config, Context{Identifier: "u1"}, nil)

	require.Equal(t, ServedValue{Text: "B", ValueID: "second-value", RuleMatched: true}, served)
}

func TestRulesAreTriedInAscendingOrderRegardlessOfTheirPosition(t *testing.T) {
	config := configWith(
		valueRule("listed-first", new(1), "B"),
		valueRule("listed-second", new(0), "A"),
	)

	served := Evaluate(config, Context{Identifier: "u1"}, nil)

	require.Equal(t, "A", served.Text)
}

func TestRulesWithoutAnOrderAreTriedAfterEveryOrderedRule(t *testing.T) {
	config := configWith(
		valueRule("unordered", nil, "C"),
		valueRule("ordered", new(5), "A"),
	)

	served := Evaluate(config, Context{Identifier: "u1"}, nil)

	require.Equal(t, "A", served.Text)
}

func TestRulesWithoutAnOrderKeepTheOrderListed(t *testing.T) {
	config := configWith(
		valueRule("unordered-first", nil, "C"),
		valueRule("unordered-second", nil, "D"),
	)

	served := Evaluate(config, Context{Identifier: "u1"}, nil)

	require.Equal(t, "C", served.Text)
}

func TestNoRuleApplyingServesTheDefaultTargetingRuleValue(t *testing.T) {
	config := configWith(valueRule("only", new(0), "A", identifierIs("u2")))

	served := Evaluate(config, Context{Identifier: "u1"}, nil)

	require.Equal(t, ServedValue{Text: "NONE", ValueID: "default-value", RuleMatched: false}, served)
}

func TestAConditionalRuleAppliesOnlyWhenEveryConditionHolds(t *testing.T) {
	config := configWith(valueRule("both", new(0), "A", identifierIs("u1"), planIs("pro")))

	require.True(t, Evaluate(config, Context{Identifier: "u1", Traits: map[string]any{"plan": "pro"}}, nil).RuleMatched)
	require.False(t, Evaluate(config, Context{Identifier: "u1", Traits: map[string]any{"plan": "free"}}, nil).RuleMatched)
}

func TestARuleWithNoConditionsAppliesToEveryContext(t *testing.T) {
	config := configWith(valueRule("everyone", new(0), "A"))

	require.True(t, Evaluate(config, Context{}, nil).RuleMatched)
}

func TestASegmentConditionInARuleIsCheckedAgainstTheSegments(t *testing.T) {
	config := configWith(valueRule("acme", new(0), "A", SegmentCondition{Operator: "in", SegmentID: acmeSegmentID}))
	segments := acmeSegment([]AttributeCondition{emailEndsWith("@acme.com")})

	require.True(t, Evaluate(config, Context{Traits: map[string]any{"email": "ann@acme.com"}}, segments).RuleMatched)
	require.False(t, Evaluate(config, Context{Traits: map[string]any{"email": "ann@acme.com"}}, nil).RuleMatched)
}

func TestARuleOfAnUnknownKindIsSkipped(t *testing.T) {
	unknown := valueRule("unknown", new(0), "A")
	unknown.Kind = "schedule"
	config := configWith(unknown, valueRule("known", new(1), "B"))

	served := Evaluate(config, Context{}, nil)

	require.Equal(t, "B", served.Text)
}

func TestAConditionalRuleWithAnUnknownTargetIsSkipped(t *testing.T) {
	unknown := valueRule("unknown", new(0), "A")
	unknown.Target = "variation"
	config := configWith(unknown, valueRule("known", new(1), "B"))

	served := Evaluate(config, Context{}, nil)

	require.Equal(t, "B", served.Text)
}

func TestAConditionalRuleCanTargetAPercentageRollout(t *testing.T) {
	rollout := percentageRule(share("b0", 50, "0"), share("b1", 50, "1"))
	rollout.Kind = RuleKindConditional
	rollout.Conditions = []Condition{identifierIs("u582")}
	config := configWith(rollout)

	served := Evaluate(config, Context{Identifier: "u582"}, nil)

	require.Equal(t, ServedValue{Text: "1", ValueID: "b1-value", RuleID: "rollout", ShareID: "b1", RuleMatched: true}, served)
}

func TestAContextOnAShareBoundaryTakesTheShareThatStartsThere(t *testing.T) {
	config := configWith(percentageRule(share("b0", 50, "0"), share("b1", 50, "1")))

	served := Evaluate(config, Context{Identifier: "u582"}, nil)

	require.Equal(t, ServedValue{Text: "1", ValueID: "b1-value", RuleID: "rollout", ShareID: "b1", RuleMatched: true}, served)
}

func TestAContextBelowAShareBoundaryTakesTheShareThatEndsThere(t *testing.T) {
	config := configWith(percentageRule(share("b0", 50, "0"), share("b1", 50, "1")))

	served := Evaluate(config, Context{Identifier: "user-1"}, nil)

	require.Equal(t, "0", served.Text)
}

func TestAZeroWidthShareServesNobody(t *testing.T) {
	config := configWith(percentageRule(share("b0", 50, "0"), share("b1", 0, "1"), share("b2", 50, "2")))

	served := Evaluate(config, Context{Identifier: "u582"}, nil)

	require.Equal(t, "2", served.Text)
}

func TestAContextWithoutAnIdentifierSkipsTheRollout(t *testing.T) {
	config := configWith(percentageRule(share("b0", 0, "0"), share("b1", 10, "1"), share("b2", 90, "2")))

	served := Evaluate(config, Context{}, nil)

	require.Equal(t, ServedValue{Text: "NONE", ValueID: "default-value", RolloutSkippedNoIdentifier: true}, served)
}

func TestADeviceWithoutAnIdentifierSkipsTheRolloutToo(t *testing.T) {
	config := configWith(percentageRule(share("b0", 0, "0"), share("b1", 10, "1"), share("b2", 90, "2")))

	served := Evaluate(config, Context{Kind: "device"}, nil)

	require.Equal(t, ServedValue{Text: "NONE", ValueID: "default-value", RolloutSkippedNoIdentifier: true}, served)
}

func TestASkippedRolloutReportsTheVariationOfTheDefaultTargetingRuleValue(t *testing.T) {
	config := configWith(percentageRule(share("b0", 100, "0")))
	config.TargetingRules.DefaultVariationID = "control"

	served := Evaluate(config, Context{}, nil)

	require.Equal(t, ServedValue{Text: "NONE", ValueID: "default-value", VariationID: "control", RolloutSkippedNoIdentifier: true}, served)
}

func TestAContextWithoutAnIdentifierIsServedByTheRuleAfterASkippedRollout(t *testing.T) {
	config := configWith(percentageRule(share("b0", 100, "0")), valueRule("next", new(1), "B"))

	served := Evaluate(config, Context{}, nil)

	require.Equal(t, ServedValue{Text: "B", ValueID: "next-value", RuleMatched: true}, served)
}

func TestAContextWithoutAnIdentifierMatchingAConditionalRolloutIsServedByALaterRuleOnTheAppVersion(t *testing.T) {
	rollout := percentageRule(share("b0", 100, "0"))
	rollout.Kind = RuleKindConditional
	rollout.Conditions = []Condition{planIs("pro")}
	config := configWith(rollout, valueRule("version-2", new(1), "B", appVersionAtLeast("2.0.0")))

	served := Evaluate(config, Context{Traits: map[string]any{"plan": "pro"}, AppVersion: "2.1.0"}, nil)

	require.Equal(t, ServedValue{Text: "B", ValueID: "version-2-value", RuleMatched: true}, served)
}

func TestAContextWithoutAnIdentifierMatchingNoRuleAfterASkippedRolloutGetsTheDefaultTargetingRuleValue(t *testing.T) {
	config := configWith(percentageRule(share("b0", 100, "0")), valueRule("next", new(1), "B", planIs("pro")))

	served := Evaluate(config, Context{Traits: map[string]any{"plan": "free"}}, nil)

	require.Equal(t, ServedValue{Text: "NONE", ValueID: "default-value", RolloutSkippedNoIdentifier: true}, served)
}

func TestARuleBeforeTheRolloutStillServesAContextWithoutAnIdentifier(t *testing.T) {
	rollout := percentageRule(share("b0", 100, "0"))
	rollout.Order = new(1)
	config := configWith(rollout, valueRule("first", new(0), "A"))

	served := Evaluate(config, Context{}, nil)

	require.Equal(t, "A", served.Text)
	require.False(t, served.RolloutSkippedNoIdentifier)
}

func TestAConditionalRolloutWhoseConditionsHoldSkipsForAContextWithoutAnIdentifier(t *testing.T) {
	rollout := percentageRule(share("b0", 100, "0"))
	rollout.Kind = RuleKindConditional
	rollout.Conditions = []Condition{planIs("pro")}
	config := configWith(rollout)

	served := Evaluate(config, Context{Traits: map[string]any{"plan": "pro"}}, nil)

	require.Equal(t, ServedValue{Text: "NONE", ValueID: "default-value", RolloutSkippedNoIdentifier: true}, served)
}

func TestAConditionalRolloutWhoseConditionsFailFallsThroughForAContextWithoutAnIdentifier(t *testing.T) {
	rollout := percentageRule(share("b0", 100, "0"))
	rollout.Kind = RuleKindConditional
	rollout.Conditions = []Condition{planIs("pro")}
	config := configWith(rollout, valueRule("next", new(1), "B"))

	served := Evaluate(config, Context{Traits: map[string]any{"plan": "free"}}, nil)

	require.Equal(t, "B", served.Text)
	require.False(t, served.RolloutSkippedNoIdentifier)
}

func TestARolloutWithoutAShareListIsNotMatchedForAContextWithoutAnIdentifier(t *testing.T) {
	config := configWith(percentageRule(), valueRule("next", new(1), "B"))

	served := Evaluate(config, Context{}, nil)

	require.Equal(t, "B", served.Text)
	require.False(t, served.RolloutSkippedNoIdentifier)
}

func TestARolloutWithAnEmptyShareListIsSkippedForAContextWithoutAnIdentifierAndTheNextRuleIsTried(t *testing.T) {
	empty := percentageRule()
	empty.Percentages = []Percentage{}
	config := configWith(empty, valueRule("next", new(1), "B"))

	served := Evaluate(config, Context{}, nil)

	require.Equal(t, "B", served.Text)
	require.False(t, served.RolloutSkippedNoIdentifier)
}

func TestARolloutWithAnEmptyShareListAndNoLaterRuleIsSkippedForAContextWithoutAnIdentifier(t *testing.T) {
	empty := percentageRule()
	empty.Percentages = []Percentage{}
	config := configWith(empty)

	served := Evaluate(config, Context{}, nil)

	require.Equal(t, "NONE", served.Text)
	require.True(t, served.RolloutSkippedNoIdentifier)
}

func TestARolloutWithAnEmptyShareListFallsThroughForAContextWithAnIdentifier(t *testing.T) {
	empty := percentageRule()
	empty.Percentages = []Percentage{}
	config := configWith(empty, valueRule("next", new(1), "B"))

	served := Evaluate(config, Context{Identifier: "u1"}, nil)

	require.Equal(t, "B", served.Text)
}

func TestARolloutReportsItsRuleShareAndVariation(t *testing.T) {
	treatment := share("b1", 50, "1")
	treatment.VariationID = "treatment"
	config := configWith(percentageRule(share("b0", 50, "0"), treatment))

	served := Evaluate(config, Context{Identifier: "u582"}, nil)

	require.Equal(t, ServedValue{Text: "1", ValueID: "b1-value", VariationID: "treatment", RuleID: "rollout", ShareID: "b1", RuleMatched: true}, served)
}

func TestARuleServingASingleValueReportsItsVariationAndNoRuleOrShare(t *testing.T) {
	rule := valueRule("only", new(0), "A")
	rule.VariationID = "treatment"
	config := configWith(rule)

	served := Evaluate(config, Context{}, nil)

	require.Equal(t, ServedValue{Text: "A", ValueID: "only-value", VariationID: "treatment", RuleMatched: true}, served)
}

func TestTheDefaultTargetingRuleValueReportsItsVariationAndNoRuleOrShare(t *testing.T) {
	config := configWith()
	config.TargetingRules.DefaultVariationID = "control"

	served := Evaluate(config, Context{Identifier: "u1"}, nil)

	require.Equal(t, ServedValue{Text: "NONE", ValueID: "default-value", VariationID: "control"}, served)
}

func TestADeviceAndAUserWithTheSameIdentifierCanLandInDifferentShares(t *testing.T) {
	config := configWith(percentageRule(share("b0", 50, "0"), share("b1", 50, "1")))
	config.ID = "11111111-1111-4111-8111-111111111111"

	user := Evaluate(config, Context{Identifier: "user-1"}, nil)
	device := Evaluate(config, Context{Identifier: "user-1", Kind: "device"}, nil)

	require.Equal(t, "0", user.Text)
	require.Equal(t, "1", device.Text)
}

func TestSharesThatDoNotCoverTheContextServeTheDefaultTargetingRuleValue(t *testing.T) {
	config := configWith(percentageRule(share("b0", 10, "0"), share("b1", 10, "1")))

	served := Evaluate(config, Context{Identifier: "u582"}, nil)

	require.Equal(t, ServedValue{Text: "NONE", ValueID: "default-value", RuleMatched: false}, served)
}

func TestAPercentageRuleCoveringNobodyFallsThroughToTheNextRule(t *testing.T) {
	config := configWith(percentageRule(share("b0", 0, "0")), valueRule("next", new(1), "B"))

	served := Evaluate(config, Context{Identifier: "u582"}, nil)

	require.Equal(t, "B", served.Text)
}
