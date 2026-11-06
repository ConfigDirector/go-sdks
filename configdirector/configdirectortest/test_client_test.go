package configdirectortest_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/ConfigDirector/go-sdks/configdirector/configdirectortest"
	"github.com/stretchr/testify/require"
)

var (
	noUser  = configdirector.UserContext{}
	proUser = configdirector.UserContext{ID: "user-123", Traits: map[string]any{"plan": "pro"}}
)

type theme struct {
	Color string `json:"color"`
}

func newTestClient(t *testing.T, values map[string]any, options ...configdirectortest.Option) *configdirectortest.TestClient {
	t.Helper()
	testClient := configdirectortest.NewTestClient(values, options...)
	t.Cleanup(func() { require.NoError(t, testClient.Close()) })
	return testClient
}

func usedDefault(reason configdirector.EvaluationReason) configdirector.EvaluationDetails {
	return configdirector.EvaluationDetails{Reason: reason, ValueID: "", UsedDefault: true}
}

func waitForReadyIn(client *configdirector.Client) <-chan error {
	result := make(chan error, 1)
	go func() { result <- client.WaitForReady(context.Background()) }()
	return result
}

func requireNoResultWithin(t *testing.T, result <-chan error, wait time.Duration) {
	t.Helper()
	select {
	case err := <-result:
		t.Fatalf("WaitForReady returned %v while initialization was held", err)
	case <-time.After(wait):
	}
}

func requireResultWithin(t *testing.T, result <-chan error, wait time.Duration) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(wait):
		t.Fatal("WaitForReady did not return")
		return nil
	}
}

type recordingHandler struct {
	mutex   sync.Mutex
	records []slog.Record
}

func newLogger() (*slog.Logger, func() []slog.Record) {
	handler := &recordingHandler{}
	return slog.New(handler), func() []slog.Record {
		handler.mutex.Lock()
		defer handler.mutex.Unlock()
		return append([]slog.Record{}, handler.records...)
	}
}

func (*recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (handler *recordingHandler) Handle(_ context.Context, record slog.Record) error {
	handler.mutex.Lock()
	defer handler.mutex.Unlock()
	handler.records = append(handler.records, record)
	return nil
}

func (handler *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return handler }

func (handler *recordingHandler) WithGroup(string) slog.Handler { return handler }

func recordsAt(level slog.Level, records []slog.Record) []slog.Record {
	var matching []slog.Record
	for _, record := range records {
		if record.Level == level {
			matching = append(matching, record)
		}
	}
	return matching
}

func TestTheClientIsReadyWhenNewTestClientReturns(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{"new-checkout": true})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := testClient.Client().WaitForReady(ctx)

	require.NoError(t, err)
	require.True(t, testClient.Client().Ready())
}

func TestABoolIsReadWithBoolValue(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, map[string]any{"new-checkout": true}).Client()

	value := client.BoolValue("new-checkout", false, noUser)

	require.True(t, value)
}

func TestAStringIsReadWithStringValue(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, map[string]any{"greeting": "Hello"}).Client()

	value := client.StringValue("greeting", "fallback", noUser)

	require.Equal(t, "Hello", value)
}

func TestAnIntIsReadWithIntValue(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, map[string]any{"max-items": 26}).Client()

	value := client.IntValue("max-items", 1, noUser)

	require.Equal(t, int64(26), value)
}

func TestAFloat64IsReadWithFloatValue(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, map[string]any{"ratio": 2.5}).Client()

	value := client.FloatValue("ratio", 1, noUser)

	require.InDelta(t, 2.5, value, 0)
}

func TestAStructIsReadWithJSONValue(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, map[string]any{"theme": theme{Color: "blue"}}).Client()

	decoded := theme{}
	err := client.JSONValue("theme", &decoded, noUser)

	require.NoError(t, err)
	require.Equal(t, theme{Color: "blue"}, decoded)
}

func TestEveryContextReceivesTheSameValue(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, map[string]any{"greeting": "Hello"}).Client()

	require.Equal(t, "Hello", client.StringValue("greeting", "fallback", noUser))
	require.Equal(t, "Hello", client.StringValue("greeting", "fallback", proUser))
	require.Equal(t, "Hello", client.StringValue("greeting", "fallback", configdirector.UserContext{ID: "other", Anonymous: true}))
}

