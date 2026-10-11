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

func newReadyClientAgainst(t *testing.T, initialPayload payloadBuilder) (*configdirector.Client, *sdkServer) {
	t.Helper()
	server := startSDKServer(t, initialPayload.json())
	client := newClientAgainst(t, server)
	waitForReady(t, client)
	return client, server
}

func newLoggedClient(t *testing.T, server *sdkServer) (*configdirector.Client, func() []slog.Record) {
	t.Helper()
	logger, records := newLogger()
	options := server.options()
	options.Logger = logger
	return newClient(t, options), records
}

func TestAReadyHookInOptionsSeesTheReadyEvent(t *testing.T) {
	t.Parallel()
	ready := make(chan struct{})
	options := startSDKServer(t, standardPayload().json()).options()
	options.Hooks = configdirector.Hooks{OnReady: func() { close(ready) }}
	client := newClient(t, options)

	await(t, ready)

	require.True(t, client.Ready())
}

func TestAConfigsUpdatedHookInOptionsSeesTheFirstConfigState(t *testing.T) {
	t.Parallel()
	updates := newRecorder[configdirector.ConfigsUpdated]()
	options := startSDKServer(t, fullPayload(textConfig("b", "2"), textConfig("a", "1")).json()).options()
	options.Hooks = configdirector.Hooks{OnConfigsUpdated: updates.record}

	newClient(t, options)

	require.Equal(t, configdirector.ConfigsUpdated{Keys: []string{"a", "b"}, RemovedKeys: []string{}}, updates.next(t))
}

func TestAConfigEvaluatedHookInOptionsSeesReads(t *testing.T) {
	t.Parallel()
	evaluations := newRecorder[configdirector.Evaluation]()
	options := startSDKServer(t, fullPayload(textConfig("welcome", "Hi")).json()).options()
	options.Hooks = configdirector.Hooks{OnConfigEvaluated: evaluations.record}
	client := newClient(t, options)
	waitForReady(t, client)

	client.StringValue("welcome", "fallback", noUser)

	evaluated := evaluations.received()
	require.Len(t, evaluated, 1)
	require.Equal(t, "welcome", evaluated[0].Key)
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
	server := startSDKServer(t)
	client := newClientAgainst(t, server)
	calls := newRecorder[bool]()
	client.OnReady(func() { calls.record(client.Ready()) })
	updates := newRecorder[configdirector.ConfigsUpdated]()
	client.OnConfigsUpdated(updates.record)
	await(t, server.requests)

	server.send(fullPayload(textConfig("a", "1")).json())
	server.send(fullPayload(textConfig("a", "2")).json())

	updates.next(t)
	updates.next(t)
	require.Equal(t, []bool{true}, calls.received())
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
	client, server := newReadyClientAgainst(t, fullPayload(textConfig("a", "1"), textConfig("b", "2")))
	updates := newRecorder[configdirector.ConfigsUpdated]()
	client.OnConfigsUpdated(updates.record)

	server.send(deltaPayload(textConfig("c", "3"), textConfig("b", "20")).json())

	require.Equal(t, configdirector.ConfigsUpdated{Keys: []string{"b", "c"}, RemovedKeys: []string{}}, updates.next(t))
}

func TestADeltaUpdateAddsAConfigAndFiresItsWatch(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, fullPayload(textConfig("a", "1")))
	values := newRecorder[string]()
	client.WatchString("c", "fallback", noUser, values.record)

	server.send(deltaPayload(textConfig("c", "3")).json())

	require.Equal(t, "3", values.next(t))
	require.Equal(t, "3", client.StringValue("c", "fallback", noUser))
	require.Equal(t, "1", client.StringValue("a", "fallback", noUser))
}

func TestAFullUpdateThatDropsAKeyReportsItRemoved(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, fullPayload(textConfig("a", "1"), textConfig("b", "2")))
	updates := newRecorder[configdirector.ConfigsUpdated]()
	client.OnConfigsUpdated(updates.record)

	server.send(fullPayload(textConfig("a", "1")).json())

	require.Equal(t, configdirector.ConfigsUpdated{Keys: []string{"a"}, RemovedKeys: []string{"b"}}, updates.next(t))
	require.Equal(t, "fallback", client.StringValue("b", "fallback", noUser))
}

func TestAFullUpdateThatDropsAKeyFiresItsWatchWithTheDefault(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, fullPayload(textConfig("a", "1"), textConfig("b", "2")))
	values := newRecorder[string]()
	client.WatchString("b", "fallback", noUser, values.record)

	server.send(fullPayload(textConfig("a", "1")).json())

	require.Equal(t, "fallback", values.next(t))
}

