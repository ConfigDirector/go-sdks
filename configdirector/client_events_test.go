package configdirector_test

import (
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/stretchr/testify/require"
)

const betaSegmentID = "segment-beta"

var betaUser = configdirector.UserContext{ID: "u-1", Traits: map[string]any{"plan": "beta"}}

func newClientWithTestConnection(t *testing.T, options configdirector.Options) (*configdirector.Client, *configdirector.TestConnection) {
	t.Helper()
	client, connection, err := configdirector.NewClientWithTestConnection(options)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return client, connection
}

func newDefaultClientWithTestConnection(t *testing.T) (*configdirector.Client, *configdirector.TestConnection) {
	t.Helper()
	return newClientWithTestConnection(t, configdirector.Options{SDKKey: testSDKKey})
}

type recorder[T any] struct {
	mutex    sync.Mutex
	received []T
}

func (recorder *recorder[T]) record(value T) {
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	recorder.received = append(recorder.received, value)
}

func (recorder *recorder[T]) all() []T {
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	return append([]T{}, recorder.received...)
}

func awaitSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal("the handler was not called within 5 seconds")
	}
}

func recoveringPanics(t *testing.T, deliver func()) {
	t.Helper()
	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		deliver()
	}()
	require.Nil(t, <-done, "a panic escaped the client")
}

func TestAReadyHookInOptionsSeesTheReadyEvent(t *testing.T) {
	t.Parallel()
	ready := make(chan struct{})
	client := newClient(t, configdirector.Options{
		SDKKey: testSDKKey,
		Hooks:  configdirector.Hooks{OnReady: func() { close(ready) }},
	})

	awaitSignal(t, ready)

	require.True(t, client.Ready())
}

func TestAConfigsUpdatedHookInOptionsSeesTheFirstConfigState(t *testing.T) {
	t.Parallel()
	updates := &recorder[configdirector.ConfigsUpdated]{}
	_, connection := newClientWithTestConnection(t, configdirector.Options{
		SDKKey: testSDKKey,
		Hooks:  configdirector.Hooks{OnConfigsUpdated: updates.record},
	})

	connection.Deliver(configdirector.FullUpdate(configdirector.TextConfig("b", "2"), configdirector.TextConfig("a", "1")))

	require.Equal(t, []configdirector.ConfigsUpdated{{Keys: []string{"a", "b"}, RemovedKeys: []string{}}}, updates.all())
}

func TestAConfigEvaluatedHookInOptionsSeesReads(t *testing.T) {
	t.Parallel()
	evaluations := &recorder[configdirector.Evaluation]{}
	client, connection := newClientWithTestConnection(t, configdirector.Options{
		SDKKey: testSDKKey,
		Hooks:  configdirector.Hooks{OnConfigEvaluated: evaluations.record},
	})
	connection.Deliver(configdirector.FullUpdate(configdirector.TextConfig("welcome", "Hi")))

	client.StringValue("welcome", "fallback", noUser)

	require.Len(t, evaluations.all(), 1)
	require.Equal(t, "welcome", evaluations.all()[0].Key)
}

func TestAnOnReadyRegisteredAfterReadinessIsCalledAtOnce(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)
	calls := 0

	client.OnReady(func() { calls++ })

	require.Equal(t, 1, calls)
}

func TestOnReadyIsCalledOnceWhenTheFirstConfigStateArrives(t *testing.T) {
	t.Parallel()
	client, connection := newDefaultClientWithTestConnection(t)
	calls := &recorder[bool]{}
	client.OnReady(func() { calls.record(client.Ready()) })

	connection.Deliver(configdirector.FullUpdate(configdirector.TextConfig("a", "1")))
	connection.Deliver(configdirector.FullUpdate(configdirector.TextConfig("a", "2")))

	require.Equal(t, []bool{true}, calls.all())
}

func TestOnReadyAfterCloseNeverCallsTheHandler(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)
	require.NoError(t, client.Close())
	calls := 0

	client.OnReady(func() { calls++ })

	require.Equal(t, 0, calls)
}

func TestADeltaUpdateReportsTheKeysItCarries(t *testing.T) {
	t.Parallel()
	client, connection := newDefaultClientWithTestConnection(t)
	connection.Deliver(configdirector.FullUpdate(configdirector.TextConfig("a", "1"), configdirector.TextConfig("b", "2")))
	updates := &recorder[configdirector.ConfigsUpdated]{}
	client.OnConfigsUpdated(updates.record)

	connection.Deliver(configdirector.DeltaUpdate(configdirector.TextConfig("c", "3"), configdirector.TextConfig("b", "20")))

	require.Equal(t, []configdirector.ConfigsUpdated{{Keys: []string{"b", "c"}, RemovedKeys: []string{}}}, updates.all())
}