func TestEveryReadWithAnInvalidContextKindIsLoggedAndServedTheValue(t *testing.T) {
	t.Parallel()
	logger, records := newLogger()
	testClient := newTestClient(t, map[string]any{"new-checkout": true}, configdirectortest.WithLogger(logger))
	invalidKind := configdirector.UserContext{ID: "user-123", Kind: "Device"}

	first := testClient.Client().BoolValue("new-checkout", false, invalidKind)
	second := testClient.Client().BoolValue("new-checkout", false, invalidKind)

	require.True(t, first)
	require.True(t, second)
	require.Len(t, recordsAt(slog.LevelError, records()), 2)
}

func TestAServedValueReportsAFoundMatch(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, map[string]any{"new-checkout": true}).Client()

	value, details := client.BoolValueDetails("new-checkout", false, noUser)

	require.True(t, value)
	require.Equal(t, configdirector.EvaluationReasonFoundMatch, details.Reason)
	require.False(t, details.UsedDefault)
	require.NotEmpty(t, details.ValueID)
}

func TestEvaluateOfATestValueIsAFoundMatchWithoutAVariation(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{"new-checkout": true})

	evaluated, err := testClient.Client().Evaluate("new-checkout", false, noUser)

	require.NoError(t, err)
	require.Equal(t, configdirector.Evaluation{
		Key:     "new-checkout",
		Value:   true,
		Reason:  configdirector.EvaluationReasonFoundMatch,
		ValueID: "test-value:new-checkout",
	}, evaluated)
}

func TestEvaluateReadsATestValueAboveTheInt64Range(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{"big": uint64(1 << 63)})

	evaluated, err := testClient.Client().Evaluate("big", uint64(7), noUser)

	require.NoError(t, err)
	require.Equal(t, uint64(1<<63), evaluated.Value)
}

func TestAContextWithoutAnIDIsServedATestValueWithAFoundMatch(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{"new-checkout": true})

	value, details := testClient.Client().BoolValueDetails("new-checkout", false, noUser)

	require.True(t, value)
	require.Equal(t, configdirector.EvaluationReasonFoundMatch, details.Reason)
	require.Nil(t, details.Variation)
}

func TestABoolReadAsAStringReturnsTheDefaultWithTypeMismatch(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, map[string]any{"new-checkout": true}).Client()

	value, details := client.StringValueDetails("new-checkout", "fallback", noUser)

	require.Equal(t, "fallback", value)
	require.Equal(t, usedDefault(configdirector.EvaluationReasonTypeMismatch), details)
}

func TestAnEmptyStringServesTheDefaultWithValueMissing(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, map[string]any{"greeting": ""}).Client()

	value, details := client.StringValueDetails("greeting", "fallback", noUser)

	require.Equal(t, "fallback", value)
	require.Equal(t, usedDefault(configdirector.EvaluationReasonValueMissing), details)
}

func TestANarrowSignedIntegerIsAnIntegerConfig(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, map[string]any{"small": int8(-3)}).Client()

	value := client.IntValue("small", 1, noUser)

	require.Equal(t, int64(-3), value)
}

func TestAnUnsignedIntegerIsAnIntegerConfig(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, map[string]any{"unsigned": uint16(500)}).Client()

	value := client.IntValue("unsigned", 1, noUser)

	require.Equal(t, int64(500), value)
}

func TestADurationIsAnIntegerConfig(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, map[string]any{"duration": 90 * time.Second}).Client()

	value := client.IntValue("duration", 1, noUser)

	require.Equal(t, int64(90*time.Second), value)
}

func TestAFloat32IsAFloatConfigWithItsShortestText(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, map[string]any{"single": float32(0.1)}).Client()

	value := client.FloatValue("single", 1, noUser)

	require.InDelta(t, 0.1, value, 0)
}

func TestAMapIsAJSONConfig(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, map[string]any{"limits": map[string]any{"max": 3, "names": []string{"a", "b"}}}).Client()

	limits := map[string]any{}
	err := client.JSONValue("limits", &limits, noUser)

	require.NoError(t, err)
	require.Equal(t, map[string]any{"max": float64(3), "names": []any{"a", "b"}}, limits)
}

