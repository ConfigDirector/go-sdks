package configdirectoropenfeature

import (
	"context"
	"encoding/json"
	"log/slog"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/ConfigDirector/go-sdks/configdirector/configdirectortest"
	"github.com/open-feature/go-sdk/openfeature"
	"github.com/open-feature/go-sdk/openfeature/isolated"
	"github.com/stretchr/testify/require"
)

const awaitTimeout = 5 * time.Second

var noContext = openfeature.EvaluationContext{}

type providerUnderTest struct {
	testClient *configdirectortest.TestClient
	provider   *Provider
	api        *openfeature.EvaluationAPI
	client     *openfeature.Client
}

func providerServing(t *testing.T, values map[string]any, options ...configdirectortest.Option) providerUnderTest {
	t.Helper()
	var testClient *configdirectortest.TestClient
	provider, err := newProvider(func(hooks configdirector.Hooks) (*configdirector.Client, error) {
		testClient = configdirectortest.NewTestClient(values, append(options, configdirectortest.WithHooks(hooks))...)
		return testClient.Client(), nil
	}, slog.Default())
	require.NoError(t, err)
	t.Cleanup(provider.Shutdown)

	api := isolated.NewAPI()
	t.Cleanup(func() { require.NoError(t, api.Shutdown(context.Background())) })
	return providerUnderTest{testClient: testClient, provider: provider, api: api, client: api.NewClient()}
}

func (fixture providerUnderTest) register(t *testing.T) {
	t.Helper()
	require.NoError(t, fixture.api.SetProviderAndWait(t.Context(), fixture.provider))
}

func registeredProviderServing(t *testing.T, values map[string]any, options ...configdirectortest.Option) providerUnderTest {
	t.Helper()
	fixture := providerServing(t, values, options...)
	fixture.register(t)
	return fixture
}

type eventRecorder struct {
	events chan openfeature.EventDetails
}

func recordEvents(client *openfeature.Client, eventType openfeature.EventType) *eventRecorder {
	recorder := &eventRecorder{events: make(chan openfeature.EventDetails, 64)}
	callback := func(details openfeature.EventDetails) { recorder.events <- details }
	client.AddHandler(eventType, &callback)
	return recorder
}

func (recorder *eventRecorder) next(t *testing.T) openfeature.EventDetails {
	t.Helper()
	return await(t, recorder.events)
}

func (recorder *eventRecorder) received() []openfeature.EventDetails {
	received := []openfeature.EventDetails{}
	for {
		select {
		case details := <-recorder.events:
			received = append(received, details)
		default:
			return received
		}
	}
}

func await[T any](t *testing.T, values <-chan T) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(awaitTimeout):
		t.Fatal("nothing arrived within the await timeout")
		var zero T
		return zero
	}
}

func TestABooleanFlagResolvesToItsValueWithTheValueIDAsTheVariant(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{"new-checkout": true})

	details, err := fixture.client.BooleanValueDetails(t.Context(), "new-checkout", false, noContext)

	require.NoError(t, err)
	require.True(t, details.Value)
	require.Equal(t, openfeature.TargetingMatchReason, details.Reason)
	require.Equal(t, "test-value:new-checkout", details.Variant)
	require.Empty(t, details.ErrorCode)
}

func TestAStringFlagResolvesToItsValueWithTheValueIDAsTheVariant(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{"greeting": "hello"})

	details, err := fixture.client.StringValueDetails(t.Context(), "greeting", "fallback", noContext)

	require.NoError(t, err)
	require.Equal(t, "hello", details.Value)
	require.Equal(t, openfeature.TargetingMatchReason, details.Reason)
	require.Equal(t, "test-value:greeting", details.Variant)
}

func TestAnIntFlagResolvesToItsValueWithTheValueIDAsTheVariant(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{"max-items": 26})

	details, err := fixture.client.IntValueDetails(t.Context(), "max-items", 1, noContext)

	require.NoError(t, err)
	require.Equal(t, int64(26), details.Value)
	require.Equal(t, openfeature.TargetingMatchReason, details.Reason)
	require.Equal(t, "test-value:max-items", details.Variant)
}

func TestAFloatFlagResolvesToItsValueWithTheValueIDAsTheVariant(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{"ratio": 1.5})

	details, err := fixture.client.FloatValueDetails(t.Context(), "ratio", 0.1, noContext)

	require.NoError(t, err)
	require.InDelta(t, 1.5, details.Value, 0)
	require.Equal(t, openfeature.TargetingMatchReason, details.Reason)
	require.Equal(t, "test-value:ratio", details.Variant)
}

