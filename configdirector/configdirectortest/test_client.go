// Package configdirectortest tests code that reads configs through a *configdirector.Client
// without a server.
//
// A test client is the SDK's real client wired to an in-memory connection the test controls. Only
// the connection to ConfigDirector is replaced: value parsing, targeting evaluation, watches,
// events and readiness are the production code, so the code under test behaves as it does against
// ConfigDirector, and no network connection is opened, no telemetry is sent and no goroutine is
// started. Since no telemetry is sent, every read supplied a context with an invalid kind logs an
// error, rather than each distinct kind once per telemetry flush interval.
//
//	testClient := configdirectortest.NewTestClient(map[string]any{"new-checkout": true})
//	defer testClient.Close()
//	service := NewCheckoutService(testClient.Client())
//
//	require.True(t, service.IsNewCheckoutEnabled("user-123"))
//
//	require.NoError(t, testClient.SetValue("new-checkout", false))
//	require.False(t, service.IsNewCheckoutEnabled("user-123"))
//
// Every change a test makes is delivered on the calling goroutine before the call returns, so
// watches and OnConfigsUpdated handlers have run when the next line executes. Updates are
// delivered one at a time, in the order they were made, as the single connection goroutine
// delivers them in production: a change made from inside a handler is delivered after the update
// being handled, once that handler returns.
//
// The events the code under test tracks with Client.Track are recorded in TrackedEvents, under the
// rules the SDK reports them by:
//
//	service.CompleteCheckout("user-123")
//
//	require.Equal(t, []configdirectortest.TrackedEvent{
//		{Name: "checkout-completed", UserContext: configdirector.UserContext{ID: "user-123"}, Value: 49.9},
//	}, testClient.TrackedEvents())
package configdirectortest

import (
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/ConfigDirector/go-sdks/configdirector/internal/connectionseam"
	"github.com/ConfigDirector/go-sdks/configdirector/internal/servedvalues"
	"github.com/ConfigDirector/go-sdks/configdirector/internal/telemetry"
)

const testClientSDKKey = "test-client"

var newClientOverConnection = connectionseam.NewClientOverConnection.(func(configdirector.Options, connectionseam.OpenConnection, func(telemetry.TrackedEventAdmission, configdirector.UserContext)) (*configdirector.Client, error))

// TestClient is a real ConfigDirector client wired to an in-memory connection the test controls.
// Build one with NewTestClient. The client under Client is the SDK's production client; the
// methods on TestClient change what it is served.
type TestClient struct {
	connection *servedvalues.Connection
	client     *configdirector.Client

	trackedEventsMutex sync.Mutex
	trackedEvents      []TrackedEvent
}

// TrackedEvent is an event the code under test tracked, as a test client recorded it.
type TrackedEvent struct {
	// Name is the name the event was tracked with.
	Name string

	// UserContext is the context the event was tracked for, as it was supplied. It is the zero value
	// when there was none, or when its kind was invalid.
	UserContext configdirector.UserContext

	// Value is the value the event was tracked with, as it was given. It is nil when there was none,
	// or when the SDK would leave it out.
	Value any

	// Details are the details the event was tracked with, keeping the Go type of each value, so
	// details of map[string]any{"seats": 3} are recorded with the int 3. Each map[string]any and []any
	// is a copy, so changing it after Track does not change the record; any other value is the one
	// given. NaN and infinity in a map[string]any or a []any are recorded as nil, the null the SDK
	// would send. It is nil when there were none, or when the SDK would leave them out.
	Details configdirector.TrackedEventDetails
}

// Option adjusts how NewTestClient builds its client.
type Option func(*testClientSettings)

type testClientSettings struct {
	options            configdirector.Options
	holdInitialization bool
}

// WithTimeout sets the client's connection timeout, which bounds how long WaitForReady waits
// while initialization is held. Defaults to the SDK's production timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(settings *testClientSettings) { settings.options.Connection.Timeout = timeout }
}

// WithLogger sets the logger the client writes to. Defaults to slog.Default(), as for a
// production client.
func WithLogger(logger *slog.Logger) Option {
	return func(settings *testClientSettings) { settings.options.Logger = logger }
}

// WithHooks attaches event handlers before the client can emit any event, as
// configdirector.Options.Hooks does: an OnConfigsUpdated hook sees the first update and an OnReady
// hook sees the client become ready, both during NewTestClient unless initialization is held.
func WithHooks(hooks configdirector.Hooks) Option {
	return func(settings *testClientSettings) { settings.options.Hooks = hooks }
}

// HoldInitialization keeps the stored values from the client until CompleteInitialization is
// called, so the client starts not ready: reads return their in-code default value and
// WaitForReady blocks, until the client's connection timeout or its context ends. Use it to test
// what the code under test does before config state arrives.
func HoldInitialization() Option {
	return func(settings *testClientSettings) { settings.holdInitialization = true }
}

