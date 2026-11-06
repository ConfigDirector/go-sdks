package configdirector_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/stretchr/testify/require"
)

type planName string

type tier uint8

func (tier) String() string {
	return "a rendering, not the number"
}

func evaluateWithoutError(t *testing.T, client *configdirector.Client, key string, defaultValue any, userContext configdirector.UserContext) configdirector.Evaluation {
	t.Helper()
	evaluated, err := client.Evaluate(key, defaultValue, userContext)
	require.NoError(t, err)
	return evaluated
}

func TestEvaluateReturnsTheEvaluationOfABooleanDefault(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	evaluated := evaluateWithoutError(t, client, "new-checkout", false, identifiedUser)

	require.Equal(t, configdirector.Evaluation{
		Key:         "new-checkout",
		Value:       true,
		Reason:      configdirector.EvaluationReasonFoundMatch,
		ValueID:     "value-new-checkout-default",
		UserContext: identifiedUser,
	}, evaluated)
}

func TestEvaluateReadsTextForAStringDefault(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	evaluated := evaluateWithoutError(t, client, "welcome-message", "fallback", noUser)

	require.Equal(t, "Welcome", evaluated.Value)
}

func TestEvaluateReturnsAnIntForAnIntDefault(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	evaluated := evaluateWithoutError(t, client, "max-items", 10, noUser)

	require.Equal(t, 26, evaluated.Value)
}

func TestEvaluateReturnsAnInt64ForAnInt64Default(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	evaluated := evaluateWithoutError(t, client, "max-items", int64(10), noUser)

	require.Equal(t, int64(26), evaluated.Value)
}

func TestEvaluateReturnsAUintForAUintDefault(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	evaluated := evaluateWithoutError(t, client, "max-items", uint(10), noUser)

	require.Equal(t, uint(26), evaluated.Value)
}

func TestEvaluateReturnsAUint64AboveTheInt64Range(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, intConfig("big", "18446744073709551615"))

	evaluated := evaluateWithoutError(t, client, "big", uint64(7), noUser)

	require.Equal(t, uint64(18446744073709551615), evaluated.Value)
	require.Equal(t, configdirector.EvaluationReasonFoundMatch, evaluated.Reason)
}

func TestEvaluateReadsAWholeNumberWrittenWithADecimalPointForAUintDefault(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	evaluated := evaluateWithoutError(t, client, "retry-limit", uint(10), noUser)

	require.Equal(t, uint(26), evaluated.Value)
}

func TestEvaluateReadsAWholeNumberWrittenWithADecimalPoint(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	evaluated := evaluateWithoutError(t, client, "retry-limit", 10, noUser)

	require.Equal(t, 26, evaluated.Value)
}

func TestEvaluateReturnsAFloat64ForAFloat64Default(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	evaluated := evaluateWithoutError(t, client, "price-multiplier", 2.5, noUser)

	require.Equal(t, 1.5, evaluated.Value)
}

func TestEvaluateReturnsAFloat32ForAFloat32Default(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	evaluated := evaluateWithoutError(t, client, "price-multiplier", float32(2.5), noUser)

	require.Equal(t, float32(1.5), evaluated.Value)
}

func TestEvaluateReturnsTheNamedTypeOfANamedStringDefault(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	evaluated := evaluateWithoutError(t, client, "welcome-message", planName("free"), noUser)

	require.Equal(t, planName("Welcome"), evaluated.Value)
}

func TestEvaluateReturnsADurationForADurationDefault(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	evaluated := evaluateWithoutError(t, client, "max-items", time.Second, noUser)

	require.Equal(t, time.Duration(26), evaluated.Value)
}

func TestEvaluateFallsBackWhenAWholeNumberDoesNotFitAnInt8Default(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, intConfig("big", "300"))

	evaluated := evaluateWithoutError(t, client, "big", int8(1), noUser)

	require.Equal(t, int8(1), evaluated.Value)
	require.True(t, evaluated.UsedDefault)
	require.Equal(t, configdirector.EvaluationReasonInvalidNumber, evaluated.Reason)
}