func TestASliceIsAJSONConfig(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, map[string]any{"order": []int{3, 1, 2}}).Client()

	order := []int{}
	err := client.JSONValue("order", &order, noUser)

	require.NoError(t, err)
	require.Equal(t, []int{3, 1, 2}, order)
}

func TestAllConfigsReportsTheConfigTypeOfEveryValue(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, map[string]any{
		"new-checkout": true,
		"greeting":     "Hello",
		"max-items":    26,
		"ratio":        2.5,
		"theme":        theme{Color: "blue"},
	}).Client()

	states := client.AllConfigs(proUser)

	require.Equal(t, []configdirector.ConfigState{
		{ID: "test-config:greeting", Key: "greeting", Type: configdirector.ConfigTypeString, Value: "Hello", ValueID: "test-value:greeting"},
		{ID: "test-config:max-items", Key: "max-items", Type: configdirector.ConfigTypeInteger, Value: "26", ValueID: "test-value:max-items"},
		{ID: "test-config:new-checkout", Key: "new-checkout", Type: configdirector.ConfigTypeBoolean, Value: "true", ValueID: "test-value:new-checkout"},
		{ID: "test-config:ratio", Key: "ratio", Type: configdirector.ConfigTypeFloat, Value: "2.5", ValueID: "test-value:ratio"},
		{ID: "test-config:theme", Key: "theme", Type: configdirector.ConfigTypeJSON, Value: `{"color":"blue"}`, ValueID: "test-value:theme"},
	}, states)
}

func TestJSONTextIsGivenAsARawMessage(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, map[string]any{"theme": json.RawMessage(`{"color": "red"}`)}).Client()

	decoded := theme{}
	require.NoError(t, client.JSONValue("theme", &decoded, noUser))

	require.Equal(t, theme{Color: "red"}, decoded)
	require.Equal(t, `{"color":"red"}`, client.StringValue("theme", "fallback", noUser))
}

func TestSetValueFiresAWatchBeforeItReturns(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{"new-checkout": true})
	seen := []bool{}
	testClient.Client().WatchBool("new-checkout", false, proUser, func(enabled bool) { seen = append(seen, enabled) })

	require.NoError(t, testClient.SetValue("new-checkout", false))

	require.Equal(t, []bool{false}, seen)
	require.False(t, testClient.Client().BoolValue("new-checkout", true, proUser))
}

func TestSetValueFiresOnConfigsUpdatedWithOnlyThatKeyBeforeItReturns(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{"new-checkout": true, "greeting": "Hello"})
	updates := []configdirector.ConfigsUpdated{}
	testClient.Client().OnConfigsUpdated(func(update configdirector.ConfigsUpdated) { updates = append(updates, update) })

	require.NoError(t, testClient.SetValue("greeting", "Hi"))

	require.Equal(t, []configdirector.ConfigsUpdated{{Keys: []string{"greeting"}, RemovedKeys: []string{}}}, updates)
	require.True(t, testClient.Client().BoolValue("new-checkout", false, noUser))
}

func TestSetValueAddsAKeyTheClientDidNotHold(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, nil)

	require.NoError(t, testClient.SetValue("max-items", 7))

	require.Equal(t, int64(7), testClient.Client().IntValue("max-items", 1, noUser))
}

func TestRemoveValueReportsTheKeyRemovedAndTheWatchGetsTheDefault(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{"new-checkout": true, "greeting": "Hello"})
	updates := []configdirector.ConfigsUpdated{}
	seen := []bool{}
	testClient.Client().OnConfigsUpdated(func(update configdirector.ConfigsUpdated) { updates = append(updates, update) })
	testClient.Client().WatchBool("new-checkout", false, noUser, func(enabled bool) { seen = append(seen, enabled) })

	testClient.RemoveValue("new-checkout")

	require.Equal(t, []configdirector.ConfigsUpdated{{Keys: []string{"greeting"}, RemovedKeys: []string{"new-checkout"}}}, updates)
	require.Equal(t, []bool{false}, seen)
	_, details := testClient.Client().BoolValueDetails("new-checkout", false, noUser)
	require.Equal(t, usedDefault(configdirector.EvaluationReasonConfigStateMissing), details)
}