func TestASegmentUpdateFiresTheWatchOfAConfigWhoseRulesUseIt(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, fullPayload(boolConfigTrueInSegment("beta-banner", betaSegmentID)).withSegment(betaSegmentID, "gold"))
	values := newRecorder[bool]()
	client.WatchBool("beta-banner", false, betaUser, values.record)

	server.send(deltaPayload().withSegment(betaSegmentID, "beta").json())

	require.True(t, values.next(t))
	require.True(t, client.BoolValue("beta-banner", false, betaUser))
}

func TestASegmentUpdateReportsTheKeysWhoseRulesUseIt(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, fullPayload(
		boolConfigTrueInSegment("beta-banner", betaSegmentID),
		textConfig("unrelated", "1"),
	).withSegment(betaSegmentID, "gold"))
	updates := newRecorder[configdirector.ConfigsUpdated]()
	client.OnConfigsUpdated(updates.record)

	server.send(deltaPayload().withSegment(betaSegmentID, "beta").json())

	require.Equal(t, configdirector.ConfigsUpdated{Keys: []string{"beta-banner"}, RemovedKeys: []string{}}, updates.next(t))
}

func TestAWatchDoesNotFireForAnUpdateThatDoesNotCarryItsKey(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, fullPayload(textConfig("a", "1")))
	values := newRecorder[string]()
	client.WatchString("a", "fallback", noUser, values.record)

	server.send(deltaPayload(textConfig("b", "2")).json())
	server.send(deltaPayload(textConfig("a", "10")).json())

	require.Equal(t, "10", values.next(t))
	require.Empty(t, values.received())
}

func TestWatchBoolFiresWithTheEvaluatedValue(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, fullPayload())
	values := newRecorder[bool]()
	client.WatchBool("flag", false, noUser, values.record)

	server.send(fullPayload(boolConfig("flag", "true")).json())

	require.True(t, values.next(t))
}

func TestWatchStringFiresWithTheEvaluatedValue(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, fullPayload())
	values := newRecorder[string]()
	client.WatchString("greeting", "fallback", noUser, values.record)

	server.send(fullPayload(textConfig("greeting", "Hello")).json())

	require.Equal(t, "Hello", values.next(t))
}

func TestWatchIntFiresWithTheEvaluatedValue(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, fullPayload())
	values := newRecorder[int64]()
	client.WatchInt("max-items", 1, noUser, values.record)

	server.send(fullPayload(intConfig("max-items", "26")).json())

	require.Equal(t, int64(26), values.next(t))
}

func TestWatchFloatFiresWithTheEvaluatedValue(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, fullPayload())
	values := newRecorder[float64]()
	client.WatchFloat("multiplier", 1, noUser, values.record)

	server.send(fullPayload(floatConfig("multiplier", "1.5")).json())

	require.InDelta(t, 1.5, values.next(t), 0)
}

func TestWatchJSONFiresWithTheDecodedTarget(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, fullPayload())
	values := newRecorder[limits]()
	client.WatchJSON("limits", func() any { return &limits{Max: 5, Min: 5} }, noUser, func(target any) { values.record(*target.(*limits)) })

	server.send(fullPayload(jsonConfig("limits", `{"max":20,"min":1}`)).json())

	require.Equal(t, limits{Max: 20, Min: 1}, values.next(t))
}

func TestWatchJSONFiresWithTheDefaultTargetOnMalformedJSON(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, fullPayload())
	values := newRecorder[limits]()
	client.WatchJSON("limits", func() any { return &limits{Max: 5, Min: 5} }, noUser, func(target any) { values.record(*target.(*limits)) })

	server.send(fullPayload(jsonConfig("limits", "{not json")).json())

	require.Equal(t, limits{Max: 5, Min: 5}, values.next(t))
}

func TestWatchJSONLogsAnErrorAndSkipsTheHandlerWhenTheTargetIsNotAPointer(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, fullPayload().json())
	client, records := newLoggedClient(t, server)
	waitForReady(t, client)
	values := newRecorder[any]()
	client.WatchJSON("limits", func() any { return limits{} }, noUser, values.record)

	server.send(fullPayload(jsonConfig("limits", `{"max":20,"min":1}`)).json())

	require.Eventually(t, func() bool { return len(recordsAt(slog.LevelError, records())) == 1 }, awaitTimeout, 10*time.Millisecond)
	require.Equal(t, "limits", attributesOf(recordsAt(slog.LevelError, records())[0])["key"])
	require.Empty(t, values.received())
}

func TestAWatchEvaluatesForItsOwnUserContext(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, fullPayload(boolConfigTrueInSegment("beta-banner", betaSegmentID)).withSegment(betaSegmentID, "beta"))
	betaValues := newRecorder[bool]()
	otherValues := newRecorder[bool]()
	client.WatchBool("beta-banner", false, betaUser, betaValues.record)
	client.WatchBool("beta-banner", false, configdirector.UserContext{ID: "u-2", Traits: map[string]any{"plan": "free"}}, otherValues.record)

	server.send(deltaPayload(boolConfigTrueInSegment("beta-banner", betaSegmentID)).json())

	require.True(t, betaValues.next(t))
	require.False(t, otherValues.next(t))
}