func TestEvaluateFallsBackWhenANegativeNumberIsReadWithAUintDefault(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, intConfig("below-zero", "-1"))

	evaluated := evaluateWithoutError(t, client, "below-zero", uint(3), noUser)

	require.Equal(t, uint(3), evaluated.Value)
	require.Equal(t, configdirector.EvaluationReasonInvalidNumber, evaluated.Reason)
}

func TestEvaluateFallsBackWhenAWholeNumberDoesNotFitAUint8Default(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, intConfig("big", "300"))

	evaluated := evaluateWithoutError(t, client, "big", uint8(3), noUser)

	require.Equal(t, uint8(3), evaluated.Value)
	require.Equal(t, configdirector.EvaluationReasonInvalidNumber, evaluated.Reason)
}

func TestEvaluateFallsBackWhenANumberDoesNotFitAFloat32Default(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, floatConfig("huge", "1e300"))

	evaluated := evaluateWithoutError(t, client, "huge", float32(1), noUser)

	require.Equal(t, float32(1), evaluated.Value)
	require.Equal(t, configdirector.EvaluationReasonInvalidNumber, evaluated.Reason)
}

func TestEvaluateFallsBackWhenTheValueIsNotABoolean(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	evaluated := evaluateWithoutError(t, client, "not-a-boolean", true, noUser)

	require.Equal(t, true, evaluated.Value)
	require.Equal(t, configdirector.EvaluationReasonInvalidBoolean, evaluated.Reason)
}

func TestEvaluateFallsBackWhenABooleanConfigIsReadWithAStringDefault(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	evaluated := evaluateWithoutError(t, client, "new-checkout", "fallback", noUser)

	require.Equal(t, "fallback", evaluated.Value)
	require.Equal(t, configdirector.EvaluationReasonTypeMismatch, evaluated.Reason)
}

func TestEvaluateDecodesAJSONObjectIntoAMapLeavingTheDefaultAlone(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)
	defaultValue := map[string]any{"max": 1.0, "fallback": true}

	evaluated := evaluateWithoutError(t, client, "limits", defaultValue, noUser)

	require.Equal(t, map[string]any{"max": 20.0, "min": 1.0}, evaluated.Value)
	require.Equal(t, map[string]any{"max": 1.0, "fallback": true}, defaultValue)
}

func TestEvaluateDecodesAJSONObjectIntoAStructOfTheDefaultsType(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	evaluated := evaluateWithoutError(t, client, "limits", limits{Max: 5}, noUser)

	require.Equal(t, limits{Max: 20, Min: 1}, evaluated.Value)
}

func TestEvaluateDecodesAJSONArrayIntoASliceOfTheDefaultsType(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	evaluated := evaluateWithoutError(t, client, "allowed-regions", []string{"mx"}, noUser)

	require.Equal(t, []string{"eu", "us"}, evaluated.Value)
}

func TestEvaluateDecodesAJSONArrayIntoAnArrayOfTheDefaultsType(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	evaluated := evaluateWithoutError(t, client, "allowed-regions", [2]string{"mx", "ca"}, noUser)

	require.Equal(t, [2]string{"eu", "us"}, evaluated.Value)
}

func TestEvaluateFallsBackWhenAJSONArrayIsReadWithAMapDefault(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)
	defaultValue := map[string]any{"fallback": true}

	evaluated := evaluateWithoutError(t, client, "allowed-regions", defaultValue, noUser)

	require.Equal(t, defaultValue, evaluated.Value)
	require.True(t, evaluated.UsedDefault)
	require.Equal(t, configdirector.EvaluationReasonInvalidJSON, evaluated.Reason)
}

func TestEvaluateFallsBackWhenAJSONObjectIsReadWithASliceDefault(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	evaluated := evaluateWithoutError(t, client, "limits", []string{"mx"}, noUser)

	require.Equal(t, []string{"mx"}, evaluated.Value)
	require.Equal(t, configdirector.EvaluationReasonInvalidJSON, evaluated.Reason)
}

