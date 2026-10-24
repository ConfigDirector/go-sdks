package configdirectoropenfeature

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/ConfigDirector/go-sdks/configdirector/wrapper"
	"github.com/open-feature/go-sdk/openfeature"
)

const (
	providerName             = "configdirector"
	loggerComponentAttribute = "configdirector-openfeature"
)

// Provider is an OpenFeature provider backed by the ConfigDirector Go server SDK.
//
// Create one with NewProvider and register it with OpenFeature; read values through an
// OpenFeature client. The provider owns a ConfigDirector client: it connects as soon as it is
// created, waits for the first config state when OpenFeature initializes it, and closes the
// client when OpenFeature shuts it down. Targeting rules are evaluated locally, so an evaluation
// makes no network calls.
//
// The OpenFeature evaluation context maps onto the ConfigDirector UserContext: the targeting
// key, or failing that an "id" attribute, becomes ID; a "kind" attribute becomes Kind, the
// context kind, with user when it is left out; a "name" attribute becomes Name; a "traits"
// attribute holding a map[string]any becomes Traits; and a boolean "anonymous" attribute becomes
// Anonymous. A "kind" inside "traits" is an ordinary trait. An evaluation whose context has an
// invalid kind still resolves successfully, to the value for no context, and the SDK logs the
// error.
//
// If the first config state does not arrive within the connection timeout, initialization still
// completes and evaluations return their default value with the error code PROVIDER_NOT_READY.
// The provider keeps connecting, and emits PROVIDER_READY once config state arrives. It emits
// PROVIDER_CONFIGURATION_CHANGED whenever config state is updated, naming the flags the update
// carried followed by the flags that were removed.
type Provider struct {
	client  *configdirector.Client
	logger  *slog.Logger
	events  chan openfeature.Event
	closing chan struct{}

	mutex       sync.Mutex
	initialized bool
	closeOnce   sync.Once
}

var (
	_ openfeature.FeatureProvider = (*Provider)(nil)
	_ openfeature.StateHandler    = (*Provider)(nil)
	_ openfeature.EventHandler    = (*Provider)(nil)
)

// NewProvider creates a provider whose ConfigDirector client is built from options, exactly as
// configdirector.NewClient builds one, and starts connecting at once. The client reports this
// provider's name and Version to ConfigDirector. Handlers in options.Hooks run after the
// provider's own. It returns the error configdirector.NewClient returns for invalid options. In
// configdirector.ConnectionModeOffline the provider needs no SDK key, serves the values in options
// without connecting, and is ready as soon as OpenFeature initializes it.
//
// Register the provider with OpenFeature right after creating it: a config state update that
// arrives before OpenFeature reads the provider's events waits for it, and so does the client.
func NewProvider(options configdirector.Options) (*Provider, error) {
	return newProvider(func(providerHooks configdirector.Hooks) (*configdirector.Client, error) {
		options.Hooks = hooksInOrder(providerHooks, options.Hooks)
		return wrapper.NewClient(wrapper.OpenFeatureProvider, Version, options)
	}, loggerFrom(options.Logger))
}

func newProvider(open func(hooks configdirector.Hooks) (*configdirector.Client, error), logger *slog.Logger) (*Provider, error) {
	provider := &Provider{
		logger:  logger,
		events:  make(chan openfeature.Event, 1),
		closing: make(chan struct{}),
	}
	client, err := open(configdirector.Hooks{
		OnReady:          provider.announceReadiness,
		OnConfigsUpdated: provider.announceChangedFlags,
	})
	if err != nil {
		return nil, err
	}
	provider.client = client
	return provider, nil
}

func loggerFrom(configured *slog.Logger) *slog.Logger {
	if configured == nil {
		configured = slog.Default()
	}
	return configured.With(slog.String("component", loggerComponentAttribute))
}

func hooksInOrder(first configdirector.Hooks, then configdirector.Hooks) configdirector.Hooks {
	hooks := then
	hooks.OnReady = readyHandlersInOrder(first.OnReady, then.OnReady)
	hooks.OnConfigsUpdated = updateHandlersInOrder(first.OnConfigsUpdated, then.OnConfigsUpdated)
	return hooks
}

func readyHandlersInOrder(first func(), then func()) func() {
	if then == nil {
		return first
	}
	return func() {
		first()
		then()
	}
}

func updateHandlersInOrder(first func(configdirector.ConfigsUpdated), then func(configdirector.ConfigsUpdated)) func(configdirector.ConfigsUpdated) {
	if then == nil {
		return first
	}
	return func(update configdirector.ConfigsUpdated) {
		first(update)
		then(update)
	}
}

// Metadata names the provider to OpenFeature.
func (*Provider) Metadata() openfeature.Metadata {
	return openfeature.Metadata{Name: providerName}
}

// Hooks returns no hooks: the provider adds none to an evaluation.
func (*Provider) Hooks() []openfeature.Hook {
	return nil
}

// Init waits for the first config state for the client's connection timeout. It returns nil
// whether or not config state arrived, so initialization completes either way, and
// configdirector.ErrClientClosed when the provider was shut down first. OpenFeature calls it when
// the provider is registered.
func (provider *Provider) Init(_ openfeature.EvaluationContext) error {
	err := provider.client.WaitForReady(context.Background())

	switch initializationOutcomeFor(err) {
	case initializationFailsOnAClosedClient:
		return err
	case initializationCompletesWithOrWithoutConfigState:
		provider.mutex.Lock()
		provider.initialized = true
		provider.mutex.Unlock()
	}
	return nil
}

type initializationOutcome int

const (
	initializationCompletesWithOrWithoutConfigState initializationOutcome = iota
	initializationFailsOnAClosedClient
)