func TestAWatchNotificationFiresOnConfigEvaluated(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, fullPayload())
	evaluations := newRecorder[configdirector.Evaluation]()
	client.OnConfigEvaluated(evaluations.record)
	client.WatchString("greeting", "fallback", noUser, func(string) {})

	server.send(fullPayload(textConfig("greeting", "Hello")).json())

	require.Equal(t, configdirector.Evaluation{
		Key:         "greeting",
		Value:       "Hello",
		UsedDefault: false,
		Reason:      configdirector.EvaluationReasonFoundMatch,
		ValueID:     "value-greeting-default",
		UserContext: noUser,
	}, evaluations.next(t))
}

func TestAServedReadFiresOnConfigEvaluatedWithTheDetailsItReturns(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)
	evaluations := newRecorder[configdirector.Evaluation]()
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
	}}, evaluations.received())
	require.Equal(t, "Hello, pro", value)
	require.Equal(t, foundMatch("value-greeting-pro"), details)
}

func TestADefaultedReadFiresOnConfigEvaluatedWithTheDetailsItReturns(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)
	evaluations := newRecorder[configdirector.Evaluation]()
	client.OnConfigEvaluated(evaluations.record)

	value, details := client.BoolValueDetails("not-a-boolean", true, noUser)

	require.Equal(t, []configdirector.Evaluation{{
		Key:         "not-a-boolean",
		Value:       true,
		UsedDefault: true,
		Reason:      configdirector.EvaluationReasonInvalidBoolean,
		ValueID:     "",
		UserContext: noUser,
	}}, evaluations.received())
	require.True(t, value)
	require.Equal(t, usedDefault(configdirector.EvaluationReasonInvalidBoolean), details)
}

func TestAJSONReadFiresOnConfigEvaluatedWithTheDecodedValue(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)
	evaluations := newRecorder[configdirector.Evaluation]()
	client.OnConfigEvaluated(evaluations.record)
	target := limits{}

	require.NoError(t, client.JSONValue("limits", &target, noUser))

	evaluated := evaluations.received()
	require.Len(t, evaluated, 1)
	require.Equal(t, limits{Max: 20, Min: 1}, evaluated[0].Value)
	require.Equal(t, configdirector.EvaluationReasonFoundMatch, evaluated[0].Reason)
}

func TestAReadBeforeReadyFiresOnConfigEvaluatedWithClientNotReady(t *testing.T) {
	t.Parallel()
	client := newClientAgainst(t, startSDKServer(t))
	evaluations := newRecorder[configdirector.Evaluation]()
	client.OnConfigEvaluated(evaluations.record)

	client.IntValue("max-items", 7, noUser)

	evaluated := evaluations.received()
	require.Len(t, evaluated, 1)
	require.Equal(t, configdirector.EvaluationReasonClientNotReady, evaluated[0].Reason)
	require.Equal(t, int64(7), evaluated[0].Value)
}

func TestAPanickingHandlerIsLoggedAndTheNextUpdateStillReachesOtherHandlers(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t)
	client, records := newLoggedClient(t, server)
	client.OnConfigsUpdated(func(configdirector.ConfigsUpdated) { panic("handler failure") })
	updates := newRecorder[configdirector.ConfigsUpdated]()
	client.OnConfigsUpdated(updates.record)
	await(t, server.requests)

	server.send(fullPayload(textConfig("a", "1")).json())
	server.send(deltaPayload(textConfig("a", "2")).json())

	updates.next(t)
	updates.next(t)
	require.Len(t, recordsAt(slog.LevelError, records()), 2)
	require.Equal(t, "configs-updated", attributesOf(recordsAt(slog.LevelError, records())[0])["event"])
	require.Equal(t, "handler failure", attributesOf(recordsAt(slog.LevelError, records())[0])["panic"])
}

func TestAPanickingWatchHandlerIsLoggedWithItsKeyAndOtherWatchesStillFire(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t)
	client, records := newLoggedClient(t, server)
	client.WatchString("a", "fallback", noUser, func(string) { panic("watch failure") })
	values := newRecorder[string]()
	client.WatchString("a", "fallback", noUser, values.record)
	await(t, server.requests)

	server.send(fullPayload(textConfig("a", "1")).json())

	require.Equal(t, "1", values.next(t))
	require.Len(t, recordsAt(slog.LevelError, records()), 1)
	require.Equal(t, "watch", attributesOf(recordsAt(slog.LevelError, records())[0])["event"])
	require.Equal(t, "a", attributesOf(recordsAt(slog.LevelError, records())[0])["key"])
}

