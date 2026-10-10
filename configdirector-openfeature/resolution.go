package configdirectoropenfeature

import (
	"fmt"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/open-feature/go-sdk/openfeature"
)

func resolutionDetailFor(key string, details configdirector.EvaluationDetails) openfeature.ProviderResolutionDetail {
	switch details.Reason {
	case configdirector.EvaluationReasonFoundMatch:
		return openfeature.ProviderResolutionDetail{Reason: openfeature.TargetingMatchReason, Variant: details.ValueID}
	case configdirector.EvaluationReasonValueMissing:
		return openfeature.ProviderResolutionDetail{Reason: openfeature.DefaultReason}
	case configdirector.EvaluationReasonConfigStateMissing:
		return resolutionErrorDetail(openfeature.NewFlagNotFoundResolutionError(fmt.Sprintf("No config with the key %q was found.", key)))
	case configdirector.EvaluationReasonClientNotReady:
		return resolutionErrorDetail(openfeature.NewProviderNotReadyResolutionError("The ConfigDirector client has not received any configs yet."))
	case configdirector.EvaluationReasonTypeMismatch,
		configdirector.EvaluationReasonInvalidNumber,
		configdirector.EvaluationReasonInvalidJSON,
		configdirector.EvaluationReasonInvalidBoolean:
		return resolutionErrorDetail(openfeature.NewTypeMismatchResolutionError(fmt.Sprintf("The value of %q does not match the requested type (%s).", key, details.Reason)))
	default:
		return resolutionDetailForAnUnknownReason(key, details)
	}
}

func resolutionDetailForAnUnknownReason(key string, details configdirector.EvaluationDetails) openfeature.ProviderResolutionDetail {
	if details.UsedDefault {
		return resolutionErrorDetail(openfeature.NewGeneralResolutionError(fmt.Sprintf("The read of %q returned its default value (%s).", key, details.Reason)))
	}
	return openfeature.ProviderResolutionDetail{Reason: openfeature.UnknownReason, Variant: details.ValueID}
}

func resolutionErrorDetail(resolutionError openfeature.ResolutionError) openfeature.ProviderResolutionDetail {
	return openfeature.ProviderResolutionDetail{ResolutionError: resolutionError, Reason: openfeature.ErrorReason}
}
