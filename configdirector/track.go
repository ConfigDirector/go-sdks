package configdirector

import (
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector/internal/telemetry"
)

// TrackedEventDetailValue is one value in the details of a tracked event: a string, a number, a
// boolean, or a nested map or slice of them. Go cannot express that set of types as a type, so the
// compiler checks nothing here; anything encoding/json can write is accepted, such as a struct,
// which is written as an object, or a time.Time, which is written as its RFC 3339 text. NaN and
// infinity in a map[string]any or a []any are written as null.
type TrackedEventDetailValue = any

// TrackedEventDetails are the details of a tracked event, such as the plan a customer chose.
// Details with more than 20 top-level keys, larger than 1 KB as JSON, or holding a value
// encoding/json cannot write, are left out and the event is still reported.
type TrackedEventDetails = map[string]TrackedEventDetailValue

// TrackOptions is what a tracked event carries besides its name. The zero value carries neither.
type TrackOptions struct {
	// Value is a number the event measures, such as an amount of money, a count or a duration; nil
	// means the event has none. It takes any integer or float kind, a json.Number, or a type whose
	// MarshalText writes a JSON number, such as a *big.Int, a *big.Float or a decimal library's
	// type, and is sent as written, without rounding it. A time.Duration is sent as its number of
	// nanoseconds. Anything else, such as NaN, a bool or a string, is left out and the event is
	// still reported. ConfigDirector keeps a value of at most 64 characters with a decimal exponent
	// of at most 400; beyond that it stores the event without its value, and no warning is logged.
	Value any

	// Details are the details of the event; nil means the event has none.
	Details TrackedEventDetails
}

// Track reports to ConfigDirector that something happened, such as a completed checkout, so it can
// be counted. The event is sent with the next telemetry batch. Track returns nothing, never
// panics, and does nothing in ConnectionModeOffline or after Close.
//
// An event counts for an experiment only when it is tracked for a context with an ID, the one the
// experiment's config was evaluated for. With UserContext{}, or a context without an ID, the event
// is still reported. The ID of a context marked Anonymous is not sent. When a context's kind is
// invalid, the error is logged as it is for an evaluation and the event is sent without a context.
//
// A telemetry batch holds at most 1,000 tracked events, and events beyond that are not reported.
// An event whose name does not have 1 to 150 characters is not reported. Details with more than 20
// top-level keys, larger than 1 KB as JSON, or holding a value encoding/json cannot write, and a
// Value that is not a finite number, are left out and the event is still reported. Each of these
// logs one warning per telemetry batch.
func (client *Client) Track(eventName string, userContext UserContext, options TrackOptions) {
	if client.Closed() {
		return
	}

	switch client.settings.mode.configStateSource() {
	case configStateFromOfflineValues:
	case configStateFromConfigDirector:
		admittedContext := client.contextToEvaluate(userContext)
		client.telemetry.RecordTrackedEvent(telemetry.AdmitTrackedEvent(telemetry.TrackCall{
			Name:       eventName,
			Context:    telemetryContextOf(admittedContext),
			Value:      options.Value,
			Details:    options.Details,
			HappenedAt: time.Now(),
		}), admittedContext)
	}
}