func TestReplaceValuesServesOnlyTheNewValues(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{"new-checkout": true, "greeting": "Hello"})
	updates := []configdirector.ConfigsUpdated{}
	testClient.Client().OnConfigsUpdated(func(update configdirector.ConfigsUpdated) { updates = append(updates, update) })

	require.NoError(t, testClient.ReplaceValues(map[string]any{"greeting": "Hi", "max-items": 3}))

	require.Equal(t, []configdirector.ConfigsUpdated{{Keys: []string{"greeting", "max-items"}, RemovedKeys: []string{"new-checkout"}}}, updates)
	require.Equal(t, "Hi", testClient.Client().StringValue("greeting", "fallback", noUser))
	require.Equal(t, int64(3), testClient.Client().IntValue("max-items", 1, noUser))
	require.False(t, testClient.Client().BoolValue("new-checkout", false, noUser))
}

func TestHooksSeeTheFirstUpdateAndTheReadyEvent(t *testing.T) {
	t.Parallel()
	events := []string{}
	hooks := configdirector.Hooks{
		OnReady: func() { events = append(events, "ready") },
		OnConfigsUpdated: func(update configdirector.ConfigsUpdated) {
			events = append(events, "updated:"+update.Keys[0]+","+update.Keys[1])
		},
	}

	newTestClient(t, map[string]any{"greeting": "Hello", "new-checkout": true}, configdirectortest.WithHooks(hooks))

	require.Equal(t, []string{"updated:greeting,new-checkout", "ready"}, events)
}

func TestHoldInitializationKeepsWaitForReadyBlockedUntilCompleteInitialization(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{"new-checkout": true}, configdirectortest.HoldInitialization())
	result := waitForReadyIn(testClient.Client())
	requireNoResultWithin(t, result, 50*time.Millisecond)
	require.False(t, testClient.Client().Ready())

	testClient.CompleteInitialization()

	require.NoError(t, requireResultWithin(t, result, time.Second))
	require.True(t, testClient.Client().BoolValue("new-checkout", false, noUser))
}

func TestAHeldClientReadsTheDefaultAsNotReady(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, map[string]any{"new-checkout": true}, configdirectortest.HoldInitialization()).Client()

	value, details := client.BoolValueDetails("new-checkout", false, noUser)

	require.False(t, value)
	require.Equal(t, usedDefault(configdirector.EvaluationReasonClientNotReady), details)
}

func TestCompleteInitializationFiresOnReadyOnTheCallingGoroutine(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{"new-checkout": true}, configdirectortest.HoldInitialization())
	readyCalls := 0
	testClient.Client().OnReady(func() { readyCalls++ })
	require.Equal(t, 0, readyCalls)

	testClient.CompleteInitialization()

	require.Equal(t, 1, readyCalls)
}

func TestValuesSetWhileHeldAreDeliveredByCompleteInitialization(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{"new-checkout": true}, configdirectortest.HoldInitialization())
	updates := []configdirector.ConfigsUpdated{}
	testClient.Client().OnConfigsUpdated(func(update configdirector.ConfigsUpdated) { updates = append(updates, update) })
	require.NoError(t, testClient.SetValue("greeting", "Hello"))
	require.Empty(t, updates)

	testClient.CompleteInitialization()

	require.Equal(t, []configdirector.ConfigsUpdated{{Keys: []string{"greeting", "new-checkout"}, RemovedKeys: []string{}}}, updates)
	require.Equal(t, "Hello", testClient.Client().StringValue("greeting", "fallback", noUser))
}

func TestCompleteInitializationAfterTheWaitTimedOutStillMakesTheClientReady(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{"new-checkout": true}, configdirectortest.HoldInitialization(), configdirectortest.WithTimeout(10*time.Millisecond))
	require.ErrorIs(t, testClient.Client().WaitForReady(context.Background()), context.DeadlineExceeded)

	testClient.CompleteInitialization()

	require.True(t, testClient.Client().Ready())
	require.NoError(t, testClient.Client().WaitForReady(context.Background()))
}