func TestAnObjectFlagResolvesToItsDecodedDocumentWithTheValueIDAsTheVariant(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{"limits": map[string]any{"max": 3, "tags": []string{"a"}}})

	details, err := fixture.client.ObjectValueDetails(t.Context(), "limits", map[string]any{"max": 1}, noContext)

	require.NoError(t, err)
	require.Equal(t, map[string]any{"max": float64(3), "tags": []any{"a"}}, details.Value)
	require.Equal(t, openfeature.TargetingMatchReason, details.Reason)
	require.Equal(t, "test-value:limits", details.Variant)
}

func TestAFlagNobodyDefinedResolvesToTheDefaultAsFlagNotFound(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{"greeting": "hello"})

	details, err := fixture.client.StringValueDetails(t.Context(), "farewell", "fallback", noContext)

	require.Error(t, err)
	require.Equal(t, "fallback", details.Value)
	require.Equal(t, openfeature.ErrorReason, details.Reason)
	require.Equal(t, openfeature.FlagNotFoundCode, details.ErrorCode)
	require.Empty(t, details.Variant)
}

func TestAFlagOfAnotherTypeResolvesToTheDefaultAsATypeMismatch(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{"greeting": "hello"})

	details, err := fixture.client.IntValueDetails(t.Context(), "greeting", 7, noContext)

	require.Error(t, err)
	require.Equal(t, int64(7), details.Value)
	require.Equal(t, openfeature.ErrorReason, details.Reason)
	require.Equal(t, openfeature.TypeMismatchCode, details.ErrorCode)
}

func TestAnObjectFlagThatIsNotADocumentResolvesToTheDefaultAsGivenAsATypeMismatch(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{"greeting": "hello"})

	details, err := fixture.client.ObjectValueDetails(t.Context(), "greeting", map[string]any{"max": 1}, noContext)

	require.Error(t, err)
	require.Equal(t, map[string]any{"max": 1}, details.Value)
	require.Equal(t, openfeature.TypeMismatchCode, details.ErrorCode)
}

func TestAnObjectFlagWhoseDocumentIsANumberResolvesToTheDefaultAsGivenAsATypeMismatch(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{"limits": json.RawMessage("42")})

	details, err := fixture.client.ObjectValueDetails(t.Context(), "limits", map[string]any{"max": 1}, noContext)

	require.Equal(t, map[string]any{"max": 1}, details.Value)
	require.Error(t, err)
	require.Equal(t, openfeature.ErrorReason, details.Reason)
	require.Equal(t, openfeature.TypeMismatchCode, details.ErrorCode)
	require.Contains(t, details.ErrorMessage, "limits")
}

func TestAnObjectFlagWhoseDocumentIsNullResolvesToTheDefaultAsGivenAsATypeMismatch(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{"limits": json.RawMessage("null")})

	details, err := fixture.client.ObjectValueDetails(t.Context(), "limits", map[string]any{"max": 1}, noContext)

	require.Equal(t, map[string]any{"max": 1}, details.Value)
	require.Error(t, err)
	require.Equal(t, openfeature.TypeMismatchCode, details.ErrorCode)
}

func TestAnObjectFlagWhoseDocumentIsAListResolvesToTheDefaultAsGivenAsATypeMismatch(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{"limits": json.RawMessage("[1,2]")})

	details, err := fixture.client.ObjectValueDetails(t.Context(), "limits", map[string]any{"max": 1}, noContext)

	require.Equal(t, map[string]any{"max": 1}, details.Value)
	require.Error(t, err)
	require.Equal(t, openfeature.TypeMismatchCode, details.ErrorCode)
}

func TestAnObjectFlagWithAStructDefaultResolvesToTheDefaultAsGivenWhenItsDocumentIsANumber(t *testing.T) {
	t.Parallel()
	type limits struct{ Max int }
	fixture := registeredProviderServing(t, map[string]any{"limits": json.RawMessage("42")})

	details, err := fixture.client.ObjectValueDetails(t.Context(), "limits", limits{Max: 1}, noContext)

	require.Equal(t, limits{Max: 1}, details.Value)
	require.Error(t, err)
	require.Equal(t, openfeature.TypeMismatchCode, details.ErrorCode)
}