func TestAFullUpdateThatDropsAKeyReportsItRemoved(t *testing.T) {
	t.Parallel()
	client, connection := newDefaultClientWithTestConnection(t)
	connection.Deliver(configdirector.FullUpdate(configdirector.TextConfig("a", "1"), configdirector.TextConfig("b", "2")))
	updates := &recorder[configdirector.ConfigsUpdated]{}
	client.OnConfigsUpdated(updates.record)

	connection.Deliver(configdirector.FullUpdate(configdirector.TextConfig("a", "1")))

	require.Equal(t, []configdirector.ConfigsUpdated{{Keys: []string{"a"}, RemovedKeys: []string{"b"}}}, updates.all())
}

func TestAFullUpdateThatDropsAKeyFiresItsWatchWithTheDefault(t *testing.T) {
	t.Parallel()
	client, connection := newDefaultClientWithTestConnection(t)
	connection.Deliver(configdirector.FullUpdate(configdirector.TextConfig("a", "1"), configdirector.TextConfig("b", "2")))
	values := &recorder[string]{}
	client.WatchString("b", "fallback", noUser, values.record)

	connection.Deliver(configdirector.FullUpdate(configdirector.TextConfig("a", "1")))

	require.Equal(t, []string{"fallback"}, values.all())
}

func TestASegmentUpdateFiresTheWatchOfAConfigWhoseRulesUseIt(t *testing.T) {
	t.Parallel()
	client, connection := newDefaultClientWithTestConnection(t)
	connection.Deliver(configdirector.WithSegment(configdirector.FullUpdate(configdirector.BoolConfigTrueInSegment("beta-banner", betaSegmentID)), betaSegmentID, "gold"))
	values := &recorder[bool]{}
	client.WatchBool("beta-banner", false, betaUser, values.record)

	connection.Deliver(configdirector.WithSegment(configdirector.DeltaUpdate(), betaSegmentID, "beta"))

	require.Equal(t, []bool{true}, values.all())
}

func TestASegmentUpdateReportsTheKeysWhoseRulesUseIt(t *testing.T) {
	t.Parallel()
	client, connection := newDefaultClientWithTestConnection(t)
	connection.Deliver(configdirector.WithSegment(configdirector.FullUpdate(
		configdirector.BoolConfigTrueInSegment("beta-banner", betaSegmentID),
		configdirector.TextConfig("unrelated", "1"),
	), betaSegmentID, "gold"))
	updates := &recorder[configdirector.ConfigsUpdated]{}
	client.OnConfigsUpdated(updates.record)

	connection.Deliver(configdirector.WithSegment(configdirector.DeltaUpdate(), betaSegmentID, "beta"))

	require.Equal(t, []configdirector.ConfigsUpdated{{Keys: []string{"beta-banner"}, RemovedKeys: []string{}}}, updates.all())
}

func TestAWatchDoesNotFireForAnUpdateThatDoesNotCarryItsKey(t *testing.T) {
	t.Parallel()
	client, connection := newDefaultClientWithTestConnection(t)
	connection.Deliver(configdirector.FullUpdate(configdirector.TextConfig("a", "1")))
	values := &recorder[string]{}
	client.WatchString("a", "fallback", noUser, values.record)

	connection.Deliver(configdirector.DeltaUpdate(configdirector.TextConfig("b", "2")))

	require.Empty(t, values.all())
}

func TestWatchBoolFiresWithTheEvaluatedValue(t *testing.T) {
	t.Parallel()
	client, connection := newDefaultClientWithTestConnection(t)
	values := &recorder[bool]{}
	client.WatchBool("flag", false, noUser, values.record)

	connection.Deliver(configdirector.FullUpdate(configdirector.BoolConfig("flag", "true")))

	require.Equal(t, []bool{true}, values.all())
}

func TestWatchStringFiresWithTheEvaluatedValue(t *testing.T) {
	t.Parallel()
	client, connection := newDefaultClientWithTestConnection(t)
	values := &recorder[string]{}
	client.WatchString("greeting", "fallback", noUser, values.record)

	connection.Deliver(configdirector.FullUpdate(configdirector.TextConfig("greeting", "Hello")))

	require.Equal(t, []string{"Hello"}, values.all())
}