func TestFailInitializationLeavesReadsOnTheDefaultWithClientNotReady(t *testing.T) {
	t.Parallel()
	logger, records := newLogger()
	testClient := newTestClient(t, map[string]any{"new-checkout": true},
		configdirectortest.HoldInitialization(),
		configdirectortest.WithTimeout(20*time.Millisecond),
		configdirectortest.WithLogger(logger),
	)
	result := waitForReadyIn(testClient.Client())

	testClient.FailInitialization()

	require.ErrorIs(t, requireResultWithin(t, result, time.Second), context.DeadlineExceeded)
	value, details := testClient.Client().BoolValueDetails("new-checkout", false, noUser)
	require.False(t, value)
	require.Equal(t, usedDefault(configdirector.EvaluationReasonClientNotReady), details)
	require.Len(t, recordsAt(slog.LevelError, records()), 1)
}

func TestCompleteInitializationAfterAFailureDeliversTheStoredValues(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{"new-checkout": true}, configdirectortest.HoldInitialization())
	testClient.FailInitialization()
	require.NoError(t, testClient.SetValue("greeting", "Hello"))

	testClient.CompleteInitialization()

	require.True(t, testClient.Client().Ready())
	require.Equal(t, "Hello", testClient.Client().StringValue("greeting", "fallback", noUser))
}

func TestInitializationControlsOnAReadyClientChangeNothing(t *testing.T) {
	t.Parallel()
	logger, records := newLogger()
	testClient := newTestClient(t, map[string]any{"new-checkout": true}, configdirectortest.WithLogger(logger))
	updates := []configdirector.ConfigsUpdated{}
	testClient.Client().OnConfigsUpdated(func(update configdirector.ConfigsUpdated) { updates = append(updates, update) })

	testClient.FailInitialization()
	testClient.CompleteInitialization()

	require.Empty(t, updates)
	require.Empty(t, recordsAt(slog.LevelError, records()))
	require.True(t, testClient.Client().BoolValue("new-checkout", false, noUser))
}

func TestAnUnsupportedValueIsRejectedAndNothingChanges(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{"new-checkout": true})
	updates := []configdirector.ConfigsUpdated{}
	testClient.Client().OnConfigsUpdated(func(update configdirector.ConfigsUpdated) { updates = append(updates, update) })

	err := testClient.SetValue("new-checkout", make(chan int))

	require.ErrorContains(t, err, `"new-checkout"`)
	require.ErrorContains(t, err, "chan int")
	require.Empty(t, updates)
	require.True(t, testClient.Client().BoolValue("new-checkout", false, noUser))
}

func TestANilValueIsRejected(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, nil)

	err := testClient.SetValue("new-checkout", nil)

	require.ErrorContains(t, err, `"new-checkout"`)
	require.ErrorContains(t, err, "nil")
}

func TestABlankKeyIsRejected(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, nil)

	err := testClient.SetValue("  ", true)

	require.ErrorContains(t, err, "blank")
}

func TestANonFiniteNumberIsRejected(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, nil)

	err := testClient.SetValue("ratio", math.Inf(1))

	require.ErrorContains(t, err, `"ratio"`)
	require.ErrorContains(t, err, "finite")
}

func TestAValueThatDoesNotEncodeAsJSONIsRejected(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, nil)

	err := testClient.SetValue("limits", map[string]any{"callback": func() {}})

	require.ErrorContains(t, err, `"limits"`)
	require.ErrorContains(t, err, "JSON")
}

func TestReplaceValuesWithAnInvalidValueChangesNothing(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{"new-checkout": true})

	err := testClient.ReplaceValues(map[string]any{"greeting": "Hello", "ratio": math.NaN()})

	require.Error(t, err)
	require.True(t, testClient.Client().BoolValue("new-checkout", false, noUser))
	require.Equal(t, "fallback", testClient.Client().StringValue("greeting", "fallback", noUser))
}

func TestNewTestClientPanicsOnAnInvalidValue(t *testing.T) {
	t.Parallel()

	require.PanicsWithError(t, `configdirector: the value for "new-checkout" is nil, which is not a value; a config holds no null`, func() {
		configdirectortest.NewTestClient(map[string]any{"new-checkout": nil})
	})
}

