package telemetry

// Evaluation is one config read as telemetry records it.
type Evaluation struct {
	// Key is the config's key.
	Key string

	// ConfigType is the type the config was declared with; empty when the client held no config
	// for the key.
	ConfigType string

	// DefaultValue is the in-code default value the read was given.
	DefaultValue Value

	// RequestedType names the Go type the read asked for.
	RequestedType string

	// EvaluatedValue is the value the read produced.
	EvaluatedValue Value

	// EvaluatedValueID is the id the SDK server sent with the served value; empty when the
	// in-code default value was returned.
	EvaluatedValueID string

	// UsedDefault is true when the in-code default value was returned.
	UsedDefault bool

	// Reason says why the read produced the value it did.
	Reason string
}

// Context is the user context a config was evaluated for. Only a context with an ID that is not
// anonymous is captured and named on the evaluation.
type Context struct {
	// ID identifies the context among those of its kind; a context without one is not captured.
	ID string

	// Kind is the context's effective kind, never empty.
	Kind string

	// Name is the user's display name.
	Name string

	// Traits are the user's attributes. The map is cloned when the context is captured.
	Traits map[string]any

	// Anonymous marks a context that is never captured nor named on an evaluation.
	Anonymous bool
}

func (context Context) identified() bool {
	return context.ID != "" && !context.Anonymous
}

func (context Context) wire() wireContext {
	return wireContext{ID: context.ID, Kind: context.Kind, Name: context.Name, Traits: context.Traits}
}

func (context Context) identity() contextIdentity {
	return contextIdentity{kind: context.Kind, id: context.ID}
}

type contextIdentity struct {
	kind string
	id   string
}

type wireContext struct {
	ID     string         `json:"id"`
	Kind   string         `json:"kind"`
	Name   string         `json:"name,omitempty"`
	Traits map[string]any `json:"traits,omitempty"`
}

type evaluatedConfigEvent struct {
	contextID   string
	contextKind string
	evaluation  Evaluation
}

func (event evaluatedConfigEvent) wire() wireEvaluatedConfigEvent {
	return wireEvaluatedConfigEvent{
		ContextID:        event.contextID,
		ContextKind:      event.contextKind,
		Key:              event.evaluation.Key,
		Type:             event.evaluation.ConfigType,
		DefaultValue:     event.evaluation.DefaultValue.wire(),
		RequestedType:    event.evaluation.RequestedType,
		EvaluatedValue:   event.evaluation.EvaluatedValue.wire(),
		EvaluatedValueID: event.evaluation.EvaluatedValueID,
		UsedDefault:      event.evaluation.UsedDefault,
		EvaluationReason: event.evaluation.Reason,
	}
}

type wireEvaluatedConfigEvent struct {
	ContextID        string    `json:"contextId,omitempty"`
	ContextKind      string    `json:"contextKind,omitempty"`
	Key              string    `json:"key"`
	Type             string    `json:"type,omitempty"`
	DefaultValue     wireValue `json:"defaultValue"`
	RequestedType    string    `json:"requestedType"`
	EvaluatedValue   wireValue `json:"evaluatedValue"`
	EvaluatedValueID string    `json:"evaluatedValueId,omitempty"`
	UsedDefault      bool      `json:"usedDefault"`
	EvaluationReason string    `json:"evaluationReason"`
}