func TestAListFlagResolvesToItsDecodedDocumentWithTheValueIDAsTheVariant(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{"regions": []string{"eu", "us"}})

	details, err := fixture.client.ObjectValueDetails(t.Context(), "regions", []any{"eu"}, noContext)

	require.NoError(t, err)
	require.Equal(t, []any{"eu", "us"}, details.Value)
	require.Equal(t, openfeature.TargetingMatchReason, details.Reason)
	require.Equal(t, "test-value:regions", details.Variant)
}

func TestAListFlagWhoseDocumentIsAnObjectResolvesToTheDefaultAsGivenAsATypeMismatch(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{"regions": map[string]any{"eu": true}})

	details, err := fixture.client.ObjectValueDetails(t.Context(), "regions", []any{"eu"}, noContext)

	require.Equal(t, []any{"eu"}, details.Value)
	require.Error(t, err)
	require.Equal(t, openfeature.ErrorReason, details.Reason)
	require.Equal(t, openfeature.TypeMismatchCode, details.ErrorCode)
	require.Contains(t, details.ErrorMessage, "regions")
}

func TestAListFlagWithAnArrayDefaultResolvesToTheDefaultAsGivenWhenItsDocumentIsAnObject(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{"regions": map[string]any{"eu": true}})

	details, err := fixture.client.ObjectValueDetails(t.Context(), "regions", [1]string{"eu"}, noContext)

	require.Equal(t, [1]string{"eu"}, details.Value)
	require.Error(t, err)
	require.Equal(t, openfeature.TypeMismatchCode, details.ErrorCode)
}

func TestAnObjectFlagWithANumberDefaultResolvesToWhicheverDocumentTheConfigHolds(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{"limits": json.RawMessage("42")})

	details, err := fixture.client.ObjectValueDetails(t.Context(), "limits", float64(0), noContext)

	require.NoError(t, err)
	require.Equal(t, float64(42), details.Value)
	require.Equal(t, openfeature.TargetingMatchReason, details.Reason)
}

func TestAnObjectFlagNobodyDefinedResolvesToATypedMapDefaultAsFlagNotFound(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{"greeting": "hello"})

	details, err := fixture.client.ObjectValueDetails(t.Context(), "limits", map[string]int{"max": 1}, noContext)

	require.Error(t, err)
	require.Equal(t, map[string]int{"max": 1}, details.Value)
	require.Equal(t, openfeature.FlagNotFoundCode, details.ErrorCode)
}

func TestAFlagWithoutAValueResolvesToTheDefaultWithTheDefaultReason(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{"greeting": ""})

	details, err := fixture.client.StringValueDetails(t.Context(), "greeting", "fallback", noContext)

	require.NoError(t, err)
	require.Equal(t, "fallback", details.Value)
	require.Equal(t, openfeature.DefaultReason, details.Reason)
	require.Empty(t, details.ErrorCode)
}

func TestTheEvaluationContextReachesTheClientAsAUserContext(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{"greeting": "hello"})
	evaluated := make(chan configdirector.UserContext, 1)
	fixture.testClient.Client().OnConfigEvaluated(func(evaluation configdirector.Evaluation) { evaluated <- evaluation.UserContext })
	evaluationContext := openfeature.NewEvaluationContext("user-1", map[string]any{
		"name":      "Ada",
		"traits":    map[string]any{"plan": "pro"},
		"anonymous": true,
	})

	fixture.client.String(t.Context(), "greeting", "fallback", evaluationContext)

	require.Equal(t, configdirector.UserContext{ID: "user-1", Name: "Ada", Traits: map[string]any{"plan": "pro"}, Anonymous: true}, await(t, evaluated))
}

func TestAnInvalidContextKindResolvesSuccessfullyToTheNoContextValue(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{"greeting": "hello"})
	evaluated := make(chan configdirector.UserContext, 1)
	fixture.testClient.Client().OnConfigEvaluated(func(evaluation configdirector.Evaluation) { evaluated <- evaluation.UserContext })
	evaluationContext := openfeature.NewEvaluationContext("42", map[string]any{"kind": "Device"})

	details, err := fixture.client.StringValueDetails(t.Context(), "greeting", "fallback", evaluationContext)

	require.NoError(t, err)
	require.Equal(t, "hello", details.Value)
	require.Equal(t, openfeature.TargetingMatchReason, details.Reason)
	require.Empty(t, details.ErrorCode)
	require.Equal(t, configdirector.UserContext{}, await(t, evaluated))
}