func TestAPanickingReadyHandlerDoesNotStopTheClientBecomingReady(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t)
	client, records := newLoggedClient(t, server)
	client.OnReady(func() { panic("ready failure") })
	await(t, server.requests)

	server.send(fullPayload(textConfig("a", "1")).json())

	waitForReady(t, client)
	require.Eventually(t, func() bool { return len(recordsAt(slog.LevelError, records())) == 1 }, awaitTimeout, 10*time.Millisecond)
	require.Equal(t, "ready", attributesOf(recordsAt(slog.LevelError, records())[0])["event"])
}

func TestAHandlerMayReadAValueFromInsideOnConfigsUpdated(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t)
	client := newClientAgainst(t, server)
	values := newRecorder[bool]()
	client.OnConfigsUpdated(func(configdirector.ConfigsUpdated) { values.record(client.BoolValue("a", false, noUser)) })
	await(t, server.requests)

	server.send(fullPayload(boolConfig("a", "true")).json())

	require.True(t, values.next(t))
}

func TestAWatchHandlerMayReadAnotherValueFromInsideItsNotification(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t)
	client := newClientAgainst(t, server)
	values := newRecorder[string]()
	client.WatchBool("a", false, noUser, func(bool) { values.record(client.StringValue("b", "fallback", noUser)) })
	await(t, server.requests)

	server.send(fullPayload(boolConfig("a", "true"), textConfig("b", "served")).json())

	require.Equal(t, "served", values.next(t))
}

func TestAHandlerMayRegisterAWatchFromInsideOnConfigsUpdated(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t)
	client := newClientAgainst(t, server)
	values := newRecorder[string]()
	var once sync.Once
	client.OnConfigsUpdated(func(configdirector.ConfigsUpdated) {
		once.Do(func() { client.WatchString("a", "fallback", noUser, values.record) })
	})
	await(t, server.requests)

	server.send(fullPayload(textConfig("a", "1")).json())
	server.send(deltaPayload(textConfig("a", "2")).json())

	require.Equal(t, "2", values.next(t))
}

func TestUnsubscribingAHandlerStopsDelivery(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, fullPayload(textConfig("a", "1")))
	updates := newRecorder[configdirector.ConfigsUpdated]()
	unsubscribe := client.OnConfigsUpdated(updates.record)
	later := newRecorder[configdirector.ConfigsUpdated]()
	client.OnConfigsUpdated(later.record)

	unsubscribe()
	server.send(deltaPayload(textConfig("a", "2")).json())

	later.next(t)
	require.Empty(t, updates.received())
}

func TestUnsubscribingAWatchStopsDelivery(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, fullPayload(textConfig("a", "1")))
	values := newRecorder[string]()
	unsubscribe := client.WatchString("a", "fallback", noUser, values.record)
	later := newRecorder[string]()
	client.WatchString("a", "fallback", noUser, later.record)

	unsubscribe()
	server.send(deltaPayload(textConfig("a", "2")).json())

	require.Equal(t, "2", later.next(t))
	require.Empty(t, values.received())
}

func TestUnsubscribingOneWatchLeavesTheOthersOnTheSameKey(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, fullPayload())
	first := newRecorder[string]()
	second := newRecorder[string]()
	unsubscribeFirst := client.WatchString("a", "fallback", noUser, first.record)
	client.WatchString("a", "fallback", noUser, second.record)

	unsubscribeFirst()
	server.send(fullPayload(textConfig("a", "1")).json())

	require.Equal(t, "1", second.next(t))
	require.Empty(t, first.received())
}

func TestUnwatchAllStopsEveryWatch(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, fullPayload())
	values := newRecorder[string]()
	client.WatchString("a", "fallback", noUser, values.record)
	client.WatchString("b", "fallback", noUser, values.record)
	updates := newRecorder[configdirector.ConfigsUpdated]()
	client.OnConfigsUpdated(updates.record)

	client.UnwatchAll()
	server.send(fullPayload(textConfig("a", "1"), textConfig("b", "2")).json())

	updates.next(t)
	require.Empty(t, values.received())
}

func TestCloseStopsEvaluatedHandlersFromBeingCalled(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)
	evaluations := newRecorder[configdirector.Evaluation]()
	client.OnConfigEvaluated(evaluations.record)
	require.NoError(t, client.Close())

	client.BoolValue("new-checkout", false, noUser)

	require.Empty(t, evaluations.received())
}

func TestRegisteringANilHandlerPanics(t *testing.T) {
	t.Parallel()
	client := newClientAgainst(t, startSDKServer(t))

	require.Panics(t, func() { client.OnConfigsUpdated(nil) })
}

func TestWatchingWithANilHandlerPanics(t *testing.T) {
	t.Parallel()
	client := newClientAgainst(t, startSDKServer(t))

	require.Panics(t, func() { client.WatchBool("a", false, noUser, nil) })
}