func TestEvaluateFallsBackWhenAJSONObjectIsExpectedAndTheConfigHoldsNull(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, jsonConfig("nothing", "null"))

	evaluated := evaluateWithoutError(t, client, "nothing", map[string]any{"fallback": true}, noUser)

	require.Equal(t, map[string]any{"fallback": true}, evaluated.Value)
	require.Equal(t, configdirector.EvaluationReasonInvalidJSON, evaluated.Reason)
}

func TestEvaluateFallsBackWhenAJSONTextIsReadWithAStructThatDecodesItself(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, jsonConfig("launch", `"2026-01-01T00:00:00Z"`))
	launch := time.Date(2027, time.June, 1, 0, 0, 0, 0, time.UTC)

	evaluated := evaluateWithoutError(t, client, "launch", launch, noUser)

	require.Equal(t, launch, evaluated.Value)
	require.Equal(t, configdirector.EvaluationReasonInvalidJSON, evaluated.Reason)
}

func TestEvaluateFallsBackWhenAJSONObjectIsReadWithARawMessageDefault(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	evaluated := evaluateWithoutError(t, client, "limits", json.RawMessage(`[]`), noUser)

	require.Equal(t, json.RawMessage(`[]`), evaluated.Value)
	require.Equal(t, configdirector.EvaluationReasonInvalidJSON, evaluated.Reason)
}

func TestEvaluateDecodesAJSONArrayIntoARawMessageDefault(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	evaluated := evaluateWithoutError(t, client, "allowed-regions", json.RawMessage(`[]`), noUser)

	require.Equal(t, json.RawMessage(`["eu","us"]`), evaluated.Value)
	require.Equal(t, configdirector.EvaluationReasonFoundMatch, evaluated.Reason)
}

func TestEvaluateFallsBackWhenBase64TextIsReadWithAByteSliceDefault(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, jsonConfig("secret", `"aGk="`))

	evaluated := evaluateWithoutError(t, client, "secret", []byte("fallback"), noUser)

	require.Equal(t, []byte("fallback"), evaluated.Value)
	require.Equal(t, configdirector.EvaluationReasonInvalidJSON, evaluated.Reason)
}

func TestEvaluateDecodesAJSONObjectWithLeadingWhitespaceIntoAMap(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, jsonConfig("limits", "\n  {\"max\":20}"))

	evaluated := evaluateWithoutError(t, client, "limits", map[string]any{}, noUser)

	require.Equal(t, map[string]any{"max": 20.0}, evaluated.Value)
	require.Equal(t, configdirector.EvaluationReasonFoundMatch, evaluated.Reason)
}

func TestEvaluateFallsBackWhenTheDocumentIsNotJSON(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	evaluated := evaluateWithoutError(t, client, "broken-json", limits{Max: 5}, noUser)

	require.Equal(t, limits{Max: 5}, evaluated.Value)
	require.Equal(t, configdirector.EvaluationReasonInvalidJSON, evaluated.Reason)
}

func TestEvaluateOfAnUnknownKeyReturnsTheDefaultWithTheReason(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	evaluated := evaluateWithoutError(t, client, "unknown-key", "fallback", noUser)

	require.Equal(t, configdirector.Evaluation{
		Key:         "unknown-key",
		Value:       "fallback",
		UsedDefault: true,
		Reason:      configdirector.EvaluationReasonConfigStateMissing,
	}, evaluated)
}

func TestEvaluateOnAClientThatIsNotReadyReturnsTheDefaultWithTheReason(t *testing.T) {
	t.Parallel()
	client := newClientAgainst(t, startSDKServer(t))

	evaluated := evaluateWithoutError(t, client, "new-checkout", false, noUser)

	require.Equal(t, false, evaluated.Value)
	require.True(t, evaluated.UsedDefault)
	require.Equal(t, configdirector.EvaluationReasonClientNotReady, evaluated.Reason)
}

func TestEvaluateOfAnEmptyValueReturnsTheDefaultWithTheReason(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	evaluated := evaluateWithoutError(t, client, "empty-message", "fallback", noUser)

	require.Equal(t, "fallback", evaluated.Value)
	require.Equal(t, configdirector.EvaluationReasonValueMissing, evaluated.Reason)
}