func TestAnAfterHookSeesTargetingMatchAndTheValueIDForAServedValue(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{"new-checkout": true})
	hook := &recordingHook{}
	fixture.client.AddHooks(hook)

	fixture.client.Boolean(t.Context(), "new-checkout", false, noContext)

	require.Len(t, hook.afterDetails, 1)
	require.Equal(t, openfeature.TargetingMatchReason, hook.afterDetails[0].Reason)
	require.Equal(t, "test-value:new-checkout", hook.afterDetails[0].Variant)
}

func TestAnErrorHookSeesFlagNotFoundForAConfigThatDoesNotExist(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{"greeting": "hello"})
	hook := &recordingHook{}
	fixture.client.AddHooks(hook)

	fixture.client.Boolean(t.Context(), "new-checkout", false, noContext)

	require.Len(t, hook.errors, 1)
	require.ErrorContains(t, hook.errors[0], string(openfeature.FlagNotFoundCode))
}

type recordingHook struct {
	openfeature.UnimplementedHook
	afterDetails []openfeature.InterfaceEvaluationDetails
	errors       []error
}

func (hook *recordingHook) After(_ context.Context, _ openfeature.HookContext, details openfeature.InterfaceEvaluationDetails, _ openfeature.HookHints) error {
	hook.afterDetails = append(hook.afterDetails, details)
	return nil
}

func (hook *recordingHook) Error(_ context.Context, _ openfeature.HookContext, err error, _ openfeature.HookHints) {
	hook.errors = append(hook.errors, err)
}

func TestTheFirstConfigStateAnnouncesEveryFlag(t *testing.T) {
	t.Parallel()
	fixture := providerServing(t, map[string]any{"greeting": "hello", "new-checkout": true})
	changes := recordEvents(fixture.client, openfeature.ProviderConfigChange)

	fixture.register(t)

	announced := changes.next(t)
	require.Equal(t, []string{"greeting", "new-checkout"}, announced.FlagChanges)
	require.Equal(t, "configdirector", announced.ProviderName)
}

func TestAValueChangeAnnouncesTheFlagThatChanged(t *testing.T) {
	t.Parallel()
	fixture := providerServing(t, map[string]any{"greeting": "hello"})
	changes := recordEvents(fixture.client, openfeature.ProviderConfigChange)
	fixture.register(t)
	changes.next(t)

	require.NoError(t, fixture.testClient.SetValue("new-checkout", true))

	require.Equal(t, []string{"new-checkout"}, changes.next(t).FlagChanges)
	require.True(t, fixture.client.Boolean(t.Context(), "new-checkout", false, noContext))
}

func TestARemovedFlagIsAnnouncedAfterTheFlagsTheUpdateCarried(t *testing.T) {
	t.Parallel()
	fixture := providerServing(t, map[string]any{"a": "1", "b": "2"})
	changes := recordEvents(fixture.client, openfeature.ProviderConfigChange)
	fixture.register(t)
	changes.next(t)

	fixture.testClient.RemoveValue("a")

	require.Equal(t, []string{"b", "a"}, changes.next(t).FlagChanges)
	require.Equal(t, "fallback", fixture.client.String(t.Context(), "a", "fallback", noContext))
}

func TestInitializationFailsAsNotReadyWhenNoConfigStateArrivesWithinTheTimeout(t *testing.T) {
	t.Parallel()
	fixture := providerServing(t, map[string]any{"greeting": "hello"}, configdirectortest.HoldInitialization(), configdirectortest.WithTimeout(50*time.Millisecond))

	err := fixture.api.SetProviderAndWait(t.Context(), fixture.provider)

	var initializationError *openfeature.ProviderInitError
	require.ErrorAs(t, err, &initializationError)
	require.Equal(t, openfeature.ProviderNotReadyCode, initializationError.ErrorCode)
	require.NotErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, openfeature.ErrorState, fixture.client.State())

	details, err := fixture.client.StringValueDetails(t.Context(), "greeting", "fallback", noContext)

	require.Error(t, err)
	require.Equal(t, "fallback", details.Value)
	require.Equal(t, openfeature.ProviderNotReadyCode, details.ErrorCode)
}