func TestWatchIntFiresWithTheEvaluatedValue(t *testing.T) {
	t.Parallel()
	client, connection := newDefaultClientWithTestConnection(t)
	values := &recorder[int64]{}
	client.WatchInt("max-items", 1, noUser, values.record)

	connection.Deliver(configdirector.FullUpdate(configdirector.IntConfig("max-items", "26")))

	require.Equal(t, []int64{26}, values.all())
}

func TestWatchFloatFiresWithTheEvaluatedValue(t *testing.T) {
	t.Parallel()
	client, connection := newDefaultClientWithTestConnection(t)
	values := &recorder[float64]{}
	client.WatchFloat("multiplier", 1, noUser, values.record)

	connection.Deliver(configdirector.FullUpdate(configdirector.FloatConfig("multiplier", "1.5")))

	require.Equal(t, []float64{1.5}, values.all())
}

func TestWatchJSONFiresWithTheDecodedTarget(t *testing.T) {
	t.Parallel()
	client, connection := newDefaultClientWithTestConnection(t)
	values := &recorder[limits]{}
	client.WatchJSON("limits", func() any { return &limits{Max: 5, Min: 5} }, noUser, func(target any) { values.record(*target.(*limits)) })

	connection.Deliver(configdirector.FullUpdate(configdirector.JSONConfig("limits", `{"max":20,"min":1}`)))

	require.Equal(t, []limits{{Max: 20, Min: 1}}, values.all())
}

func TestWatchJSONFiresWithTheDefaultTargetOnMalformedJSON(t *testing.T) {
	t.Parallel()
	client, connection := newDefaultClientWithTestConnection(t)
	values := &recorder[limits]{}
	client.WatchJSON("limits", func() any { return &limits{Max: 5, Min: 5} }, noUser, func(target any) { values.record(*target.(*limits)) })

	connection.Deliver(configdirector.FullUpdate(configdirector.JSONConfig("limits", "{not json")))

	require.Equal(t, []limits{{Max: 5, Min: 5}}, values.all())
}

func TestWatchJSONLogsAnErrorAndSkipsTheHandlerWhenTheTargetIsNotAPointer(t *testing.T) {
	t.Parallel()
	logger, records := newLogger()
	client, connection := newClientWithTestConnection(t, configdirector.Options{SDKKey: testSDKKey, Logger: logger})
	values := &recorder[any]{}
	client.WatchJSON("limits", func() any { return limits{} }, noUser, values.record)

	connection.Deliver(configdirector.FullUpdate(configdirector.JSONConfig("limits", `{"max":20,"min":1}`)))

	require.Empty(t, values.all())
	require.Len(t, recordsAt(slog.LevelError, records()), 1)
	require.Equal(t, "limits", attributesOf(recordsAt(slog.LevelError, records())[0])["key"])
}

func TestAWatchEvaluatesForItsOwnUserContext(t *testing.T) {
	t.Parallel()
	client, connection := newDefaultClientWithTestConnection(t)
	connection.Deliver(configdirector.WithSegment(configdirector.FullUpdate(configdirector.BoolConfigTrueInSegment("beta-banner", betaSegmentID)), betaSegmentID, "beta"))
	betaValues := &recorder[bool]{}
	otherValues := &recorder[bool]{}
	client.WatchBool("beta-banner", false, betaUser, betaValues.record)
	client.WatchBool("beta-banner", false, configdirector.UserContext{ID: "u-2", Traits: map[string]any{"plan": "free"}}, otherValues.record)

	connection.Deliver(configdirector.DeltaUpdate(configdirector.BoolConfigTrueInSegment("beta-banner", betaSegmentID)))

	require.Equal(t, []bool{true}, betaValues.all())
	require.Equal(t, []bool{false}, otherValues.all())
}

func TestAWatchNotificationFiresOnConfigEvaluated(t *testing.T) {
	t.Parallel()
	client, connection := newDefaultClientWithTestConnection(t)
	evaluations := &recorder[configdirector.Evaluation]{}
	client.OnConfigEvaluated(evaluations.record)
	client.WatchString("greeting", "fallback", noUser, func(string) {})

	connection.Deliver(configdirector.FullUpdate(configdirector.TextConfig("greeting", "Hello")))

	require.Equal(t, []configdirector.Evaluation{{
		Key:         "greeting",
		Value:       "Hello",
		UsedDefault: false,
		Reason:      configdirector.EvaluationReasonFoundMatch,
		ValueID:     "value-greeting-default",
		UserContext: noUser,
	}}, evaluations.all())
}

