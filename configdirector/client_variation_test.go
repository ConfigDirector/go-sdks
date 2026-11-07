package configdirector_test

import (
	"testing"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/stretchr/testify/require"
)

var (
	userInTheTreatmentShare = configdirector.UserContext{ID: "u582"}

	controlVariation   = configdirector.Variation{ID: controlVariationID, Name: "control"}
	treatmentVariation = configdirector.Variation{ID: treatmentVariationID, Name: "treatment"}
)

func skippedRollout() configdirector.EvaluationDetails {
	return configdirector.EvaluationDetails{
		Reason:    configdirector.EvaluationReasonRolloutSkippedNoIdentifier,
		ValueID:   controlValueID,
		Variation: &controlVariation,
	}
}

func TestAReadServedByARolloutCarriesTheVariationOfItsShare(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, experimentConfig("checkout-cta", evenExperimentRollout()))

	value, details := client.StringValueDetails("checkout-cta", "fallback", userInTheTreatmentShare)

	require.Equal(t, treatmentText, value)
	require.Equal(t, configdirector.EvaluationDetails{
		Reason:    configdirector.EvaluationReasonFoundMatch,
		ValueID:   treatmentValueID,
		Variation: &treatmentVariation,
	}, details)
}

func TestAReadServedByARuleWithASingleValueCarriesItsVariation(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, experimentConfig("checkout-cta", treatmentRule()))

	_, details := client.StringValueDetails("checkout-cta", "fallback", identifiedUser)

	require.Equal(t, &treatmentVariation, details.Variation)
}

func TestAReadServedTheDefaultTargetingRuleValueCarriesItsVariation(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, experimentConfig("checkout-cta"))

	_, details := client.StringValueDetails("checkout-cta", "fallback", identifiedUser)

	require.Equal(t, configdirector.EvaluationDetails{
		Reason:    configdirector.EvaluationReasonFoundMatch,
		ValueID:   controlValueID,
		Variation: &controlVariation,
	}, details)
}

func TestAReadOfAConfigWithoutVariationsCarriesNoVariation(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, boolConfig("new-checkout", "true"))

	_, details := client.BoolValueDetails("new-checkout", false, identifiedUser)

	require.Nil(t, details.Variation)
}

func TestAnUnnamedVariationHasAnEmptyName(t *testing.T) {
	t.Parallel()
	rule := treatmentRule()
	rule["value"] = unnamedText
	rule["variationId"] = unnamedVariationID
	client := newReadyClientServing(t, experimentConfig("checkout-cta", rule))

	_, details := client.StringValueDetails("checkout-cta", "fallback", identifiedUser)

	require.Equal(t, &configdirector.Variation{ID: unnamedVariationID, Name: ""}, details.Variation)
}

func TestAVariationPayloadWithoutIDsCarriesNoVariation(t *testing.T) {
	t.Parallel()
	config := textConfig("checkout-cta", controlText)
	config["variations"] = []any{map[string]any{"name": "control", "value": controlText}}
	client := newReadyClientServing(t, config)

	_, details := client.StringValueDetails("checkout-cta", "fallback", identifiedUser)

	require.Nil(t, details.Variation)
}

func TestAVariationIDTheConfigDoesNotListCarriesNoVariation(t *testing.T) {
	t.Parallel()
	rule := treatmentRule()
	rule["variationId"] = "00000000-0000-4000-8000-000000000000"
	client := newReadyClientServing(t, experimentConfig("checkout-cta", rule))

	value, details := client.StringValueDetails("checkout-cta", "fallback", identifiedUser)

	require.Equal(t, treatmentText, value)
	require.Equal(t, foundMatch(treatmentValueID), details)
}

func TestAReadThatFellBackToTheInCodeDefaultValueCarriesNoVariation(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, experimentConfig("checkout-cta", evenExperimentRollout()))

	value, details := client.BoolValueDetails("checkout-cta", true, userInTheTreatmentShare)

	require.True(t, value)
	require.Equal(t, usedDefault(configdirector.EvaluationReasonInvalidBoolean), details)
}

func TestAJSONReadServedByARolloutCarriesTheVariationOfItsShare(t *testing.T) {
	t.Parallel()
	share := treatmentShare()
	share["value"] = `{"max":20}`
	config := withField(experimentConfig("limits", percentageRule(rolloutRuleID, controlShare(), share)), "type", "json")
	client := newReadyClientServing(t, config)

	target := map[string]any{}
	details, err := client.JSONValueDetails("limits", &target, userInTheTreatmentShare)

	require.NoError(t, err)
	require.Equal(t, map[string]any{"max": 20.0}, target)
	require.Equal(t, &treatmentVariation, details.Variation)
}

