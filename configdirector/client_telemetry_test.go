package configdirector_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/stretchr/testify/require"
)

type telemetryReport struct {
	ServerSDKKey   string            `json:"serverSdkKey"`
	MetaContext    map[string]string `json:"metaContext"`
	DiscreteEvents struct {
		CapturedContexts []capturedContext `json:"capturedContexts"`
	} `json:"discreteEvents"`
	AggregatedEvents struct {
		EvaluatedConfig []aggregatedEvaluation `json:"evaluatedConfig"`
	} `json:"aggregatedEvents"`
	DroppedEvents struct {
		EvaluatedConfig  int `json:"evaluatedConfig"`
		CapturedContexts int `json:"capturedContexts"`
	} `json:"droppedEvents"`
}

type capturedContext struct {
	ID     string         `json:"id"`
	Name   *string        `json:"name"`
	Traits map[string]any `json:"traits"`
}

type aggregatedEvaluation struct {
	StartTime string               `json:"startTime"`
	EndTime   string               `json:"endTime"`
	Count     int                  `json:"count"`
	Event     evaluatedConfigEvent `json:"event"`
}

type evaluatedConfigEvent struct {
	ContextID        *string        `json:"contextId"`
	Key              string         `json:"key"`
	Type             *string        `json:"type"`
	DefaultValue     telemetryValue `json:"defaultValue"`
	RequestedType    string         `json:"requestedType"`
	EvaluatedValue   telemetryValue `json:"evaluatedValue"`
	EvaluatedValueID *string        `json:"evaluatedValueId"`
	UsedDefault      bool           `json:"usedDefault"`
	EvaluationReason string         `json:"evaluationReason"`
}

type telemetryValue struct {
	Value   *string `json:"value"`
	ValueID *string `json:"valueId"`
}

var identifiedUser = configdirector.UserContext{ID: "user-123", Name: "Ada", Traits: map[string]any{"plan": "pro"}}

func inline(value string) telemetryValue {
	return telemetryValue{Value: &value}
}

func byID(valueID string) telemetryValue {
	return telemetryValue{ValueID: &valueID}
}

func pointerTo(value string) *string {
	return &value
}

const firstScheduledFlushTimeout = 8 * time.Second

func (server *sdkServer) nextTelemetryReport(t *testing.T) telemetryReport {
	t.Helper()
	return server.telemetryReportWithin(t, awaitTimeout)
}

func (server *sdkServer) telemetryReportWithin(t *testing.T, timeout time.Duration) telemetryReport {
	t.Helper()
	recorded := awaitFor(t, server.telemetryRequests, timeout)
	var report telemetryReport
	require.NoError(t, json.Unmarshal(recorded.bodyJSON, &report))
	return report
}

func reportAfterClosing(t *testing.T, client *configdirector.Client, server *sdkServer) telemetryReport {
	t.Helper()
	require.NoError(t, client.Close())
	return server.nextTelemetryReport(t)
}

func expectNoTelemetry(t *testing.T, server *sdkServer, within time.Duration) {
	t.Helper()
	select {
	case recorded := <-server.telemetryRequests:
		t.Fatalf("a telemetry report arrived although none was expected: %s", recorded.bodyJSON)
	case <-time.After(within):
	}
}

func eventsOf(report telemetryReport) []aggregatedEvaluation {
	return report.AggregatedEvents.EvaluatedConfig
}

func onlyEventOf(t *testing.T, report telemetryReport) aggregatedEvaluation {
	t.Helper()
	require.Len(t, eventsOf(report), 1)
	return eventsOf(report)[0]
}

func requireRFC3339Millis(t *testing.T, timestamp string) {
	t.Helper()
	_, err := time.Parse("2006-01-02T15:04:05.000Z", timestamp)
	require.NoError(t, err, timestamp)
}

func TestTwoIdenticalReadsArriveAsOneEventWithACountOfTwo(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	client.BoolValue("new-checkout", false, identifiedUser)
	client.BoolValue("new-checkout", false, identifiedUser)

	aggregated := onlyEventOf(t, reportAfterClosing(t, client, server))
	require.Equal(t, 2, aggregated.Count)
	requireRFC3339Millis(t, aggregated.StartTime)
	requireRFC3339Millis(t, aggregated.EndTime)
	require.Equal(t, evaluatedConfigEvent{
		ContextID:        pointerTo("user-123"),
		Key:              "new-checkout",
		Type:             pointerTo("boolean"),
		DefaultValue:     inline("false"),
		RequestedType:    "bool",
		EvaluatedValue:   inline("true"),
		EvaluatedValueID: pointerTo("value-new-checkout-default"),
		UsedDefault:      false,
		EvaluationReason: "found-match",
	}, aggregated.Event)
}