func TestEvaluateReturnsTheVariationOfTheShareThatServedTheValue(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, experimentConfig("checkout-cta", evenExperimentRollout()))

	evaluated := evaluateWithoutError(t, client, "checkout-cta", "fallback", userInTheTreatmentShare)

	require.Equal(t, configdirector.Evaluation{
		Key:         "checkout-cta",
		Value:       treatmentText,
		Reason:      configdirector.EvaluationReasonFoundMatch,
		ValueID:     treatmentValueID,
		UserContext: userInTheTreatmentShare,
		Variation:   &treatmentVariation,
	}, evaluated)
}

func TestEvaluateWithoutAnIDSkipsTheRollout(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, experimentConfig("checkout-cta", evenExperimentRollout()))

	evaluated := evaluateWithoutError(t, client, "checkout-cta", "fallback", noUser)

	require.Equal(t, configdirector.Evaluation{
		Key:       "checkout-cta",
		Value:     controlText,
		Reason:    configdirector.EvaluationReasonRolloutSkippedNoIdentifier,
		ValueID:   controlValueID,
		Variation: &controlVariation,
	}, evaluated)
}

func TestEvaluateWithAnInvalidKindEvaluatesForNoContext(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	evaluated := evaluateWithoutError(t, client, "greeting", "fallback", configdirector.UserContext{ID: "u-pro", Kind: "Not Valid"})

	require.Equal(t, "Hello", evaluated.Value)
	require.Equal(t, noUser, evaluated.UserContext)
}

func TestEvaluateReachesTheEvaluatedHandlersWithWhatItReturns(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, experimentConfig("checkout-cta", evenExperimentRollout()))
	evaluations := newRecorder[configdirector.Evaluation]()
	client.OnConfigEvaluated(evaluations.record)

	evaluateWithoutError(t, client, "checkout-cta", "fallback", userInTheTreatmentShare)

	require.Equal(t, configdirector.Evaluation{
		Key:         "checkout-cta",
		Value:       treatmentText,
		Reason:      configdirector.EvaluationReasonFoundMatch,
		ValueID:     treatmentValueID,
		UserContext: userInTheTreatmentShare,
		Variation:   &treatmentVariation,
	}, evaluations.next(t))
}

func TestEvaluateIsCountedInTelemetryWithTheRuleShareAndVariation(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, fullPayload(experimentConfig("checkout-cta", evenExperimentRollout())))

	evaluateWithoutError(t, client, "checkout-cta", "fallback", userInTheTreatmentShare)

	require.Equal(t, evaluatedConfigEvent{
		ContextID:        new("u582"),
		ContextKind:      new("user"),
		Key:              "checkout-cta",
		Type:             new("string"),
		DefaultValue:     inline("fallback"),
		RequestedType:    "string",
		EvaluatedValue:   inline(treatmentText),
		EvaluatedValueID: new(treatmentValueID),
		UsedDefault:      false,
		EvaluationReason: "found-match",
		RuleID:           new(rolloutRuleID),
		ShareID:          new(treatmentShareID),
		VariationID:      new(treatmentVariationID),
	}, onlyEventOf(t, reportAfterClosing(t, client, server)).Event)
}

func TestEvaluateReportsAnIntDefaultAsANumberOfItsType(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	evaluateWithoutError(t, client, "max-items", 10, noUser)

	event := onlyEventOf(t, reportAfterClosing(t, client, server)).Event
	require.Equal(t, "int", event.RequestedType)
	require.Equal(t, inline("10"), event.DefaultValue)
	require.Equal(t, inline("26"), event.EvaluatedValue)
}

func TestEvaluateReportsADurationDefaultAsItsNumberRatherThanItsRendering(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	evaluateWithoutError(t, client, "max-items", time.Second, noUser)

	event := onlyEventOf(t, reportAfterClosing(t, client, server)).Event
	require.Equal(t, "Duration", event.RequestedType)
	require.Equal(t, inline("1000000000"), event.DefaultValue)
	require.Equal(t, inline("26"), event.EvaluatedValue)
}

