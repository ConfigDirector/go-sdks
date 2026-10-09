package configdirector_test

import (
	"context"
	"testing"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/stretchr/testify/require"
)

const testSDKKey = "test-server-sdk-key"

var noUser = configdirector.UserContext{}

func newClient(t *testing.T, options configdirector.Options) *configdirector.Client {
	t.Helper()
	client, err := configdirector.NewClient(options)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return client
}

func newReadyClient(t *testing.T) *configdirector.Client {
	t.Helper()
	client := newClient(t, configdirector.Options{SDKKey: testSDKKey})
	waitForReady(t, client)
	return client
}

func waitForReady(t *testing.T, client *configdirector.Client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, client.WaitForReady(ctx))
}

func foundMatch(valueID string) configdirector.EvaluationDetails {
	return configdirector.EvaluationDetails{Reason: configdirector.EvaluationReasonFoundMatch, ValueID: valueID, UsedDefault: false}
}

func usedDefault(reason configdirector.EvaluationReason) configdirector.EvaluationDetails {
	return configdirector.EvaluationDetails{Reason: reason, ValueID: "", UsedDefault: true}
}

func TestANewClientIsNeitherReadyNorClosed(t *testing.T) {
	t.Parallel()
	client := newClient(t, configdirector.Options{SDKKey: testSDKKey})

	require.False(t, client.Ready())
	require.False(t, client.Closed())
}

func TestWaitForReadyReturnsOnceTheFirstConfigStateArrives(t *testing.T) {
	t.Parallel()
	client := newClient(t, configdirector.Options{SDKKey: testSDKKey})

	err := client.WaitForReady(context.Background())

	require.NoError(t, err)
	require.True(t, client.Ready())
}

func TestWaitForReadyReturnsTheContextErrorWhenTheContextEndsFirst(t *testing.T) {
	t.Parallel()
	client := newClient(t, configdirector.Options{SDKKey: testSDKKey})
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	err := client.WaitForReady(ctx)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.False(t, client.Ready())
}

func TestWaitForReadyReturnsCanceledWhenTheContextIsCancelled(t *testing.T) {
	t.Parallel()
	client := newClient(t, configdirector.Options{SDKKey: testSDKKey})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.WaitForReady(ctx)

	require.ErrorIs(t, err, context.Canceled)
}

func TestTheClientKeepsConnectingAfterAWaitEnds(t *testing.T) {
	t.Parallel()
	client := newClient(t, configdirector.Options{SDKKey: testSDKKey})
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	require.Error(t, client.WaitForReady(ctx))

	err := client.WaitForReady(context.Background())

	require.NoError(t, err)
	require.True(t, client.Ready())
}

