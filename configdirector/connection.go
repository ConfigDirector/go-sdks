package configdirector

import "github.com/ConfigDirector/go-sdks/configdirector/internal/evaluation"

type payloadKind string

const (
	payloadKindFull  payloadKind = "full"
	payloadKindDelta payloadKind = "delta"
)

type configDefinition struct {
	id             string
	key            string
	configType     ConfigType
	targetingRules evaluation.TargetingRules
}

type serverPayload struct {
	kind     payloadKind
	configs  map[string]configDefinition
	segments evaluation.Segments
}

type payloadReceiver interface {
	receive(payload serverPayload)
}

type connection interface {
	open(receiver payloadReceiver)
	close() error
}