func TestAnEvaluatedHandlerReceivesTheVariation(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, experimentConfig("checkout-cta", evenExperimentRollout()))
	evaluations := newRecorder[configdirector.Evaluation]()
	client.OnConfigEvaluated(evaluations.record)

	client.StringValue("checkout-cta", "fallback", userInTheTreatmentShare)

	require.Equal(t, &treatmentVariation, evaluations.next(t).Variation)
}

func TestAnEvaluatedHandlerOfAWatchThatFiresReceivesTheVariation(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, fullPayload(experimentConfig("checkout-cta")))
	evaluations := newRecorder[configdirector.Evaluation]()
	client.OnConfigEvaluated(evaluations.record)
	client.WatchString("checkout-cta", "fallback", userInTheTreatmentShare, func(string) {})

	server.send(fullPayload(experimentConfig("checkout-cta", evenExperimentRollout())).json())

	require.Equal(t, &treatmentVariation, evaluations.next(t).Variation)
}

func TestAReadWithoutAnIDSkipsTheRolloutAndIsServedTheDefaultTargetingRuleValue(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, experimentConfig("checkout-cta", evenExperimentRollout()))

	value, details := client.StringValueDetails("checkout-cta", "fallback", noUser)

	require.Equal(t, controlText, value)
	require.Equal(t, skippedRollout(), details)
}

func TestAContextOfAnotherKindWithoutAnIDSkipsTheRollout(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, experimentConfig("checkout-cta", evenExperimentRollout()))

	_, details := client.StringValueDetails("checkout-cta", "fallback", configdirector.UserContext{Kind: "device"})

	require.Equal(t, skippedRollout(), details)
}

func TestAContextIgnoredForAnInvalidKindSkipsTheRollout(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, experimentConfig("checkout-cta", evenExperimentRollout()))

	_, details := client.StringValueDetails("checkout-cta", "fallback", configdirector.UserContext{ID: "u582", Kind: "Not Valid"})

	require.Equal(t, skippedRollout(), details)
}

func TestASkippedRolloutWithoutVariationsHasTheSkippedReason(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, evenSplitConfig("split"))

	value, details := client.StringValueDetails("split", "fallback", noUser)

	require.Equal(t, "NONE", value)
	require.Equal(t, configdirector.EvaluationDetails{
		Reason:  configdirector.EvaluationReasonRolloutSkippedNoIdentifier,
		ValueID: "value-split-default",
	}, details)
}

func TestAContextWithoutAnIDIsServedTheRuleAfterASkippedRollout(t *testing.T) {
	t.Parallel()
	later := treatmentRule()
	later["order"] = 1
	client := newReadyClientServing(t, experimentConfig("checkout-cta", evenExperimentRollout(), later))

	value, details := client.StringValueDetails("checkout-cta", "fallback", noUser)

	require.Equal(t, treatmentText, value)
	require.Equal(t, configdirector.EvaluationDetails{
		Reason:    configdirector.EvaluationReasonFoundMatch,
		ValueID:   treatmentValueID,
		Variation: &treatmentVariation,
	}, details)
}

func TestAContextWithoutAnIDMatchingNoRuleAfterASkippedRolloutKeepsTheSkippedReason(t *testing.T) {
	t.Parallel()
	later := treatmentRule(attributeCondition("traits", "/plan", "equals", "text", "pro"))
	later["order"] = 1
	client := newReadyClientServing(t, experimentConfig("checkout-cta", evenExperimentRollout(), later))

	value, details := client.StringValueDetails("checkout-cta", "fallback", configdirector.UserContext{Traits: map[string]any{"plan": "free"}})

	require.Equal(t, controlText, value)
	require.Equal(t, skippedRollout(), details)
}

func TestARuleWithASingleValueStillServesAContextWithoutAnID(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, experimentConfig("checkout-cta", treatmentRule()))

	value, details := client.StringValueDetails("checkout-cta", "fallback", noUser)

	require.Equal(t, treatmentText, value)
	require.Equal(t, configdirector.EvaluationReasonFoundMatch, details.Reason)
}

func TestASkippedRolloutWhoseDefaultTargetingRuleValueCannotBeReadFallsBack(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, experimentConfig("checkout-cta", evenExperimentRollout()))

	value, details := client.IntValueDetails("checkout-cta", 7, noUser)

	require.Equal(t, int64(7), value)
	require.Equal(t, usedDefault(configdirector.EvaluationReasonInvalidNumber), details)
}