func TestWaitForReadyIsBoundedByTheConnectionTimeout(t *testing.T) {
	t.Parallel()
	client := newClient(t, configdirector.Options{
		SDKKey:     testSDKKey,
		Connection: configdirector.ConnectionOptions{Timeout: time.Millisecond},
	})

	err := client.WaitForReady(context.Background())

	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestWaitForReadyReturnsAtOnceWhenTheClientIsReady(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.WaitForReady(ctx)

	require.NoError(t, err)
}

func TestWaitForReadyOnAClosedClientReportsTheClientClosed(t *testing.T) {
	t.Parallel()
	client := newClient(t, configdirector.Options{SDKKey: testSDKKey})
	require.NoError(t, client.Close())

	err := client.WaitForReady(context.Background())

	require.ErrorIs(t, err, configdirector.ErrClientClosed)
}

func TestBoolValueReturnsTheServedValue(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	value, details := client.BoolValueDetails("new-checkout", false, noUser)

	require.True(t, value)
	require.Equal(t, foundMatch("value-new-checkout-default"), details)
	require.True(t, client.BoolValue("new-checkout", false, noUser))
}

func TestBoolValueIgnoresTheCaseOfTrueAndFalse(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	value, details := client.BoolValueDetails("upper-case-flag", false, noUser)

	require.True(t, value)
	require.Equal(t, foundMatch("value-upper-case-flag-default"), details)
}

func TestBoolValueFallsBackOnAValueThatIsNotTrueOrFalse(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	value, details := client.BoolValueDetails("not-a-boolean", true, noUser)

	require.True(t, value)
	require.Equal(t, usedDefault(configdirector.EvaluationReasonInvalidBoolean), details)
}

func TestStringValueReturnsTheServedValue(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	value, details := client.StringValueDetails("welcome-message", "fallback", noUser)

	require.Equal(t, "Welcome", value)
	require.Equal(t, foundMatch("value-welcome-message-default"), details)
	require.Equal(t, "Welcome", client.StringValue("welcome-message", "fallback", noUser))
}

func TestStringValueReturnsAJSONConfigAsItsRawDocument(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	value, details := client.StringValueDetails("limits", "fallback", noUser)

	require.Equal(t, `{"max":20,"min":1}`, value)
	require.Equal(t, foundMatch("value-limits-default"), details)
}

func TestStringValueFallsBackOnABooleanConfig(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	value, details := client.StringValueDetails("new-checkout", "fallback", noUser)

	require.Equal(t, "fallback", value)
	require.Equal(t, usedDefault(configdirector.EvaluationReasonTypeMismatch), details)
}

func TestStringValueFallsBackOnAnEmptyValue(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	value, details := client.StringValueDetails("empty-message", "fallback", noUser)

	require.Equal(t, "fallback", value)
	require.Equal(t, usedDefault(configdirector.EvaluationReasonValueMissing), details)
}

func TestIntValueReturnsTheServedValue(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	value, details := client.IntValueDetails("max-items", 1, noUser)

	require.Equal(t, int64(26), value)
	require.Equal(t, foundMatch("value-max-items-default"), details)
	require.Equal(t, int64(26), client.IntValue("max-items", 1, noUser))
}

func TestIntValueAcceptsAWholeNumberWrittenWithADecimalPoint(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	value, details := client.IntValueDetails("retry-limit", 1, noUser)

	require.Equal(t, int64(26), value)
	require.Equal(t, foundMatch("value-retry-limit-default"), details)
}

func TestIntValueFallsBackOnAValueThatIsNotANumber(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	value, details := client.IntValueDetails("not-a-number", 7, noUser)

	require.Equal(t, int64(7), value)
	require.Equal(t, usedDefault(configdirector.EvaluationReasonInvalidNumber), details)
}

func TestFloatValueReturnsTheServedValue(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	value, details := client.FloatValueDetails("price-multiplier", 1, noUser)

	require.InDelta(t, 1.5, value, 0)
	require.Equal(t, foundMatch("value-price-multiplier-default"), details)
	require.InDelta(t, 1.5, client.FloatValue("price-multiplier", 1, noUser), 0)
}

func TestFloatValueFallsBackOnAValueThatIsNotANumber(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	value, details := client.FloatValueDetails("not-a-number", 2.5, noUser)

	require.InDelta(t, 2.5, value, 0)
	require.Equal(t, usedDefault(configdirector.EvaluationReasonInvalidNumber), details)
}

type limits struct {
	Max int `json:"max"`
	Min int `json:"min"`
}

func TestJSONValueDecodesIntoAStruct(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)
	target := limits{Max: 5, Min: 5}

	details, err := client.JSONValueDetails("limits", &target, noUser)

	require.NoError(t, err)
	require.Equal(t, limits{Max: 20, Min: 1}, target)
	require.Equal(t, foundMatch("value-limits-default"), details)
}

func TestJSONValueDecodesIntoAMap(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)
	target := map[string]any{}

	err := client.JSONValue("limits", &target, noUser)

	require.NoError(t, err)
	require.Equal(t, map[string]any{"max": float64(20), "min": float64(1)}, target)
}

func TestJSONValueDecodesAnArrayIntoASlice(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)
	target := []string{"default"}

	err := client.JSONValue("allowed-regions", &target, noUser)

	require.NoError(t, err)
	require.Equal(t, []string{"eu", "us"}, target)
}

func TestJSONValueLeavesTheTargetUntouchedOnMalformedJSON(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)
	target := limits{Max: 5, Min: 5}

	details, err := client.JSONValueDetails("broken-json", &target, noUser)

	require.NoError(t, err)
	require.Equal(t, limits{Max: 5, Min: 5}, target)
	require.Equal(t, usedDefault(configdirector.EvaluationReasonInvalidJSON), details)
}

func TestJSONValueLeavesTheTargetUntouchedWhenTheDocumentDoesNotFitIt(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)
	target := []string{"default"}

	details, err := client.JSONValueDetails("limits", &target, noUser)

	require.NoError(t, err)
	require.Equal(t, []string{"default"}, target)
	require.Equal(t, usedDefault(configdirector.EvaluationReasonInvalidJSON), details)
}

func TestJSONValueRejectsATargetThatIsNotAPointer(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	details, err := client.JSONValueDetails("limits", limits{}, noUser)

	require.Error(t, err)
	require.Equal(t, configdirector.EvaluationDetails{}, details)
	require.Error(t, client.JSONValue("limits", limits{}, noUser))
}

func TestJSONValueRejectsANilPointerTarget(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)
	var target *limits

	err := client.JSONValue("limits", target, noUser)

	require.Error(t, err)
}

func TestAReadOfAnUnknownKeyFallsBackWithConfigStateMissing(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	value, details := client.StringValueDetails("unknown-key", "fallback", noUser)

	require.Equal(t, "fallback", value)
	require.Equal(t, usedDefault(configdirector.EvaluationReasonConfigStateMissing), details)
}