// NewTestClient creates a test client serving values, keyed by config key; see SetValue for the
// Go values accepted and the config each becomes. The client under Client is ready when
// NewTestClient returns, with the values delivered as its first update on the calling goroutine,
// unless HoldInitialization is among the options. NewTestClient panics when a value cannot be
// served or an option is invalid, as these are mistakes in the test rather than conditions it
// can meet.
func NewTestClient(values map[string]any, options ...Option) *TestClient {
	configs, err := servedvalues.ConfigsFrom(values)
	if err != nil {
		panic(err)
	}
	settings := testClientSettings{options: configdirector.Options{SDKKey: testClientSDKKey}}
	for _, option := range options {
		option(&settings)
	}

	testClient := &TestClient{connection: servedvalues.NewConnection(servedvalues.TestValues, configs)}
	testClient.client, err = newClientOverConnection(settings.options, testClient.connection.Open, testClient.recordTrackedEvent)
	if err != nil {
		panic(err)
	}
	if !settings.holdInitialization {
		testClient.connection.CompleteInitialization()
	}
	return testClient
}

func (testClient *TestClient) recordTrackedEvent(admission telemetry.TrackedEventAdmission, userContext configdirector.UserContext) {
	testClient.trackedEventsMutex.Lock()
	defer testClient.trackedEventsMutex.Unlock()
	testClient.trackedEvents = append(testClient.trackedEvents, TrackedEvent{Name: admission.Name, UserContext: userContext, Value: admission.Value, Details: admission.Details})
}

// Client is the SDK client to hand to the code under test.
func (testClient *TestClient) Client() *configdirector.Client {
	return testClient.client
}

// SetValue stores value under key and, once the client has received its first update, delivers
// it as an update carrying only that key, so reads, watches of key and OnConfigsUpdated handlers
// see it before SetValue returns; called from inside a handler, it returns at once and the update
// is delivered after the one being handled. Every user context receives the same value.
//
// The config type follows from the Go value: a bool is a boolean config; a string is a string
// config, so JSON text meant as a JSON config is given as a json.RawMessage; any integer kind,
// time.Duration included, is an integer config; a float32 or float64 is a float config; and a map,
// slice, array or struct is a JSON config holding what encoding/json writes for it. A nil value,
// a blank key, a non-finite number, a value that does not encode as JSON, or a value of any other
// kind returns an error, and nothing changes.
func (testClient *TestClient) SetValue(key string, value any) error {
	return testClient.connection.SetValue(key, value)
}

// RemoveValue removes key and, once the client has received its first update, delivers a full
// update without it: reads of key return their in-code default value with the config-state-missing
// reason, watches of key receive that default, and OnConfigsUpdated handlers see key in
// RemovedKeys, all before RemoveValue returns. As every full update does, it also fires the
// watches of every remaining key.
func (testClient *TestClient) RemoveValue(key string) {
	testClient.connection.RemoveValue(key)
}

// ReplaceValues replaces every stored value with values and, once the client has received its
// first update, delivers them as a full update. Use it to reset a test client shared across
// tests. A value ReplaceValues cannot serve returns the error SetValue would, and nothing changes.
func (testClient *TestClient) ReplaceValues(values map[string]any) error {
	return testClient.connection.ReplaceValues(values)
}

// CompleteInitialization delivers the stored values to a client built with HoldInitialization, on
// the calling goroutine: OnConfigsUpdated handlers, watches and OnReady handlers run, and a
// blocked WaitForReady returns, before CompleteInitialization returns. On a client that is ready
// or closed it does nothing.
func (testClient *TestClient) CompleteInitialization() {
	testClient.connection.CompleteInitialization()
}

// FailInitialization fails the held initialization the way a refused SDK key does in production:
// the failure is logged at error level and the client stays not ready, so reads return their
// in-code default value with the client-not-ready reason and a WaitForReady in flight ends with
// its timeout or its context. CompleteInitialization still delivers the stored values afterwards.
// On a client that is ready or closed it does nothing.
func (testClient *TestClient) FailInitialization() {
	testClient.connection.FailInitialization()
}

// TrackedEvents returns every event the code under test tracked through the client, in call order,
// as the SDK would report it, and starts from nothing after ClearTrackedEvents. An event whose name
// does not have 1 to 150 characters is not recorded, and a value or details the SDK would leave out
// are missing from the record. The limit of 1,000 events per telemetry batch does not apply. Since
// no telemetry is sent, every event that is not recorded as it was tracked logs a warning, rather
// than each cause once per telemetry batch. Holding the slice does not show later events; call
// TrackedEvents again for them. ReplaceValues leaves the record alone.
func (testClient *TestClient) TrackedEvents() []TrackedEvent {
	testClient.trackedEventsMutex.Lock()
	defer testClient.trackedEventsMutex.Unlock()
	return slices.Clone(testClient.trackedEvents)
}

// ClearTrackedEvents forgets every event recorded so far. Use it to reset a test client shared
// across tests, together with ReplaceValues.
func (testClient *TestClient) ClearTrackedEvents() {
	testClient.trackedEventsMutex.Lock()
	defer testClient.trackedEventsMutex.Unlock()
	testClient.trackedEvents = nil
}

// Close closes the client. After Close, reads return their in-code default value, every handler
// and watch is cancelled, and SetValue, RemoveValue, ReplaceValues and CompleteInitialization
// store without delivering. Closing twice is harmless.
func (testClient *TestClient) Close() error {
	return testClient.client.Close()
}