func TestAnUpdateMadeByAHandlerIsDeliveredAfterTheOneBeingHandled(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{"new-checkout": true, "greeting": "Hello"})
	greetingsSeenByTheSecondWatch := []string{}
	greetings := []string{}
	testClient.Client().WatchBool("new-checkout", true, noUser, func(bool) {
		require.NoError(t, testClient.SetValue("greeting", "Goodbye"))
	})
	testClient.Client().WatchBool("new-checkout", true, noUser, func(bool) {
		greetingsSeenByTheSecondWatch = append(greetingsSeenByTheSecondWatch, testClient.Client().StringValue("greeting", "fallback", noUser))
	})
	testClient.Client().WatchString("greeting", "fallback", noUser, func(greeting string) { greetings = append(greetings, greeting) })

	require.NoError(t, testClient.SetValue("new-checkout", false))

	require.Equal(t, []string{"Hello"}, greetingsSeenByTheSecondWatch)
	require.Equal(t, []string{"Goodbye"}, greetings)
	require.Equal(t, "Goodbye", testClient.Client().StringValue("greeting", "fallback", noUser))
}

func TestCloseClosesTheClientAndSilencesTheControls(t *testing.T) {
	t.Parallel()
	testClient := configdirectortest.NewTestClient(map[string]any{"new-checkout": true})
	updates := []configdirector.ConfigsUpdated{}
	testClient.Client().OnConfigsUpdated(func(update configdirector.ConfigsUpdated) { updates = append(updates, update) })

	require.NoError(t, testClient.Close())
	require.NoError(t, testClient.SetValue("new-checkout", false))
	testClient.RemoveValue("new-checkout")
	testClient.CompleteInitialization()

	require.True(t, testClient.Client().Closed())
	require.Empty(t, updates)
	_, details := testClient.Client().BoolValueDetails("new-checkout", false, noUser)
	require.Equal(t, usedDefault(configdirector.EvaluationReasonClientNotReady), details)
	require.NoError(t, testClient.Close())
}

func TestFailInitializationAfterCloseLogsNothing(t *testing.T) {
	t.Parallel()
	logger, records := newLogger()
	testClient := newTestClient(t, map[string]any{"new-checkout": true}, configdirectortest.HoldInitialization(), configdirectortest.WithLogger(logger))
	require.NoError(t, testClient.Close())

	testClient.FailInitialization()

	require.Empty(t, recordsAt(slog.LevelError, records()))
}

func TestNoGoroutineIsStartedAndNoneRemainsAfterClose(t *testing.T) {
	before := runtime.NumGoroutine()

	testClient := configdirectortest.NewTestClient(map[string]any{"new-checkout": true}, configdirectortest.HoldInitialization())
	testClient.CompleteInitialization()
	require.NoError(t, testClient.SetValue("greeting", "Hello"))
	testClient.RemoveValue("greeting")
	require.Equal(t, before, runtime.NumGoroutine())
	require.NoError(t, testClient.Close())

	require.Equal(t, before, runtime.NumGoroutine())
}

type refusingTransport struct {
	t *testing.T
}

func (transport refusingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.t.Errorf("the test client sent an HTTP request to %s", request.URL)
	return nil, errors.New("the test client must not use the network")
}

func TestNothingReachesTheNetwork(t *testing.T) {
	defaultTransport := http.DefaultTransport
	http.DefaultTransport = refusingTransport{t: t}
	t.Cleanup(func() { http.DefaultTransport = defaultTransport })

	testClient := newTestClient(t, map[string]any{"new-checkout": true}, configdirectortest.HoldInitialization())
	testClient.CompleteInitialization()
	require.NoError(t, testClient.SetValue("greeting", "Hello"))
	testClient.RemoveValue("greeting")
	require.NoError(t, testClient.ReplaceValues(map[string]any{"new-checkout": false}))

	require.False(t, testClient.Client().BoolValue("new-checkout", true, noUser))
}

func TestATestClientStartsWithNoTrackedEvents(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{})

	require.Empty(t, testClient.TrackedEvents())
}

func TestTrackedEventsAreRecordedInCallOrderWithWhatTheyWereTrackedWith(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{})

	testClient.Client().Track("checkout-completed", proUser, configdirector.TrackOptions{Value: 49.9, Details: configdirector.TrackedEventDetails{"plan": "pro"}})
	testClient.Client().Track("nightly-sync-finished", noUser, configdirector.TrackOptions{})

	require.Equal(t, []configdirectortest.TrackedEvent{
		{Name: "checkout-completed", UserContext: proUser, Value: 49.9, Details: map[string]any{"plan": "pro"}},
		{Name: "nightly-sync-finished"},
	}, testClient.TrackedEvents())
}