func initializationOutcomeFor(waitError error) initializationOutcome {
	if errors.Is(waitError, configdirector.ErrClientClosed) {
		return initializationFailsOnAClosedClient
	}
	return initializationCompletesWithOrWithoutConfigState
}

// Shutdown closes the ConfigDirector client, which sends the telemetry collected since the last
// report. OpenFeature calls it when the provider is replaced or OpenFeature shuts down. Calling
// it more than once is harmless.
func (provider *Provider) Shutdown() {
	provider.closeOnce.Do(func() {
		close(provider.closing)
		if err := provider.client.Close(); err != nil {
			provider.logger.Warn("The ConfigDirector client did not close cleanly", slog.Any("cause", err))
		}
	})
}

// EventChannel delivers the provider's events to OpenFeature: PROVIDER_CONFIGURATION_CHANGED on
// every config state update and PROVIDER_READY when config state arrives after initialization
// completed without it. The channel holds one event; a further event waits until OpenFeature
// reads it.
func (provider *Provider) EventChannel() <-chan openfeature.Event {
	return provider.events
}

func (provider *Provider) announceChangedFlags(update configdirector.ConfigsUpdated) {
	provider.deliverToOpenFeature(openfeature.Event{
		ProviderName:         providerName,
		EventType:            openfeature.ProviderConfigChange,
		ProviderEventDetails: openfeature.ProviderEventDetails{FlagChanges: slices.Concat(update.Keys, update.RemovedKeys)},
	})
}

type readinessAnnouncer int

const (
	openFeatureAnnouncesReadiness readinessAnnouncer = iota
	providerAnnouncesReadiness
)

func readinessAnnouncerFor(initialized bool) readinessAnnouncer {
	if initialized {
		return providerAnnouncesReadiness
	}
	return openFeatureAnnouncesReadiness
}

func (provider *Provider) announceReadiness() {
	provider.mutex.Lock()
	announcer := readinessAnnouncerFor(provider.initialized)
	provider.mutex.Unlock()

	switch announcer {
	case openFeatureAnnouncesReadiness:
		return
	case providerAnnouncesReadiness:
		provider.deliverToOpenFeature(openfeature.Event{
			ProviderName:         providerName,
			EventType:            openfeature.ProviderReady,
			ProviderEventDetails: openfeature.ProviderEventDetails{Message: "The first config state arrived"},
		})
	}
}

func (provider *Provider) deliverToOpenFeature(event openfeature.Event) {
	select {
	case provider.events <- event:
	case <-provider.closing:
	}
}

// BooleanEvaluation evaluates flag as a boolean.
func (provider *Provider) BooleanEvaluation(_ context.Context, flag string, defaultValue bool, flattenedContext openfeature.FlattenedContext) openfeature.BoolResolutionDetail {
	return resolutionOf(flag, defaultValue, flattenedContext, provider.client.BoolValueDetails)
}

// StringEvaluation evaluates flag as text.
func (provider *Provider) StringEvaluation(_ context.Context, flag string, defaultValue string, flattenedContext openfeature.FlattenedContext) openfeature.StringResolutionDetail {
	return resolutionOf(flag, defaultValue, flattenedContext, provider.client.StringValueDetails)
}

// FloatEvaluation evaluates flag as a number.
func (provider *Provider) FloatEvaluation(_ context.Context, flag string, defaultValue float64, flattenedContext openfeature.FlattenedContext) openfeature.FloatResolutionDetail {
	return resolutionOf(flag, defaultValue, flattenedContext, provider.client.FloatValueDetails)
}

// IntEvaluation evaluates flag as a whole number.
func (provider *Provider) IntEvaluation(_ context.Context, flag string, defaultValue int64, flattenedContext openfeature.FlattenedContext) openfeature.IntResolutionDetail {
	return resolutionOf(flag, defaultValue, flattenedContext, provider.client.IntValueDetails)
}

// ObjectEvaluation evaluates flag as a JSON document, decoded as encoding/json decodes into an
// any: an object becomes a map[string]any, an array a []any, a number a float64. It returns
// defaultValue as given when the flag falls back.
func (provider *Provider) ObjectEvaluation(_ context.Context, flag string, defaultValue any, flattenedContext openfeature.FlattenedContext) openfeature.InterfaceResolutionDetail {
	return documentResolutionOf(flag, defaultValue, flattenedContext, provider.client.JSONValueDetails)
}

func documentResolutionOf(flag string, defaultValue any, flattenedContext openfeature.FlattenedContext, read func(key string, target any, userContext configdirector.UserContext) (configdirector.EvaluationDetails, error)) openfeature.InterfaceResolutionDetail {
	value := defaultValue
	details, err := read(flag, &value, userContextFrom(flattenedContext))
	if err != nil {
		return openfeature.InterfaceResolutionDetail{
			Value:                    defaultValue,
			ProviderResolutionDetail: resolutionErrorDetail(openfeature.NewGeneralResolutionError(err.Error())),
		}
	}
	return openfeature.InterfaceResolutionDetail{Value: value, ProviderResolutionDetail: resolutionDetailFor(flag, details)}
}

func resolutionOf[T any](flag string, defaultValue T, flattenedContext openfeature.FlattenedContext, read func(key string, defaultValue T, userContext configdirector.UserContext) (T, configdirector.EvaluationDetails)) openfeature.GenericResolutionDetail[T] {
	value, details := read(flag, defaultValue, userContextFrom(flattenedContext))
	return openfeature.GenericResolutionDetail[T]{Value: value, ProviderResolutionDetail: resolutionDetailFor(flag, details)}
}