func TestReadsThatDifferArriveAsSeparateEventsInTheOrderTheyWereMade(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	client.StringValue("welcome-message", "fallback", noUser)
	client.StringValue("greeting", "fallback", noUser)
	client.StringValue("welcome-message", "fallback", noUser)

	events := eventsOf(reportAfterClosing(t, client, server))
	require.Len(t, events, 2)
	require.Equal(t, "welcome-message", events[0].Event.Key)
	require.Equal(t, 2, events[0].Count)
	require.Equal(t, "greeting", events[1].Event.Key)
	require.Equal(t, 1, events[1].Count)
}

func TestAReadThatUsedTheDefaultCarriesUsedDefaultAndTheReason(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	client.IntValue("not-a-number", 10, noUser)

	event := onlyEventOf(t, reportAfterClosing(t, client, server)).Event
	require.True(t, event.UsedDefault)
	require.Equal(t, "invalid-number", event.EvaluationReason)
	require.Equal(t, pointerTo("integer"), event.Type)
	require.Equal(t, inline("10"), event.DefaultValue)
	require.Equal(t, inline("10"), event.EvaluatedValue)
	require.Nil(t, event.EvaluatedValueID)
	require.Nil(t, event.ContextID)
}

func TestAReadOfAnUnknownKeyCarriesNoConfigType(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	client.StringValue("unknown-key", "fallback", noUser)

	event := onlyEventOf(t, reportAfterClosing(t, client, server)).Event
	require.Nil(t, event.Type)
	require.Equal(t, "config-state-missing", event.EvaluationReason)
	require.Equal(t, inline("fallback"), event.EvaluatedValue)
	require.Equal(t, "string", event.RequestedType)
}

func TestAStringReadReportsStringAsTheRequestedType(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	client.StringValue("welcome-message", "fallback", noUser)

	event := onlyEventOf(t, reportAfterClosing(t, client, server)).Event
	require.Equal(t, "string", event.RequestedType)
	require.Equal(t, inline("Welcome"), event.EvaluatedValue)
	require.Equal(t, inline("fallback"), event.DefaultValue)
}

func TestAnIntReadReportsInt64AsTheRequestedType(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	client.IntValue("max-items", 10, noUser)

	event := onlyEventOf(t, reportAfterClosing(t, client, server)).Event
	require.Equal(t, "int64", event.RequestedType)
	require.Equal(t, inline("26"), event.EvaluatedValue)
	require.Equal(t, inline("10"), event.DefaultValue)
}

func TestAFloatReadReportsFloat64AsTheRequestedType(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	client.FloatValue("price-multiplier", 2.5, noUser)

	event := onlyEventOf(t, reportAfterClosing(t, client, server)).Event
	require.Equal(t, "float64", event.RequestedType)
	require.Equal(t, inline("1.5"), event.EvaluatedValue)
	require.Equal(t, inline("2.5"), event.DefaultValue)
}

func TestAWholeNumberFloatIsReportedWithoutAFraction(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	client.FloatValue("unknown-key", 2.0, noUser)

	event := onlyEventOf(t, reportAfterClosing(t, client, server)).Event
	require.Equal(t, inline("2"), event.DefaultValue)
	require.Equal(t, inline("2"), event.EvaluatedValue)
}

func TestAJSONReadIntoAStructReportsTheStructNameAsTheRequestedType(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	target := limits{Max: 5, Min: 1}
	require.NoError(t, client.JSONValue("limits", &target, noUser))

	event := onlyEventOf(t, reportAfterClosing(t, client, server)).Event
	require.Equal(t, "limits", event.RequestedType)
	require.Equal(t, pointerTo("json"), event.Type)
}

func TestAJSONReadCarriesTheServerValueIDAndTheDigestOfItsDefault(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	target := limits{Max: 5, Min: 1}
	require.NoError(t, client.JSONValue("limits", &target, noUser))

	event := onlyEventOf(t, reportAfterClosing(t, client, server)).Event
	require.Equal(t, byID("value-limits-default"), event.EvaluatedValue)
	require.Equal(t, pointerTo("value-limits-default"), event.EvaluatedValueID)
	require.Equal(t, byID("5zh2NOzZcJmKjjBJ5XJBzx"), event.DefaultValue)
}