func TestASkippedRolloutWhoseDefaultTargetingRuleValueIsEmptyHasNoValue(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, configOf("checkout-cta", "string", "", evenExperimentRollout()))

	value, details := client.StringValueDetails("checkout-cta", "fallback", noUser)

	require.Equal(t, "fallback", value)
	require.Equal(t, usedDefault(configdirector.EvaluationReasonValueMissing), details)
}

func TestAJSONReadWithoutAnIDHasTheSkippedReason(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, configOf("limits", "json", `{"max":1}`, evenExperimentRollout()))

	target := map[string]any{}
	details, err := client.JSONValueDetails("limits", &target, noUser)

	require.NoError(t, err)
	require.Equal(t, map[string]any{"max": 1.0}, target)
	require.Equal(t, configdirector.EvaluationReasonRolloutSkippedNoIdentifier, details.Reason)
}

func TestAWatchWithoutAnIDFiresWithTheDefaultTargetingRuleValueAndTheSkippedReason(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, fullPayload(experimentConfig("checkout-cta")))
	values := newRecorder[string]()
	evaluations := newRecorder[configdirector.Evaluation]()
	client.OnConfigEvaluated(evaluations.record)
	client.WatchString("checkout-cta", "fallback", noUser, values.record)

	server.send(fullPayload(experimentConfig("checkout-cta", evenExperimentRollout())).json())

	require.Equal(t, controlText, values.next(t))
	require.Equal(t, configdirector.EvaluationReasonRolloutSkippedNoIdentifier, evaluations.next(t).Reason)
}

func TestARolloutWhosePayloadHasNoShareListFallsThroughForAContextWithoutAnID(t *testing.T) {
	t.Parallel()
	later := treatmentRule()
	later["order"] = 1
	client := newReadyClientServing(t, experimentConfig("checkout-cta", percentageRule(rolloutRuleID), later))

	value, details := client.StringValueDetails("checkout-cta", "fallback", noUser)

	require.Equal(t, treatmentText, value)
	require.Equal(t, configdirector.EvaluationReasonFoundMatch, details.Reason)
}

func TestARolloutWhosePayloadHasAnEmptyShareListIsSkippedForAContextWithoutAnIDAndTheNextRuleIsTried(t *testing.T) {
	t.Parallel()
	later := treatmentRule()
	later["order"] = 1
	rollout := percentageRule(rolloutRuleID)
	rollout["percentages"] = []any{}
	client := newReadyClientServing(t, experimentConfig("checkout-cta", rollout, later))

	value, details := client.StringValueDetails("checkout-cta", "fallback", noUser)

	require.Equal(t, treatmentText, value)
	require.Equal(t, configdirector.EvaluationReasonFoundMatch, details.Reason)
}

func TestAllConfigsCarriesTheVariationRuleAndShareOfARollout(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, experimentConfig("checkout-cta", evenExperimentRollout()))

	states := client.AllConfigs(userInTheTreatmentShare)

	require.Equal(t, []configdirector.ConfigState{{
		ID:        experimentConfigID,
		Key:       "checkout-cta",
		Type:      configdirector.ConfigTypeString,
		Value:     treatmentText,
		ValueID:   treatmentValueID,
		Variation: &treatmentVariation,
		RuleID:    rolloutRuleID,
		ShareID:   treatmentShareID,
	}}, states)
}

func TestAllConfigsCarriesOnlyTheVariationOfARuleWithASingleValue(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, experimentConfig("checkout-cta", treatmentRule()))

	states := client.AllConfigs(identifiedUser)

	require.Equal(t, []configdirector.ConfigState{{
		ID:        experimentConfigID,
		Key:       "checkout-cta",
		Type:      configdirector.ConfigTypeString,
		Value:     treatmentText,
		ValueID:   treatmentValueID,
		Variation: &treatmentVariation,
	}}, states)
}

func TestAllConfigsOfAContextWithoutAnIDCarriesTheDefaultTargetingRuleValue(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, experimentConfig("checkout-cta", evenExperimentRollout()))

	states := client.AllConfigs(noUser)

	require.Equal(t, []configdirector.ConfigState{{
		ID:        experimentConfigID,
		Key:       "checkout-cta",
		Type:      configdirector.ConfigTypeString,
		Value:     controlText,
		ValueID:   controlValueID,
		Variation: &controlVariation,
	}}, states)
}

func TestAllConfigsOfAConfigWithoutVariationsCarriesNoVariationRuleOrShare(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, textConfig("welcome-message", "Welcome"))

	states := client.AllConfigs(identifiedUser)

	require.Equal(t, []configdirector.ConfigState{{
		ID:      "config-welcome-message",
		Key:     "welcome-message",
		Type:    configdirector.ConfigTypeString,
		Value:   "Welcome",
		ValueID: "value-welcome-message-default",
	}}, states)
}
