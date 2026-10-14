package configdirector

import (
	"log/slog"

	"github.com/ConfigDirector/go-sdks/configdirector/internal/telemetry"
)

const (
	contextKindMaximumLength           = 64
	contextKindSampleMaximumCharacters = 128
)

type contextAdmission struct {
	outcome           contextAdmissionOutcome
	context           UserContext
	contextKindSample string
}

type contextAdmissionOutcome int

const (
	contextAdmitted contextAdmissionOutcome = iota
	contextIgnoredForAnInvalidContextKind
)

func admitContext(supplied UserContext) contextAdmission {
	if isOmittedOrValidContextKind(supplied.Kind) {
		return contextAdmission{outcome: contextAdmitted, context: supplied}
	}
	return contextAdmission{outcome: contextIgnoredForAnInvalidContextKind, contextKindSample: firstCharactersOf(supplied.Kind, contextKindSampleMaximumCharacters)}
}

func isOmittedOrValidContextKind(contextKind string) bool {
	if len(contextKind) > contextKindMaximumLength {
		return false
	}
	for index := range len(contextKind) {
		if !isContextKindCharacter(contextKind[index]) {
			return false
		}
	}
	return true
}

func isContextKindCharacter(character byte) bool {
	return (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' || character == '_'
}

func firstCharactersOf(text string, maximumCharacters int) string {
	characters := 0
	for index := range text {
		if characters == maximumCharacters {
			return text[:index]
		}
		characters++
	}
	return text
}

func (client *Client) contextToEvaluate(supplied UserContext) UserContext {
	admission := admitContext(supplied)
	switch admission.outcome {
	case contextAdmitted:
		return admission.context
	case contextIgnoredForAnInvalidContextKind:
		recording := client.telemetry.RecordInvalidContext(admission.contextKindSample)
		client.logInvalidContext(recording, admission.contextKindSample)
	}
	return UserContext{}
}

func (client *Client) logInvalidContext(recording telemetry.InvalidContextRecording, contextKindSample string) {
	logger := client.settings.logger
	switch recording {
	case telemetry.InvalidContextSampled:
		logger.Error(
			"Ignored a context with an invalid context kind and evaluated as if no context was supplied; a context kind has 1 to 64 characters, using only lowercase letters, digits, '-' and '_'",
			slog.String("contextKind", contextKindSample),
		)
	case telemetry.InvalidContextAlreadySampled:
	case telemetry.FirstInvalidContextBeyondSamples:
		logger.Error(
			"Ignored a context with another invalid context kind and evaluated as if no context was supplied; further invalid context kinds are not logged individually until the next telemetry report",
			slog.Int("loggedContextKinds", telemetry.DistinctContextKindSampleLimit),
		)
	case telemetry.InvalidContextBeyondSamples:
	}
}