func TestAJSONDefaultIsIdentifiedByTheDigestOfItsCompactDocument(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	target := map[string]any{"on": true}
	require.NoError(t, client.JSONValue("unknown-key", &target, noUser))

	event := onlyEventOf(t, reportAfterClosing(t, client, server)).Event
	require.Equal(t, byID("2rnhxPOGf2H7CuV9sivHrq"), event.DefaultValue)
	require.Equal(t, byID("2rnhxPOGf2H7CuV9sivHrq"), event.EvaluatedValue)
	require.Nil(t, event.EvaluatedValueID)
	require.Equal(t, "map[string]interface {}", event.RequestedType)
}

func TestAJSONDefaultWithHTMLCharactersIsDigestedUnescaped(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	target := map[string]any{"html": "<b>&</b>"}
	require.NoError(t, client.JSONValue("unknown-key", &target, noUser))

	event := onlyEventOf(t, reportAfterClosing(t, client, server)).Event
	require.Equal(t, byID("5qShBCZPWfF6ON7XbsJ0Wl"), event.DefaultValue)
}

func TestAnEmptyStringDefaultTravelsInlineAsAnEmptyValue(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	client.StringValue("unknown-key", "", noUser)

	event := onlyEventOf(t, reportAfterClosing(t, client, server)).Event
	require.Equal(t, inline(""), event.DefaultValue)
	require.Equal(t, inline(""), event.EvaluatedValue)
}

func TestAJSONReadIntoASliceReportsTheSliceTypeAsTheRequestedType(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	var target []any
	require.NoError(t, client.JSONValue("allowed-regions", &target, noUser))

	event := onlyEventOf(t, reportAfterClosing(t, client, server)).Event
	require.Equal(t, "[]interface {}", event.RequestedType)
	require.Equal(t, byID("value-allowed-regions-default"), event.EvaluatedValue)
}

func TestATypedReadOfAJSONConfigReportsBothValuesAsDocuments(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	client.StringValue("limits", "fallback", noUser)

	event := onlyEventOf(t, reportAfterClosing(t, client, server)).Event
	require.Equal(t, byID("value-limits-default"), event.EvaluatedValue)
	require.Equal(t, byID("0DNWcqYZCP0V1fCd5DKy69"), event.DefaultValue)
	require.Equal(t, "string", event.RequestedType)
}

func TestAServedValueLongerThan500CharactersTravelsAsItsServerValueID(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 501)
	client, server := newReadyClientAgainst(t, fullPayload(textConfig("long-text", long)))

	client.StringValue("long-text", "fallback", noUser)

	event := onlyEventOf(t, reportAfterClosing(t, client, server)).Event
	require.Equal(t, byID("value-long-text-default"), event.EvaluatedValue)
}

func TestADefaultLongerThan500CharactersTravelsAsItsDigest(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	client.StringValue("unknown-key", strings.Repeat("y", 501), noUser)

	event := onlyEventOf(t, reportAfterClosing(t, client, server)).Event
	require.Equal(t, byID("3U0HsSyOct2qwhuFfMKsWO"), event.DefaultValue)
	require.Equal(t, byID("3U0HsSyOct2qwhuFfMKsWO"), event.EvaluatedValue)
}

func TestAValueOfExactly500CharactersTravelsInline(t *testing.T) {
	t.Parallel()
	exact := strings.Repeat("z", 500)
	client, server := newReadyClientAgainst(t, fullPayload(textConfig("exact-text", exact)))

	client.StringValue("exact-text", "fallback", noUser)

	event := onlyEventOf(t, reportAfterClosing(t, client, server)).Event
	require.Equal(t, inline(exact), event.EvaluatedValue)
}

func TestAnIdentifiedContextIsCapturedOnceAcrossItsReads(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	client.BoolValue("new-checkout", false, identifiedUser)
	client.StringValue("welcome-message", "fallback", identifiedUser)

	report := reportAfterClosing(t, client, server)
	require.Equal(t, []capturedContext{{ID: "user-123", Name: pointerTo("Ada"), Traits: map[string]any{"plan": "pro"}}}, report.DiscreteEvents.CapturedContexts)
	require.Equal(t, pointerTo("user-123"), eventsOf(report)[0].Event.ContextID)
	require.Equal(t, pointerTo("user-123"), eventsOf(report)[1].Event.ContextID)
}

func TestACapturedContextKeepsTheTraitsItWasReadWithWhenTheCallerChangesTheMap(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())
	traits := map[string]any{"plan": "pro"}

	client.BoolValue("new-checkout", false, configdirector.UserContext{ID: "user-789", Traits: traits})
	traits["plan"] = "enterprise"

	report := reportAfterClosing(t, client, server)
	require.Equal(t, []capturedContext{{ID: "user-789", Traits: map[string]any{"plan": "pro"}}}, report.DiscreteEvents.CapturedContexts)
}

