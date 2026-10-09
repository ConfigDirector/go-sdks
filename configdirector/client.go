package configdirector

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/ConfigDirector/go-sdks/configdirector/internal/evaluation"
)

// ErrClientClosed is returned by WaitForReady once Close has been called.
var ErrClientClosed = errors.New("configdirector: the client is closed")

// Client reads configs and feature flags from ConfigDirector.
//
// Create one with NewClient during application startup, wait for it with WaitForReady, and
// close it on shutdown. A Client is safe to share between goroutines.
//
// Every read takes an in-code default value and returns it rather than failing: a config the
// client does not hold, a client that is not ready, or a value that cannot be read as the
// requested type all produce the default. The *ValueDetails methods report which of those
// happened.
type Client struct {
	settings   clientSettings
	connection connection

	mutex    sync.RWMutex
	snapshot *configSnapshot
	closed   bool

	ready     chan struct{}
	readyOnce sync.Once
	closing   chan struct{}
	closeOnce sync.Once
	closeErr  error
}

// NewClient validates options, starts connecting to ConfigDirector in the background and
// returns at once. It returns an error when the SDK key is blank, the connection URL is not
// usable, the connection mode is unknown, or a duration or limit is out of range. Until the
// first config state arrives every read returns its in-code default value; call WaitForReady to
// wait for it.
func NewClient(options Options) (*Client, error) {
	settings, err := settingsFrom(options)
	if err != nil {
		return nil, err
	}

	client := &Client{
		settings:   settings,
		connection: newStubConnection(),
		ready:      make(chan struct{}),
		closing:    make(chan struct{}),
	}
	client.connection.open(client)
	return client, nil
}

// WaitForReady blocks until the first config state arrives, the connection timeout elapses, or
// ctx ends, whichever comes first. It returns nil once the client is ready, the context's error
// when ctx ended first, context.DeadlineExceeded when the connection timeout elapsed first, and
// ErrClientClosed when the client was closed. An error only says that the wait ended: the client
// keeps connecting in the background and becomes ready when config state arrives.
func (client *Client) WaitForReady(ctx context.Context) error {
	if client.Ready() {
		return nil
	}
	if client.Closed() {
		return ErrClientClosed
	}

	waitContext, cancel := context.WithTimeout(ctx, client.settings.timeout)
	defer cancel()

	select {
	case <-client.ready:
		return nil
	case <-client.closing:
		return ErrClientClosed
	case <-waitContext.Done():
		client.settings.logger.Warn(
			"The wait for the first config state ended before it arrived; reads return their default value until it does",
			slog.String("mode", string(client.settings.mode)),
			slog.Duration("timeout", client.settings.timeout),
		)
		return waitContext.Err()
	}
}

// Ready reports whether the client holds config state. Until it does, and after Close, every
// read returns its in-code default value.
func (client *Client) Ready() bool {
	_, ready := client.heldSnapshot()
	return ready
}

// Closed reports whether Close has been called.
func (client *Client) Closed() bool {
	client.mutex.RLock()
	defer client.mutex.RUnlock()
	return client.closed
}

// Close stops the connection and releases the client's goroutines. After Close the client is not
// ready and every read returns its in-code default value. Calling Close more than once is
// harmless and returns the first call's result.
func (client *Client) Close() error {
	client.closeOnce.Do(func() {
		client.mutex.Lock()
		client.closed = true
		client.snapshot = nil
		client.mutex.Unlock()

		close(client.closing)
		client.closeErr = client.connection.close()
		client.settings.logger.Debug("The client is closed")
	})
	return client.closeErr
}

func (client *Client) receive(payload serverPayload) {
	client.mutex.Lock()
	if client.closed {
		client.mutex.Unlock()
		return
	}
	client.snapshot = client.snapshot.withPayload(payload)
	client.mutex.Unlock()

	client.readyOnce.Do(func() {
		close(client.ready)
		client.settings.logger.Debug("Received the first config state, the client is ready")
	})
}

// BoolValue evaluates key as a boolean for userContext. It returns defaultValue when the config
// is missing, the client is not ready, or the value is neither true nor false.
func (client *Client) BoolValue(key string, defaultValue bool, userContext UserContext) bool {
	value, _ := client.BoolValueDetails(key, defaultValue, userContext)
	return value
}

// BoolValueDetails is BoolValue with the details of how the value was arrived at.
func (client *Client) BoolValueDetails(key string, defaultValue bool, userContext UserContext) (bool, EvaluationDetails) {
	return readValue(client, key, defaultValue, userContext, boolFrom)
}

// StringValue evaluates key as text for userContext. It returns defaultValue when the config is
// missing, the client is not ready, the value is empty, or the config is a boolean, integer or
// float. A JSON config is returned as its raw document.
func (client *Client) StringValue(key string, defaultValue string, userContext UserContext) string {
	value, _ := client.StringValueDetails(key, defaultValue, userContext)
	return value
}