func TestAServedReadFiresOnConfigEvaluatedWithTheDetailsItReturns(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)
	evaluations := &recorder[configdirector.Evaluation]{}
	client.OnConfigEvaluated(evaluations.record)
	user := configdirector.UserContext{ID: "u-pro"}

	value, details := client.StringValueDetails("greeting", "fallback", user)

	require.Equal(t, []configdirector.Evaluation{{
		Key:         "greeting",
		Value:       "Hello, pro",
		UsedDefault: false,
		Reason:      configdirector.EvaluationReasonFoundMatch,
		ValueID:     "value-greeting-pro",
		UserContext: user,
	}}, evaluations.all())
	require.Equal(t, "Hello, pro", value)
	require.Equal(t, foundMatch("value-greeting-pro"), details)
}

func TestADefaultedReadFiresOnConfigEvaluatedWithTheDetailsItReturns(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)
	evaluations := &recorder[configdirector.Evaluation]{}
	client.OnConfigEvaluated(evaluations.record)

	value, details := client.BoolValueDetails("not-a-boolean", true, noUser)

	require.Equal(t, []configdirector.Evaluation{{
		Key:         "not-a-boolean",
		Value:       true,
		UsedDefault: true,
		Reason:      configdirector.EvaluationReasonInvalidBoolean,
		ValueID:     "",
		UserContext: noUser,
	}}, evaluations.all())
	require.True(t, value)
	require.Equal(t, usedDefault(configdirector.EvaluationReasonInvalidBoolean), details)
}

func TestAJSONReadFiresOnConfigEvaluatedWithTheDecodedValue(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)
	evaluations := &recorder[configdirector.Evaluation]{}
	client.OnConfigEvaluated(evaluations.record)
	target := limits{}

	require.NoError(t, client.JSONValue("limits", &target, noUser))

	require.Len(t, evaluations.all(), 1)
	require.Equal(t, limits{Max: 20, Min: 1}, evaluations.all()[0].Value)
	require.Equal(t, configdirector.EvaluationReasonFoundMatch, evaluations.all()[0].Reason)
}

func TestAReadBeforeReadyFiresOnConfigEvaluatedWithClientNotReady(t *testing.T) {
	t.Parallel()
	client, _ := newDefaultClientWithTestConnection(t)
	evaluations := &recorder[configdirector.Evaluation]{}
	client.OnConfigEvaluated(evaluations.record)

	client.IntValue("max-items", 7, noUser)

	require.Len(t, evaluations.all(), 1)
	require.Equal(t, configdirector.EvaluationReasonClientNotReady, evaluations.all()[0].Reason)
	require.Equal(t, int64(7), evaluations.all()[0].Value)
}

func TestAPanickingHandlerIsLoggedAndTheNextUpdateStillReachesOtherHandlers(t *testing.T) {
	t.Parallel()
	logger, records := newLogger()
	client, connection := newClientWithTestConnection(t, configdirector.Options{SDKKey: testSDKKey, Logger: logger})
	client.OnConfigsUpdated(func(configdirector.ConfigsUpdated) { panic("handler failure") })
	updates := &recorder[configdirector.ConfigsUpdated]{}
	client.OnConfigsUpdated(updates.record)

	recoveringPanics(t, func() { connection.Deliver(configdirector.FullUpdate(configdirector.TextConfig("a", "1"))) })
	recoveringPanics(t, func() { connection.Deliver(configdirector.DeltaUpdate(configdirector.TextConfig("a", "2"))) })

	require.Len(t, updates.all(), 2)
	require.Len(t, recordsAt(slog.LevelError, records()), 2)
	require.Equal(t, "configs-updated", attributesOf(recordsAt(slog.LevelError, records())[0])["event"])
	require.Equal(t, "handler failure", attributesOf(recordsAt(slog.LevelError, records())[0])["panic"])
}