func TestARecordedEventKeepsTheContextAsItWasSupplied(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{})
	anonymousDevice := configdirector.UserContext{ID: "device-42", Kind: "device", Name: "Kiosk", Anonymous: true}

	testClient.Client().Track("kiosk-opened", anonymousDevice, configdirector.TrackOptions{})

	require.Equal(t, anonymousDevice, testClient.TrackedEvents()[0].UserContext)
}

func TestAWholeNumberValueIsRecordedAsItWasGiven(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{})

	testClient.Client().Track("counted", noUser, configdirector.TrackOptions{Value: 3})

	require.Equal(t, 3, testClient.TrackedEvents()[0].Value)
}

func TestAJSONNumberValueIsRecordedAsItWasGiven(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{})

	testClient.Client().Track("priced", noUser, configdirector.TrackOptions{Value: json.Number("49.90")})

	require.Equal(t, json.Number("49.90"), testClient.TrackedEvents()[0].Value)
}

func TestRecordedDetailsKeepTheGoTypeOfEachValue(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{})

	testClient.Client().Track("checkout-completed", noUser, configdirector.TrackOptions{Details: configdirector.TrackedEventDetails{
		"seats": 3,
		"cart":  map[string]any{"items": []any{"book", 2}},
		"tags":  []string{"gift"},
	}})

	require.Equal(t, map[string]any{
		"seats": 3,
		"cart":  map[string]any{"items": []any{"book", 2}},
		"tags":  []string{"gift"},
	}, testClient.TrackedEvents()[0].Details)
}

func TestRecordedDetailsAreACopyOfWhatWasTracked(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{})
	items := []any{"book"}
	details := configdirector.TrackedEventDetails{"plan": "pro", "items": items}

	testClient.Client().Track("checkout-completed", noUser, configdirector.TrackOptions{Details: details})
	details["plan"] = "free"
	items[0] = "pen"

	require.Equal(t, map[string]any{"plan": "pro", "items": []any{"book"}}, testClient.TrackedEvents()[0].Details)
}

func TestNaNInRecordedDetailsIsRecordedAsTheNullTheSDKWouldSend(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{})

	testClient.Client().Track("checkout-completed", noUser, configdirector.TrackOptions{Details: configdirector.TrackedEventDetails{"score": math.NaN()}})

	require.Equal(t, map[string]any{"score": nil}, testClient.TrackedEvents()[0].Details)
}

func TestAnEventWithAnEmptyNameIsNotRecordedAndEachOneLogsAWarning(t *testing.T) {
	t.Parallel()
	logger, records := newLogger()
	testClient := newTestClient(t, map[string]any{}, configdirectortest.WithLogger(logger))

	testClient.Client().Track("", noUser, configdirector.TrackOptions{})
	testClient.Client().Track("", noUser, configdirector.TrackOptions{})

	require.Empty(t, testClient.TrackedEvents())
	require.Len(t, recordsAt(slog.LevelWarn, records()), 2)
}

func TestAnEventWhoseNameHas151CharactersIsNotRecorded(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{})

	testClient.Client().Track(strings.Repeat("n", 151), noUser, configdirector.TrackOptions{})

	require.Empty(t, testClient.TrackedEvents())
}

func TestAValueTheSDKWouldLeaveOutIsMissingFromTheRecordAndEachOneLogsAWarning(t *testing.T) {
	t.Parallel()
	logger, records := newLogger()
	testClient := newTestClient(t, map[string]any{}, configdirectortest.WithLogger(logger))

	testClient.Client().Track("first", noUser, configdirector.TrackOptions{Value: math.NaN()})
	testClient.Client().Track("second", noUser, configdirector.TrackOptions{Value: math.NaN()})

	require.Equal(t, []configdirectortest.TrackedEvent{{Name: "first"}, {Name: "second"}}, testClient.TrackedEvents())
	require.Len(t, recordsAt(slog.LevelWarn, records()), 2)
}

