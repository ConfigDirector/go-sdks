package configdirectoropenfeature_test

import (
	"testing"

	"github.com/open-feature/go-sdk/openfeature"
	"github.com/stretchr/testify/require"
)

const experimentState = `{"payloadVersion":1,"kind":"full","segments":{},"configs":{
	"checkout-cta":{"id":"11111111-1111-4111-8111-111111111111","key":"checkout-cta","type":"string",
		"variations":[
			{"id":"variation-control","name":"control","value":"Start free trial"},
			{"id":"variation-treatment","name":"treatment","value":"Try it free"}
		],
		"target":{"defaultValue":"Start free trial","defaultValueId":"value-control","defaultVariationId":"variation-control","rules":[
			{"id":"rollout","order":0,"type":"percentage","target":"percentage","conditions":[],"percentages":[
				{"id":"share-control","percentage":50,"value":"Start free trial","valueId":"value-control","variationId":"variation-control"},
				{"id":"share-treatment","percentage":50,"value":"Try it free","valueId":"value-treatment","variationId":"variation-treatment"}
			]}
		]}},
	"checkout-layout":{"id":"11111111-1111-4111-8111-111111111111","key":"checkout-layout","type":"json",
		"variations":[
			{"id":"variation-compact","name":"compact","value":"{\"columns\":1}"},
			{"id":"variation-wide","name":"wide","value":"{\"columns\":3}"}
		],
		"target":{"defaultValue":"{\"columns\":1}","defaultValueId":"value-compact","defaultVariationId":"variation-compact","rules":[
			{"id":"rollout","order":0,"type":"percentage","target":"percentage","conditions":[],"percentages":[
				{"id":"share-compact","percentage":50,"value":"{\"columns\":1}","valueId":"value-compact","variationId":"variation-compact"},
				{"id":"share-wide","percentage":50,"value":"{\"columns\":3}","valueId":"value-wide","variationId":"variation-wide"}
			]}
		]}},
	"greeting":{"id":"config-1","key":"greeting","type":"string","variations":[],"target":{"rules":[],"defaultValue":"hello","defaultValueId":"value-1"}}
}}`

func TestAnExperimentResolvesWithTheVariationNameAsTheVariant(t *testing.T) {
	t.Parallel()
	client := registerProvider(t, startSDKServerServing(t, experimentState).options())

	details, err := client.StringValueDetails(t.Context(), "checkout-cta", "fallback", openfeature.NewEvaluationContext("u582", nil))

	require.NoError(t, err)
	require.Equal(t, "Try it free", details.Value)
	require.Equal(t, openfeature.TargetingMatchReason, details.Reason)
	require.Equal(t, "treatment", details.Variant)
}

func TestAnObjectExperimentResolvesWithTheVariationNameAsTheVariant(t *testing.T) {
	t.Parallel()
	client := registerProvider(t, startSDKServerServing(t, experimentState).options())

	details, err := client.ObjectValueDetails(t.Context(), "checkout-layout", map[string]any{}, openfeature.NewEvaluationContext("u582", nil))

	require.NoError(t, err)
	require.Equal(t, map[string]any{"columns": 3.0}, details.Value)
	require.Equal(t, "wide", details.Variant)
}

func TestAFlagWithoutVariationsKeepsTheValueIDAsTheVariant(t *testing.T) {
	t.Parallel()
	client := registerProvider(t, startSDKServerServing(t, experimentState).options())

	details, err := client.StringValueDetails(t.Context(), "greeting", "fallback", openfeature.NewEvaluationContext("u582", nil))

	require.NoError(t, err)
	require.Equal(t, openfeature.TargetingMatchReason, details.Reason)
	require.Equal(t, "value-1", details.Variant)
}

func TestAContextWithoutATargetingKeySkipsTheRolloutAndResolvesWithTheDefaultReason(t *testing.T) {
	t.Parallel()
	client := registerProvider(t, startSDKServerServing(t, experimentState).options())

	details, err := client.StringValueDetails(t.Context(), "checkout-cta", "fallback", openfeature.EvaluationContext{})

	require.NoError(t, err)
	require.Equal(t, "Start free trial", details.Value)
	require.Equal(t, openfeature.DefaultReason, details.Reason)
	require.Equal(t, "control", details.Variant)
	require.Empty(t, details.ErrorCode)
}