func TestAPanickingWatchHandlerIsLoggedWithItsKeyAndOtherWatchesStillFire(t *testing.T) {
	t.Parallel()
	logger, records := newLogger()
	client, connection := newClientWithTestConnection(t, configdirector.Options{SDKKey: testSDKKey, Logger: logger})
	client.WatchString("a", "fallback", noUser, func(string) { panic("watch failure") })
	values := &recorder[string]{}
	client.WatchString("a", "fallback", noUser, values.record)

	recoveringPanics(t, func() { connection.Deliver(configdirector.FullUpdate(configdirector.TextConfig("a", "1"))) })

	require.Equal(t, []string{"1"}, values.all())
	require.Len(t, recordsAt(slog.LevelError, records()), 1)
	require.Equal(t, "watch", attributesOf(recordsAt(slog.LevelError, records())[0])["event"])
	require.Equal(t, "a", attributesOf(recordsAt(slog.LevelError, records())[0])["key"])
}

func TestAPanickingReadyHandlerDoesNotStopTheClientBecomingReady(t *testing.T) {
	t.Parallel()
	logger, records := newLogger()
	client, connection := newClientWithTestConnection(t, configdirector.Options{SDKKey: testSDKKey, Logger: logger})
	client.OnReady(func() { panic("ready failure") })

	recoveringPanics(t, func() { connection.Deliver(configdirector.FullUpdate(configdirector.TextConfig("a", "1"))) })

	require.True(t, client.Ready())
	require.Len(t, recordsAt(slog.LevelError, records()), 1)
	require.Equal(t, "ready", attributesOf(recordsAt(slog.LevelError, records())[0])["event"])
}

func TestUnsubscribingAHandlerStopsDelivery(t *testing.T) {
	t.Parallel()
	client, connection := newDefaultClientWithTestConnection(t)
	updates := &recorder[configdirector.ConfigsUpdated]{}
	unsubscribe := client.OnConfigsUpdated(updates.record)
	connection.Deliver(configdirector.FullUpdate(configdirector.TextConfig("a", "1")))

	unsubscribe()
	connection.Deliver(configdirector.DeltaUpdate(configdirector.TextConfig("a", "2")))

	require.Len(t, updates.all(), 1)
}

func TestUnsubscribingAWatchStopsDelivery(t *testing.T) {
	t.Parallel()
	client, connection := newDefaultClientWithTestConnection(t)
	values := &recorder[string]{}
	unsubscribe := client.WatchString("a", "fallback", noUser, values.record)
	connection.Deliver(configdirector.FullUpdate(configdirector.TextConfig("a", "1")))

	unsubscribe()
	connection.Deliver(configdirector.DeltaUpdate(configdirector.TextConfig("a", "2")))

	require.Equal(t, []string{"1"}, values.all())
}

func TestUnsubscribingOneWatchLeavesTheOthersOnTheSameKey(t *testing.T) {
	t.Parallel()
	client, connection := newDefaultClientWithTestConnection(t)
	first := &recorder[string]{}
	second := &recorder[string]{}
	unsubscribeFirst := client.WatchString("a", "fallback", noUser, first.record)
	client.WatchString("a", "fallback", noUser, second.record)

	unsubscribeFirst()
	connection.Deliver(configdirector.FullUpdate(configdirector.TextConfig("a", "1")))

	require.Empty(t, first.all())
	require.Equal(t, []string{"1"}, second.all())
}

func TestUnwatchAllStopsEveryWatch(t *testing.T) {
	t.Parallel()
	client, connection := newDefaultClientWithTestConnection(t)
	values := &recorder[string]{}
	client.WatchString("a", "fallback", noUser, values.record)
	client.WatchString("b", "fallback", noUser, values.record)

	client.UnwatchAll()
	connection.Deliver(configdirector.FullUpdate(configdirector.TextConfig("a", "1"), configdirector.TextConfig("b", "2")))

	require.Empty(t, values.all())
}

func TestCloseStopsEvaluatedHandlersFromBeingCalled(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)
	evaluations := &recorder[configdirector.Evaluation]{}
	client.OnConfigEvaluated(evaluations.record)
	require.NoError(t, client.Close())

	client.BoolValue("new-checkout", false, noUser)

	require.Empty(t, evaluations.all())
}

func TestRegisteringANilHandlerPanics(t *testing.T) {
	t.Parallel()
	client, _ := newDefaultClientWithTestConnection(t)

	require.Panics(t, func() { client.OnConfigsUpdated(nil) })
}

func TestWatchingWithANilHandlerPanics(t *testing.T) {
	t.Parallel()
	client, _ := newDefaultClientWithTestConnection(t)

	require.Panics(t, func() { client.WatchBool("a", false, noUser, nil) })
}
