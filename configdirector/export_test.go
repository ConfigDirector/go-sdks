package configdirector

import "github.com/ConfigDirector/go-sdks/configdirector/internal/evaluation"

// TestConnection is a connection the tests drive by hand. Deliver hands a payload to the client
// on the calling goroutine, so every handler has run by the time it returns.
type TestConnection struct {
	receiver payloadReceiver
}

func NewClientWithTestConnection(options Options) (*Client, *TestConnection, error) {
	connection := &TestConnection{}
	client, err := newClient(options, connection)
	return client, connection, err
}

func (connection *TestConnection) open(receiver payloadReceiver) {
	connection.receiver = receiver
}

func (connection *TestConnection) close() error {
	return nil
}

func (connection *TestConnection) Deliver(payload serverPayload) {
	connection.receiver.receive(payload)
}

func FullUpdate(configs ...configDefinition) serverPayload {
	return serverPayload{kind: payloadKindFull, configs: definitionsByKey(configs...), segments: evaluation.Segments{}}
}

func DeltaUpdate(configs ...configDefinition) serverPayload {
	return serverPayload{kind: payloadKindDelta, configs: definitionsByKey(configs...), segments: evaluation.Segments{}}
}

// WithSegment adds a segment matching users whose plan trait equals plan.
func WithSegment(payload serverPayload, segmentID string, plan string) serverPayload {
	payload.segments[segmentID] = evaluation.Segment{Groups: [][]evaluation.AttributeCondition{{traitEquals("/plan", plan)}}}
	return payload
}

func TextConfig(key string, value string) configDefinition {
	return stubConfig(key, ConfigTypeString, value)
}

func BoolConfig(key string, value string) configDefinition {
	return stubConfig(key, ConfigTypeBoolean, value)
}

func IntConfig(key string, value string) configDefinition {
	return stubConfig(key, ConfigTypeInteger, value)
}

func FloatConfig(key string, value string) configDefinition {
	return stubConfig(key, ConfigTypeFloat, value)
}

func JSONConfig(key string, value string) configDefinition {
	return stubConfig(key, ConfigTypeJSON, value)
}

// BoolConfigTrueInSegment serves true to users in the segment and false to everyone else.
func BoolConfigTrueInSegment(key string, segmentID string) configDefinition {
	return stubConfigWithRules(key, ConfigTypeBoolean, "false", []evaluation.Rule{
		valueRule(key+"-in-segment", 0, "true", inSegment(segmentID)),
	})
}