// StringValueDetails is StringValue with the details of how the value was arrived at.
func (client *Client) StringValueDetails(key string, defaultValue string, userContext UserContext) (string, EvaluationDetails) {
	return readValue(client, key, defaultValue, userContext, stringFrom)
}

// IntValue evaluates key as a whole number for userContext. It returns defaultValue when the
// config is missing, the client is not ready, or the value is not a whole number that fits in an
// int64. A whole number written with a decimal point, such as 26.0, is accepted.
func (client *Client) IntValue(key string, defaultValue int64, userContext UserContext) int64 {
	value, _ := client.IntValueDetails(key, defaultValue, userContext)
	return value
}

// IntValueDetails is IntValue with the details of how the value was arrived at.
func (client *Client) IntValueDetails(key string, defaultValue int64, userContext UserContext) (int64, EvaluationDetails) {
	return readValue(client, key, defaultValue, userContext, int64From)
}

// FloatValue evaluates key as a number for userContext. It returns defaultValue when the config
// is missing, the client is not ready, or the value is not a finite decimal number.
func (client *Client) FloatValue(key string, defaultValue float64, userContext UserContext) float64 {
	value, _ := client.FloatValueDetails(key, defaultValue, userContext)
	return value
}

// FloatValueDetails is FloatValue with the details of how the value was arrived at.
func (client *Client) FloatValueDetails(key string, defaultValue float64, userContext UserContext) (float64, EvaluationDetails) {
	return readValue(client, key, defaultValue, userContext, float64From)
}

// JSONValue evaluates key as a JSON document for userContext and decodes it into target, a
// non-nil pointer to a struct, map, slice or any other type encoding/json can decode into. The
// value the caller put in target beforehand is the in-code default value: target is left as it
// was when the config is missing, the client is not ready, or the document cannot be decoded
// into it. The only error is for a target that is not a non-nil pointer.
func (client *Client) JSONValue(key string, target any, userContext UserContext) error {
	_, err := client.JSONValueDetails(key, target, userContext)
	return err
}

// JSONValueDetails is JSONValue with the details of how the value was arrived at. When it
// returns an error, the details are empty.
func (client *Client) JSONValueDetails(key string, target any, userContext UserContext) (EvaluationDetails, error) {
	targetValue, err := jsonTargetFrom(target)
	if err != nil {
		return EvaluationDetails{}, err
	}

	state, reason := client.readableState(key, userContext)
	if reason == EvaluationReasonFoundMatch {
		reason = decodeJSONInto(state, targetValue)
	}
	if reason != EvaluationReasonFoundMatch {
		return defaultEvaluation(key, reason, userContext).details(), nil
	}
	return servedEvaluation(key, targetValue.Elem().Interface(), state.ValueID, userContext).details(), nil
}

// AllConfigs returns the evaluated state of every config the client holds, or of the configs
// with the given keys, sorted by key. Intended for handing state to a client SDK to hydrate
// with; it records no telemetry. Empty until the client is ready.
func (client *Client) AllConfigs(userContext UserContext, keys ...string) []ConfigState {
	snapshot, ready := client.heldSnapshot()
	if !ready {
		return []ConfigState{}
	}
	if len(keys) == 0 {
		keys = snapshot.keys()
	}
	return snapshot.statesOf(keys, client.evaluationContextFor(userContext))
}

func readValue[T any](client *Client, key string, defaultValue T, userContext UserContext, valueFrom func(ConfigState) (T, EvaluationReason)) (T, EvaluationDetails) {
	state, reason := client.readableState(key, userContext)
	if reason != EvaluationReasonFoundMatch {
		return defaultValue, defaultEvaluation(key, reason, userContext).details()
	}

	value, reason := valueFrom(state)
	if reason != EvaluationReasonFoundMatch {
		return defaultValue, defaultEvaluation(key, reason, userContext).details()
	}
	return value, servedEvaluation(key, value, state.ValueID, userContext).details()
}

func (client *Client) readableState(key string, userContext UserContext) (ConfigState, EvaluationReason) {
	snapshot, ready := client.heldSnapshot()
	if !ready {
		return ConfigState{}, EvaluationReasonClientNotReady
	}
	state, found := snapshot.stateOf(key, client.evaluationContextFor(userContext))
	if !found {
		return ConfigState{}, EvaluationReasonConfigStateMissing
	}
	if state.Value == "" {
		return state, EvaluationReasonValueMissing
	}
	return state, EvaluationReasonFoundMatch
}

func (client *Client) heldSnapshot() (*configSnapshot, bool) {
	client.mutex.RLock()
	defer client.mutex.RUnlock()
	return client.snapshot, client.snapshot != nil
}

func (client *Client) evaluationContextFor(userContext UserContext) evaluation.EvaluationContext {
	return evaluation.EvaluationContext{
		Identifier: userContext.ID,
		Name:       userContext.Name,
		Traits:     userContext.Traits,
		AppName:    client.settings.metadata.AppName,
		AppVersion: client.settings.metadata.AppVersion,
	}
}