func TestInitializationFailsAsNotReadyWhenConfigDirectorRejectsTheSDKKey(t *testing.T) {
	t.Parallel()
	fixture := providerServing(t, map[string]any{"greeting": "hello"}, configdirectortest.HoldInitialization(), configdirectortest.WithTimeout(50*time.Millisecond))
	fixture.testClient.FailInitialization()

	err := fixture.api.SetProviderAndWait(t.Context(), fixture.provider)

	var initializationError *openfeature.ProviderInitError
	require.ErrorAs(t, err, &initializationError)
	require.Equal(t, openfeature.ProviderNotReadyCode, initializationError.ErrorCode)
	require.Equal(t, openfeature.ErrorState, fixture.client.State())
}

func TestConfigStateArrivingAfterAFailedInitializationIsAnnouncedAsReadyOnceAndEvaluated(t *testing.T) {
	t.Parallel()
	fixture := providerServing(t, map[string]any{"greeting": "hello"}, configdirectortest.HoldInitialization(), configdirectortest.WithTimeout(50*time.Millisecond))
	readiness := recordEvents(fixture.client, openfeature.ProviderReady)
	changes := recordEvents(fixture.client, openfeature.ProviderConfigChange)
	require.Error(t, fixture.api.SetProviderAndWait(t.Context(), fixture.provider))
	awaitTheEndOfEveryReadinessCheck(t)

	fixture.testClient.CompleteInitialization()

	require.Equal(t, "configdirector", readiness.next(t).ProviderName)
	require.Equal(t, openfeature.ReadyState, fixture.client.State())
	require.Equal(t, "hello", fixture.client.String(t.Context(), "greeting", "fallback", noContext))
	changes.next(t)

	require.NoError(t, fixture.testClient.SetValue("greeting", "hi"))

	changes.next(t)
	require.Empty(t, readiness.received())
}

func TestAFailedInitializationAnnouncesNoReadinessBeforeConfigStateArrives(t *testing.T) {
	t.Parallel()
	fixture := providerServing(t, map[string]any{"greeting": "hello"}, configdirectortest.HoldInitialization(), configdirectortest.WithTimeout(50*time.Millisecond))
	readiness := recordEvents(fixture.client, openfeature.ProviderReady)
	require.Error(t, fixture.api.SetProviderAndWait(t.Context(), fixture.provider))

	awaitTheEndOfEveryReadinessCheck(t)

	require.Never(t, func() bool { return len(readiness.events) > 0 }, 200*time.Millisecond, 10*time.Millisecond)
	require.Equal(t, openfeature.ErrorState, fixture.client.State())
}

func TestConfigStateArrivingWhileOpenFeatureRecordsTheFailedInitializationIsAnnouncedAsReady(t *testing.T) {
	t.Parallel()
	fixture := providerServing(t, map[string]any{"greeting": "hello"}, configdirectortest.HoldInitialization(), configdirectortest.WithTimeout(50*time.Millisecond))
	readiness := recordEvents(fixture.client, openfeature.ProviderReady)
	failures := recordEvents(fixture.client, openfeature.ProviderError)
	changes := recordEvents(fixture.client, openfeature.ProviderConfigChange)
	provider := &providerWhoseFailureOpenFeatureRecordsLate{Provider: fixture.provider, failed: make(chan struct{}), recordFailure: make(chan struct{})}
	require.NoError(t, fixture.api.SetProvider(t.Context(), provider))
	await(t, provider.failed)

	fixture.testClient.CompleteInitialization()
	changes.next(t)
	require.NoError(t, fixture.testClient.SetValue("greeting", "hi"))
	changes.next(t)

	close(provider.recordFailure)

	failures.next(t)
	require.Equal(t, "configdirector", readiness.next(t).ProviderName)
	require.Equal(t, openfeature.ReadyState, fixture.client.State())

	require.NoError(t, fixture.testClient.SetValue("greeting", "hey"))

	changes.next(t)
	require.Empty(t, readiness.received())
}

type providerWhoseFailureOpenFeatureRecordsLate struct {
	*Provider
	failed        chan struct{}
	recordFailure chan struct{}
}

func (provider *providerWhoseFailureOpenFeatureRecordsLate) Init(evaluationContext openfeature.EvaluationContext) error {
	err := provider.Provider.Init(evaluationContext)
	close(provider.failed)
	<-provider.recordFailure
	return err
}