func TestAReadBeforeReadyFallsBackWithClientNotReady(t *testing.T) {
	t.Parallel()
	client := newClient(t, configdirector.Options{SDKKey: testSDKKey})

	value, details := client.BoolValueDetails("new-checkout", false, noUser)

	require.False(t, value)
	require.Equal(t, usedDefault(configdirector.EvaluationReasonClientNotReady), details)
}

func TestAReadAfterCloseFallsBackWithClientNotReady(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)
	require.NoError(t, client.Close())

	value, details := client.BoolValueDetails("new-checkout", false, noUser)

	require.False(t, value)
	require.Equal(t, usedDefault(configdirector.EvaluationReasonClientNotReady), details)
}

func TestReadsEvaluateTargetingRulesAgainstTheUserContext(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	proGreeting, proDetails := client.StringValueDetails("greeting", "fallback", configdirector.UserContext{ID: "u-pro"})
	otherGreeting := client.StringValue("greeting", "fallback", configdirector.UserContext{ID: "u-free"})

	require.Equal(t, "Hello, pro", proGreeting)
	require.Equal(t, foundMatch("value-greeting-pro"), proDetails)
	require.Equal(t, "Hello", otherGreeting)
}

func TestReadsEvaluateSegmentsAgainstTheUserTraits(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	tester := client.BoolValue("beta-banner", false, configdirector.UserContext{ID: "u-1", Traits: map[string]any{"plan": "beta"}})
	other := client.BoolValue("beta-banner", false, configdirector.UserContext{ID: "u-2", Traits: map[string]any{"plan": "free"}})

	require.True(t, tester)
	require.False(t, other)
}

func TestReadsEvaluateTargetingRulesAgainstTheApplicationMetadata(t *testing.T) {
	t.Parallel()
	opsConsole := newClient(t, configdirector.Options{SDKKey: testSDKKey, Metadata: configdirector.Metadata{AppName: "ops-console"}})
	storefront := newClient(t, configdirector.Options{SDKKey: testSDKKey, Metadata: configdirector.Metadata{AppName: "storefront"}})
	waitForReady(t, opsConsole)
	waitForReady(t, storefront)

	require.True(t, opsConsole.BoolValue("maintenance-banner", false, noUser))
	require.False(t, storefront.BoolValue("maintenance-banner", false, noUser))
}

func TestAllConfigsReturnsEveryServedStateSortedByKey(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	states := client.AllConfigs(noUser)

	require.Len(t, states, 15)
	require.Equal(t, configdirector.ConfigState{
		ID:      "config-allowed-regions",
		Key:     "allowed-regions",
		Type:    configdirector.ConfigTypeJSON,
		Value:   `["eu","us"]`,
		ValueID: "value-allowed-regions-default",
	}, states[0])
	require.Equal(t, "beta-banner", states[1].Key)
}

func TestAllConfigsFiltersByKey(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	states := client.AllConfigs(noUser, "welcome-message", "max-items", "unknown-key")

	require.Equal(t, []configdirector.ConfigState{
		{ID: "config-welcome-message", Key: "welcome-message", Type: configdirector.ConfigTypeString, Value: "Welcome", ValueID: "value-welcome-message-default"},
		{ID: "config-max-items", Key: "max-items", Type: configdirector.ConfigTypeInteger, Value: "26", ValueID: "value-max-items-default"},
	}, states)
}

func TestAllConfigsEvaluatesAgainstTheUserContext(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	states := client.AllConfigs(configdirector.UserContext{ID: "u-pro"}, "greeting")

	require.Equal(t, "Hello, pro", states[0].Value)
	require.Equal(t, "value-greeting-pro", states[0].ValueID)
}

func TestAllConfigsIsEmptyBeforeReady(t *testing.T) {
	t.Parallel()
	client := newClient(t, configdirector.Options{SDKKey: testSDKKey})

	states := client.AllConfigs(noUser)

	require.Empty(t, states)
	require.NotNil(t, states)
}

func TestCloseIsIdempotent(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	require.NoError(t, client.Close())
	require.NoError(t, client.Close())
	require.True(t, client.Closed())
}

func TestAClosedClientIsNotReady(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	require.NoError(t, client.Close())

	require.False(t, client.Ready())
}

func TestNoConfigStateArrivesAfterClose(t *testing.T) {
	t.Parallel()
	client := newClient(t, configdirector.Options{SDKKey: testSDKKey})
	require.NoError(t, client.Close())

	time.Sleep(300 * time.Millisecond)

	require.False(t, client.Ready())
	require.Empty(t, client.AllConfigs(noUser))
}