func TestDetailsTheSDKWouldLeaveOutAreMissingFromTheRecordAndEachOneLogsAWarning(t *testing.T) {
	t.Parallel()
	logger, records := newLogger()
	testClient := newTestClient(t, map[string]any{}, configdirectortest.WithLogger(logger))
	oversized := configdirector.TrackedEventDetails{"k": strings.Repeat("x", 1017)}

	testClient.Client().Track("first", noUser, configdirector.TrackOptions{Details: oversized})
	testClient.Client().Track("second", noUser, configdirector.TrackOptions{Details: oversized})

	require.Equal(t, []configdirectortest.TrackedEvent{{Name: "first"}, {Name: "second"}}, testClient.TrackedEvents())
	require.Len(t, recordsAt(slog.LevelWarn, records()), 2)
}

func TestAnEventForAnInvalidContextKindIsRecordedWithoutAContextAndLogsAnError(t *testing.T) {
	t.Parallel()
	logger, records := newLogger()
	testClient := newTestClient(t, map[string]any{}, configdirectortest.WithLogger(logger))

	testClient.Client().Track("checkout-completed", configdirector.UserContext{ID: "u-pro", Kind: "Device"}, configdirector.TrackOptions{})

	require.Equal(t, []configdirectortest.TrackedEvent{{Name: "checkout-completed"}}, testClient.TrackedEvents())
	require.Len(t, recordsAt(slog.LevelError, records()), 1)
}

func TestTheLimitOfAThousandEventsPerBatchDoesNotApplyToTheRecord(t *testing.T) {
	t.Parallel()
	logger, records := newLogger()
	testClient := newTestClient(t, map[string]any{}, configdirectortest.WithLogger(logger))

	for range 1001 {
		testClient.Client().Track("checkout-completed", noUser, configdirector.TrackOptions{})
	}

	require.Len(t, testClient.TrackedEvents(), 1001)
	require.Empty(t, recordsAt(slog.LevelWarn, records()))
}

func TestChangingTheReturnedEventsDoesNotChangeTheRecord(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{})
	testClient.Client().Track("checkout-completed", noUser, configdirector.TrackOptions{})

	testClient.TrackedEvents()[0].Name = "changed"

	require.Equal(t, "checkout-completed", testClient.TrackedEvents()[0].Name)
}

func TestClearTrackedEventsForgetsEveryEventRecordedSoFar(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{})
	testClient.Client().Track("before", noUser, configdirector.TrackOptions{})

	testClient.ClearTrackedEvents()
	require.Empty(t, testClient.TrackedEvents())

	testClient.Client().Track("after", noUser, configdirector.TrackOptions{})
	require.Equal(t, []configdirectortest.TrackedEvent{{Name: "after"}}, testClient.TrackedEvents())
}

func TestReplaceValuesLeavesTheRecordAlone(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{})
	testClient.Client().Track("checkout-completed", noUser, configdirector.TrackOptions{})

	require.NoError(t, testClient.ReplaceValues(map[string]any{"greeting": "Hello"}))

	require.Equal(t, []configdirectortest.TrackedEvent{{Name: "checkout-completed"}}, testClient.TrackedEvents())
}

func TestAnEventTrackedWhileInitializationIsHeldIsRecorded(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{}, configdirectortest.HoldInitialization())

	testClient.Client().Track("checkout-completed", noUser, configdirector.TrackOptions{})

	require.Equal(t, []configdirectortest.TrackedEvent{{Name: "checkout-completed"}}, testClient.TrackedEvents())
}

func TestAnEventTrackedAfterCloseIsNotRecorded(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{})
	require.NoError(t, testClient.Close())

	testClient.Client().Track("checkout-completed", noUser, configdirector.TrackOptions{})

	require.Empty(t, testClient.TrackedEvents())
}

func TestEventsTrackedFromManyGoroutinesAreAllRecorded(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, map[string]any{})

	var goroutines sync.WaitGroup
	for range 10 {
		goroutines.Go(func() {
			for range 50 {
				testClient.Client().Track("checkout-completed", noUser, configdirector.TrackOptions{})
				testClient.TrackedEvents()
			}
		})
	}
	goroutines.Wait()

	require.Len(t, testClient.TrackedEvents(), 500)
}