func TestShuttingDownEndsAPendingReadinessCheck(t *testing.T) {
	fixture := providerServing(t, map[string]any{"greeting": "hello"}, configdirectortest.HoldInitialization(), configdirectortest.WithTimeout(50*time.Millisecond))
	require.Error(t, fixture.api.SetProviderAndWait(t.Context(), fixture.provider))
	require.Eventually(t, func() bool { return readinessChecksRunning() == 1 }, awaitTimeout, 10*time.Millisecond)

	fixture.provider.Shutdown()

	require.Eventually(t, func() bool { return readinessChecksRunning() == 0 }, timeAllowedForOpenFeatureToRecordTheFailure/2, 10*time.Millisecond)
}

func awaitTheEndOfEveryReadinessCheck(t *testing.T) {
	t.Helper()
	require.Eventually(t, func() bool { return readinessChecksRunning() == 0 }, awaitTimeout, 10*time.Millisecond)
}

func readinessChecksRunning() int {
	return goroutinesIn("(*Provider).announceReadinessOnceTheFailureIsRecorded")
}

func TestReadinessInTimeIsAnnouncedOnce(t *testing.T) {
	t.Parallel()
	fixture := providerServing(t, map[string]any{"greeting": "hello"})
	readiness := recordEvents(fixture.client, openfeature.ProviderReady)
	changes := recordEvents(fixture.client, openfeature.ProviderConfigChange)
	fixture.register(t)
	readiness.next(t)
	changes.next(t)

	require.NoError(t, fixture.testClient.SetValue("greeting", "hi"))

	changes.next(t)
	require.Empty(t, readiness.received())
}

func TestInitializationFailsOnAProviderThatWasShutDown(t *testing.T) {
	t.Parallel()
	fixture := providerServing(t, map[string]any{"greeting": "hello"}, configdirectortest.HoldInitialization())
	fixture.provider.Shutdown()

	err := fixture.api.SetProviderAndWait(t.Context(), fixture.provider)

	require.ErrorIs(t, err, configdirector.ErrClientClosed)
}

func TestShuttingDownClosesTheClient(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{"greeting": "hello"})

	require.NoError(t, fixture.api.Shutdown(t.Context()))

	require.True(t, fixture.testClient.Client().Closed())
}

func TestShuttingDownReleasesAnUpdateWaitingForOpenFeature(t *testing.T) {
	t.Parallel()
	fixture := providerServing(t, map[string]any{"greeting": "hello"})
	delivered := make(chan struct{})
	go func() {
		_ = fixture.testClient.SetValue("greeting", "hi")
		close(delivered)
	}()
	require.Eventually(t, func() bool { return goroutinesIn("(*Provider).deliverToOpenFeature") == 1 }, awaitTimeout, 10*time.Millisecond)

	fixture.provider.Shutdown()

	await(t, delivered)
}

func goroutinesIn(frame string) int {
	buffer := make([]byte, 1<<16)
	for {
		written := runtime.Stack(buffer, true)
		if written < len(buffer) {
			return strings.Count(string(buffer[:written]), frame)
		}
		buffer = make([]byte, len(buffer)*2)
	}
}

func TestTheApplicationsHooksRunAfterTheProviders(t *testing.T) {
	t.Parallel()
	calls := []string{}
	hooks := hooksInOrder(
		configdirector.Hooks{
			OnReady:          func() { calls = append(calls, "provider ready") },
			OnConfigsUpdated: func(configdirector.ConfigsUpdated) { calls = append(calls, "provider updated") },
		},
		configdirector.Hooks{
			OnReady:           func() { calls = append(calls, "application ready") },
			OnConfigsUpdated:  func(configdirector.ConfigsUpdated) { calls = append(calls, "application updated") },
			OnConfigEvaluated: func(configdirector.Evaluation) { calls = append(calls, "application evaluated") },
		},
	)

	hooks.OnConfigsUpdated(configdirector.ConfigsUpdated{})
	hooks.OnReady()
	hooks.OnConfigEvaluated(configdirector.Evaluation{})

	require.Equal(t, []string{"provider updated", "application updated", "provider ready", "application ready", "application evaluated"}, calls)
}

func TestTheProviderNamesItselfToOpenFeatureAndAddsNoHooks(t *testing.T) {
	t.Parallel()
	fixture := providerServing(t, map[string]any{})

	require.Equal(t, "configdirector", fixture.provider.Metadata().Name)
	require.Empty(t, fixture.provider.Hooks())
}
