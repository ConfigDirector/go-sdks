package configdirector

// UserContext is the user a config is evaluated for. Targeting rules match on its fields, and
// percentage rollouts bucket by ID. It is named UserContext rather than Context because Go's own
// context.Context takes the short name.
type UserContext struct {
	// ID uniquely identifies the user. It decides which bucket of a percentage rollout the user
	// falls in, so changing it can move the user to a different bucket. Leave it empty for a user
	// without a stable identity.
	ID string

	// Name is the user's display name. It is shown in the ConfigDirector dashboard and usable in
	// targeting rules.
	Name string

	// Traits are arbitrary attributes of the user. They are shown in the ConfigDirector dashboard
	// and usable in targeting rules, which address a trait with a JSON pointer such as /plan.
	Traits map[string]any

	// Anonymous marks a context that is evaluated but never stored or shown in the dashboard.
	Anonymous bool
}

// EvaluationReason says why a read produced the value it did.
type EvaluationReason string

const (
	// EvaluationReasonFoundMatch means the config was found and its value read as the requested
	// type.
	EvaluationReasonFoundMatch EvaluationReason = "found-match"

	// EvaluationReasonConfigStateMissing means the client holds no config with the requested key.
	EvaluationReasonConfigStateMissing EvaluationReason = "config-state-missing"

	// EvaluationReasonClientNotReady means the client has not received config state yet, or has
	// been closed.
	EvaluationReasonClientNotReady EvaluationReason = "client-not-ready"

	// EvaluationReasonValueMissing means the config evaluated to an empty value.
	EvaluationReasonValueMissing EvaluationReason = "value-missing"

	// EvaluationReasonInvalidNumber means the value could not be read as the requested number
	// type.
	EvaluationReasonInvalidNumber EvaluationReason = "invalid-number"

	// EvaluationReasonInvalidJSON means the value could not be decoded into the requested target.
	EvaluationReasonInvalidJSON EvaluationReason = "invalid-json"

	// EvaluationReasonInvalidBoolean means the value is neither true nor false.
	EvaluationReasonInvalidBoolean EvaluationReason = "invalid-boolean"

	// EvaluationReasonTypeMismatch means a boolean, integer or float config was read as a string.
	EvaluationReasonTypeMismatch EvaluationReason = "type-mismatch"
)

// EvaluationDetails describes how a read arrived at its value.
type EvaluationDetails struct {
	// Reason says why the read produced the value it did.
	Reason EvaluationReason

	// ValueID identifies the served value in ConfigDirector. Empty when the in-code default value
	// was returned.
	ValueID string

	// UsedDefault is true when the read returned the in-code default value.
	UsedDefault bool
}

// ConfigType is the type a config was declared with in the ConfigDirector dashboard.
type ConfigType string

const (
	// ConfigTypeCustom is a config whose value is free text with a custom validation.
	ConfigTypeCustom ConfigType = "custom"

	// ConfigTypeBoolean is a config holding true or false.
	ConfigTypeBoolean ConfigType = "boolean"

	// ConfigTypeString is a config holding text.
	ConfigTypeString ConfigType = "string"

	// ConfigTypeInteger is a config holding a whole number.
	ConfigTypeInteger ConfigType = "integer"

	// ConfigTypeFloat is a config holding a number.
	ConfigTypeFloat ConfigType = "float"

	// ConfigTypeEnum is a config holding one of a fixed set of values.
	ConfigTypeEnum ConfigType = "enum"

	// ConfigTypeURL is a config holding a URL.
	ConfigTypeURL ConfigType = "url"

	// ConfigTypeJSON is a config holding a JSON document.
	ConfigTypeJSON ConfigType = "json"
)

// ConfigState is the evaluated state of one config before its value is read as a type.
type ConfigState struct {
	// ID identifies the config in ConfigDirector.
	ID string

	// Key is the config's key.
	Key string

	// Type is the type the config was declared with.
	Type ConfigType

	// Value is the value the targeting rules served, as text.
	Value string

	// ValueID identifies the served value in ConfigDirector.
	ValueID string
}

type configEvaluation struct {
	key         string
	value       any
	usedDefault bool
	reason      EvaluationReason
	valueID     string
	userContext UserContext
}

func servedEvaluation(key string, value any, valueID string, userContext UserContext) configEvaluation {
	return configEvaluation{key: key, value: value, reason: EvaluationReasonFoundMatch, valueID: valueID, userContext: userContext}
}

func defaultEvaluation(key string, reason EvaluationReason, userContext UserContext) configEvaluation {
	return configEvaluation{key: key, usedDefault: true, reason: reason, userContext: userContext}
}

func (evaluated configEvaluation) details() EvaluationDetails {
	return EvaluationDetails{
		Reason:      evaluated.reason,
		ValueID:     evaluated.valueID,
		UsedDefault: evaluated.usedDefault,
	}
}