func TestEvaluateReportsAFloat32DefaultWithItsShortestSpelling(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	evaluateWithoutError(t, client, "price-multiplier", float32(0.1), noUser)

	event := onlyEventOf(t, reportAfterClosing(t, client, server)).Event
	require.Equal(t, "float32", event.RequestedType)
	require.Equal(t, inline("0.1"), event.DefaultValue)
	require.Equal(t, inline("1.5"), event.EvaluatedValue)
}

func TestEvaluateReportsAUintDefaultAsANumberOfItsType(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	evaluateWithoutError(t, client, "max-items", uint(10), noUser)

	event := onlyEventOf(t, reportAfterClosing(t, client, server)).Event
	require.Equal(t, "uint", event.RequestedType)
	require.Equal(t, inline("10"), event.DefaultValue)
}

func TestEvaluateReportsAUintDefaultAsItsNumberRatherThanItsRendering(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	evaluateWithoutError(t, client, "max-items", tier(10), noUser)

	event := onlyEventOf(t, reportAfterClosing(t, client, server)).Event
	require.Equal(t, "tier", event.RequestedType)
	require.Equal(t, inline("10"), event.DefaultValue)
	require.Equal(t, inline("26"), event.EvaluatedValue)
}

func TestEvaluateReportsAMapDefaultOfAnUnknownKeyByTheDigestOfItsDocument(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	evaluateWithoutError(t, client, "unknown-key", map[string]any{"on": true}, noUser)

	event := onlyEventOf(t, reportAfterClosing(t, client, server)).Event
	require.Equal(t, "map[string]interface {}", event.RequestedType)
	require.Equal(t, byID("2rnhxPOGf2H7CuV9sivHrq"), event.DefaultValue)
	require.Equal(t, byID("2rnhxPOGf2H7CuV9sivHrq"), event.EvaluatedValue)
}

func TestEvaluateReportsAJSONReadByTheValueIDAndTheDigestOfItsDefault(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	evaluateWithoutError(t, client, "limits", limits{Max: 5, Min: 1}, noUser)

	event := onlyEventOf(t, reportAfterClosing(t, client, server)).Event
	require.Equal(t, "limits", event.RequestedType)
	require.Equal(t, byID("value-limits-default"), event.EvaluatedValue)
	require.Equal(t, byID("5zh2NOzZcJmKjjBJ5XJBzx"), event.DefaultValue)
}

func TestEvaluateRefusesABlankKeyAndReportsNothing(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())
	evaluations := newRecorder[configdirector.Evaluation]()
	client.OnConfigEvaluated(evaluations.record)

	evaluated, err := client.Evaluate("  ", "fallback", noUser)

	require.Error(t, err)
	require.Equal(t, configdirector.Evaluation{}, evaluated)
	require.Empty(t, evaluations.received())
	require.NoError(t, client.Close())
	expectNoTelemetry(t, server, 100*time.Millisecond)
}

func TestEvaluateRefusesANilDefaultAndReportsNothing(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())
	evaluations := newRecorder[configdirector.Evaluation]()
	client.OnConfigEvaluated(evaluations.record)

	evaluated, err := client.Evaluate("new-checkout", nil, noUser)

	require.Error(t, err)
	require.Equal(t, configdirector.Evaluation{}, evaluated)
	require.Empty(t, evaluations.received())
	require.NoError(t, client.Close())
	expectNoTelemetry(t, server, 100*time.Millisecond)
}

func TestEvaluateRefusesAPointerDefault(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	_, err := client.Evaluate("limits", &limits{}, noUser)

	require.Error(t, err)
}

func TestEvaluateRefusesAChannelDefault(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	_, err := client.Evaluate("limits", make(chan int), noUser)

	require.Error(t, err)
}

func TestEvaluateRefusesAFuncDefault(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	_, err := client.Evaluate("limits", func() {}, noUser)

	require.Error(t, err)
}