func TestAContextWithOnlyAnIDIsCapturedWithoutNameOrTraits(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	client.BoolValue("new-checkout", false, configdirector.UserContext{ID: "user-456"})

	report := reportAfterClosing(t, client, server)
	require.Equal(t, []capturedContext{{ID: "user-456"}}, report.DiscreteEvents.CapturedContexts)
}

func TestAnAnonymousContextIsNeitherCapturedNorIdentified(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	client.BoolValue("new-checkout", false, configdirector.UserContext{ID: "user-123", Anonymous: true})

	report := reportAfterClosing(t, client, server)
	require.Empty(t, report.DiscreteEvents.CapturedContexts)
	require.Nil(t, onlyEventOf(t, report).Event.ContextID)
}

func TestAContextWithoutAnIDIsNeitherCapturedNorIdentified(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	client.BoolValue("new-checkout", false, configdirector.UserContext{Traits: map[string]any{"plan": "pro"}})

	report := reportAfterClosing(t, client, server)
	require.Empty(t, report.DiscreteEvents.CapturedContexts)
	require.Nil(t, onlyEventOf(t, report).Event.ContextID)
}

func TestFillingTheEvaluationQueuePastItsShareDropsTheOldestAndCountsThem(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	options := server.options()
	options.Telemetry.EventQueueLimit = 100
	client := newClient(t, options)
	waitForReady(t, client)

	for index := range 72 {
		client.StringValue("key-"+string(rune('A'+index/26))+string(rune('a'+index%26)), "fallback", noUser)
	}

	report := reportAfterClosing(t, client, server)
	require.Len(t, eventsOf(report), 70)
	require.Equal(t, "key-Ac", eventsOf(report)[0].Event.Key)
	require.Equal(t, "key-Ct", eventsOf(report)[69].Event.Key)
	require.Equal(t, 2, report.DroppedEvents.EvaluatedConfig)
	require.Equal(t, 0, report.DroppedEvents.CapturedContexts)
}

func TestFillingTheContextShareOfTheQueueDropsTheOldestContextAndCountsIt(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	options := server.options()
	options.Telemetry.EventQueueLimit = 100
	client := newClient(t, options)
	waitForReady(t, client)

	for index := range 31 {
		client.BoolValue("new-checkout", false, configdirector.UserContext{ID: "user-" + string(rune('a'+index%26)) + string(rune('a'+index/26))})
	}

	report := reportAfterClosing(t, client, server)
	require.Len(t, report.DiscreteEvents.CapturedContexts, 30)
	require.Equal(t, "user-ba", report.DiscreteEvents.CapturedContexts[0].ID)
	require.Equal(t, "user-eb", report.DiscreteEvents.CapturedContexts[29].ID)
	require.Equal(t, 1, report.DroppedEvents.CapturedContexts)
	require.Equal(t, 0, report.DroppedEvents.EvaluatedConfig)
}

func TestTheFirstFlushHappensFiveSecondsAfterTheClientIsCreated(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())
	started := time.Now()

	client.BoolValue("new-checkout", false, noUser)
	expectNoTelemetry(t, server, 4*time.Second)
	first := server.telemetryReportWithin(t, firstScheduledFlushTimeout)

	require.GreaterOrEqual(t, time.Since(started), 4500*time.Millisecond)
	require.Equal(t, "new-checkout", onlyEventOf(t, first).Event.Key)
}

func TestFlushesAfterTheFirstFollowTheFlushIntervalAndCarryOnlyWhatIsNew(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	options := server.options()
	options.Telemetry.FlushInterval = 300 * time.Millisecond
	client := newClient(t, options)
	waitForReady(t, client)

	client.BoolValue("new-checkout", false, noUser)
	awaitFor(t, server.telemetryRequests, firstScheduledFlushTimeout)
	firstArrived := time.Now()
	client.StringValue("welcome-message", "fallback", noUser)
	second := server.nextTelemetryReport(t)
	secondArrived := time.Now()

	require.Equal(t, "welcome-message", onlyEventOf(t, second).Event.Key)
	require.Equal(t, 1, onlyEventOf(t, second).Count)
	require.GreaterOrEqual(t, secondArrived.Sub(firstArrived), 250*time.Millisecond)
	require.Less(t, secondArrived.Sub(firstArrived), 2*time.Second)
}

func TestCloseFlushesWhatWasCollectedBeforeTheFirstScheduledFlush(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	client.BoolValue("new-checkout", false, noUser)
	expectNoTelemetry(t, server, 100*time.Millisecond)
	require.NoError(t, client.Close())

	require.Equal(t, "new-checkout", onlyEventOf(t, server.nextTelemetryReport(t)).Event.Key)
}

