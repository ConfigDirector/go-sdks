package configdirectoropenfeature

import (
	"testing"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/open-feature/go-sdk/openfeature"
	"github.com/stretchr/testify/require"
)

func servedWith(reason configdirector.EvaluationReason) configdirector.EvaluationDetails {
	return configdirector.EvaluationDetails{Reason: reason, ValueID: "value-1"}
}

func fellBackWith(reason configdirector.EvaluationReason) configdirector.EvaluationDetails {
	return configdirector.EvaluationDetails{Reason: reason, UsedDefault: true}
}

func TestAMatchIsATargetingMatchWithTheValueIDAsItsVariant(t *testing.T) {
	t.Parallel()

	detail := resolutionDetailFor("greeting", servedWith(configdirector.EvaluationReasonFoundMatch))

	require.Equal(t, openfeature.ProviderResolutionDetail{Reason: openfeature.TargetingMatchReason, Variant: "value-1"}, detail)
}

func TestAConfigWithoutAValueResolvesWithTheDefaultReasonAndNoError(t *testing.T) {
	t.Parallel()

	detail := resolutionDetailFor("greeting", fellBackWith(configdirector.EvaluationReasonValueMissing))

	require.Equal(t, openfeature.ProviderResolutionDetail{Reason: openfeature.DefaultReason}, detail)
}

func TestAMissingConfigIsAFlagNotFoundError(t *testing.T) {
	t.Parallel()

	detail := resolutionDetailFor("greeting", fellBackWith(configdirector.EvaluationReasonConfigStateMissing))

	require.Equal(t, openfeature.ErrorReason, detail.Reason)
	require.Equal(t, openfeature.FlagNotFoundCode, detail.ResolutionDetail().ErrorCode)
	require.Contains(t, detail.ResolutionDetail().ErrorMessage, "greeting")
	require.Empty(t, detail.Variant)
}

func TestAClientWithoutConfigStateIsAProviderNotReadyError(t *testing.T) {
	t.Parallel()

	detail := resolutionDetailFor("greeting", fellBackWith(configdirector.EvaluationReasonClientNotReady))

	require.Equal(t, openfeature.ErrorReason, detail.Reason)
	require.Equal(t, openfeature.ProviderNotReadyCode, detail.ResolutionDetail().ErrorCode)
	require.NotEmpty(t, detail.ResolutionDetail().ErrorMessage)
}

func TestATypeMismatchIsATypeMismatchError(t *testing.T) {
	t.Parallel()

	detail := resolutionDetailFor("greeting", fellBackWith(configdirector.EvaluationReasonTypeMismatch))

	require.Equal(t, openfeature.ErrorReason, detail.Reason)
	require.Equal(t, openfeature.TypeMismatchCode, detail.ResolutionDetail().ErrorCode)
	require.Contains(t, detail.ResolutionDetail().ErrorMessage, "greeting")
}

func TestAValueThatIsNotANumberIsATypeMismatchError(t *testing.T) {
	t.Parallel()

	detail := resolutionDetailFor("greeting", fellBackWith(configdirector.EvaluationReasonInvalidNumber))

	require.Equal(t, openfeature.TypeMismatchCode, detail.ResolutionDetail().ErrorCode)
}

func TestAValueThatDoesNotDecodeIsATypeMismatchError(t *testing.T) {
	t.Parallel()

	detail := resolutionDetailFor("greeting", fellBackWith(configdirector.EvaluationReasonInvalidJSON))

	require.Equal(t, openfeature.TypeMismatchCode, detail.ResolutionDetail().ErrorCode)
}

func TestAValueThatIsNotABooleanIsATypeMismatchError(t *testing.T) {
	t.Parallel()

	detail := resolutionDetailFor("greeting", fellBackWith(configdirector.EvaluationReasonInvalidBoolean))

	require.Equal(t, openfeature.TypeMismatchCode, detail.ResolutionDetail().ErrorCode)
}

func TestAFallbackForAReasonThisProviderDoesNotKnowIsAGeneralError(t *testing.T) {
	t.Parallel()

	detail := resolutionDetailFor("greeting", fellBackWith("reason-of-a-newer-sdk"))

	require.Equal(t, openfeature.ErrorReason, detail.Reason)
	require.Equal(t, openfeature.GeneralCode, detail.ResolutionDetail().ErrorCode)
	require.Contains(t, detail.ResolutionDetail().ErrorMessage, "reason-of-a-newer-sdk")
}

func TestAServedValueForAReasonThisProviderDoesNotKnowKeepsItsVariant(t *testing.T) {
	t.Parallel()

	detail := resolutionDetailFor("greeting", servedWith("reason-of-a-newer-sdk"))

	require.Equal(t, openfeature.ProviderResolutionDetail{Reason: openfeature.UnknownReason, Variant: "value-1"}, detail)
}