func TestAFatalStatusStopsTelemetryForGoodWithOneErrorLog(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	server.answerTelemetryWith(http.StatusUnauthorized)
	logger, records := newLogger()
	options := server.options()
	options.Telemetry.FlushInterval = 100 * time.Millisecond
	options.Logger = logger
	client := newClient(t, options)
	waitForReady(t, client)

	client.BoolValue("new-checkout", false, noUser)
	awaitFor(t, server.telemetryRequests, firstScheduledFlushTimeout)
	client.BoolValue("new-checkout", false, noUser)
	expectNoTelemetry(t, server, time.Second)
	require.NoError(t, client.Close())

	expectNoTelemetry(t, server, 200*time.Millisecond)
	require.Len(t, recordsAt(slog.LevelError, records()), 1)
}

func TestAServerErrorIsLoggedAsAWarningAndTelemetryContinues(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	server.answerTelemetryWith(http.StatusInternalServerError)
	logger, records := newLogger()
	options := server.options()
	options.Telemetry.FlushInterval = 100 * time.Millisecond
	options.Logger = logger
	client := newClient(t, options)
	waitForReady(t, client)

	client.BoolValue("new-checkout", false, noUser)
	awaitFor(t, server.telemetryRequests, firstScheduledFlushTimeout)
	server.answerTelemetryWith(http.StatusNoContent)
	client.StringValue("welcome-message", "fallback", noUser)
	second := server.nextTelemetryReport(t)

	require.Equal(t, "welcome-message", onlyEventOf(t, second).Event.Key)
	require.Len(t, recordsAt(slog.LevelWarn, records()), 1)
	require.Empty(t, recordsAt(slog.LevelError, records()))
}

func TestAllConfigsRecordsNoTelemetry(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	require.NotEmpty(t, client.AllConfigs(identifiedUser))
	require.NoError(t, client.Close())

	expectNoTelemetry(t, server, 300*time.Millisecond)
}

func TestAWatchThatFiresIsRecorded(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())
	fired := newRecorder[string]()
	client.WatchString("greeting", "fallback", identifiedUser, fired.record)

	server.send(deltaPayload(textConfig("greeting", "Bonjour")).json())
	require.Equal(t, "Bonjour", fired.next(t))

	event := onlyEventOf(t, reportAfterClosing(t, client, server)).Event
	require.Equal(t, "greeting", event.Key)
	require.Equal(t, inline("Bonjour"), event.EvaluatedValue)
	require.Equal(t, pointerTo("user-123"), event.ContextID)
}

func TestTheReportIdentifiesTheSDKKeyTheSDKAndTheApplication(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	options := server.options()
	options.Metadata = configdirector.Metadata{AppName: "checkout", AppVersion: "4.5.6"}
	client := newClient(t, options)
	waitForReady(t, client)

	client.BoolValue("new-checkout", false, noUser)

	report := reportAfterClosing(t, client, server)
	require.Equal(t, testSDKKey, report.ServerSDKKey)
	require.Equal(t, map[string]string{
		"sdkName":    "go-server-sdk",
		"sdkVersion": configdirector.Version,
		"appName":    "checkout",
		"appVersion": "4.5.6",
	}, report.MetaContext)
}

func TestTelemetryIsPostedToTheTelemetryPathUnderTheProxyPath(t *testing.T) {
	t.Parallel()
	server := startSDKServerUnder(t, "/proxy", standardPayload().json())
	options := server.options()
	options.Connection.URL = server.URL + "/proxy"
	client := newClient(t, options)
	waitForReady(t, client)

	client.BoolValue("new-checkout", false, noUser)
	require.NoError(t, client.Close())

	recorded := await(t, server.telemetryRequests)
	require.Equal(t, "/proxy/server/telemetry/v1", recorded.path)
	require.Equal(t, "application/json", recorded.header.Get("Content-Type"))
	require.Equal(t, "go-server-sdk/"+configdirector.Version, recorded.header.Get("User-Agent"))
}

func TestTelemetryGoesThroughTheConfiguredHTTPClient(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	counting := &countingTransport{next: server.Client().Transport}
	options := server.options()
	options.Connection.Mode = configdirector.ConnectionModePolling
	options.Connection.HTTPClient = &http.Client{Transport: counting}
	client := newClient(t, options)
	waitForReady(t, client)

	client.BoolValue("new-checkout", false, noUser)
	require.NoError(t, client.Close())

	await(t, server.telemetryRequests)
	require.Equal(t, int64(2), counting.requests.Load())
}
