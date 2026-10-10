package configdirector_test

import (
	"context"
	"log/slog"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/stretchr/testify/require"
)

func TestStreamingModeBecomesReadyAndServesValues(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())

	client := newClientAgainst(t, server)
	waitForReady(t, client)

	require.Equal(t, streamPath, await(t, server.requests).path)
	require.True(t, client.BoolValue("new-checkout", false, noUser))
	require.Equal(t, "Hello, pro", client.StringValue("greeting", "fallback", configdirector.UserContext{ID: "u-pro"}))
}

func TestPollingModeBecomesReadyAndServesValues(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	options := server.options()
	options.Connection.Mode = configdirector.ConnectionModePolling

	client := newClient(t, options)
	waitForReady(t, client)

	require.Equal(t, pollingPath, await(t, server.requests).path)
	require.True(t, client.BoolValue("new-checkout", false, noUser))
	require.Equal(t, int64(26), client.IntValue("max-items", 1, noUser))
}

func TestTheClientIdentifiesItselfAndTheApplicationToTheSDKServer(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	options := server.options()
	options.Metadata = configdirector.Metadata{AppName: "checkout", AppVersion: "7.1.0"}

	newClient(t, options)

	request := await(t, server.requests)
	require.Equal(t, "go-server-sdk/"+configdirector.Version, request.header.Get("User-Agent"))
	require.Equal(t, testSDKKey, request.body["serverSdkKey"])
	require.Equal(t, map[string]any{
		"sdkName":    "go-server-sdk",
		"sdkVersion": configdirector.Version,
		"appName":    "checkout",
		"appVersion": "7.1.0",
	}, request.body["metaContext"])
}

func TestTheConnectionURLMayPointAtAProxyPath(t *testing.T) {
	t.Parallel()
	server := startSDKServerUnder(t, "/configdirector", standardPayload().json())
	options := server.options()
	options.Connection.URL = server.URL + "/configdirector"

	client := newClient(t, options)
	waitForReady(t, client)

	require.Equal(t, "/configdirector"+streamPath, await(t, server.requests).path)
	require.True(t, client.BoolValue("new-checkout", false, noUser))
}

type countingTransport struct {
	next     http.RoundTripper
	requests atomic.Int64
}

func (transport *countingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.requests.Add(1)
	return transport.next.RoundTrip(request)
}

func TestEveryRequestGoesThroughTheConfiguredHTTPClient(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	counting := &countingTransport{next: server.Client().Transport}
	options := server.options()
	options.Connection.Mode = configdirector.ConnectionModePolling
	options.Connection.HTTPClient = &http.Client{Transport: counting}

	client := newClient(t, options)
	waitForReady(t, client)

	require.Equal(t, int64(1), counting.requests.Load())
}

func TestStreamingKeepsConnectingAfterTheReadinessWaitTimesOut(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	server.refuseNextRequest(http.StatusServiceUnavailable)
	options := server.options()
	options.Connection.Timeout = 10 * time.Millisecond
	client := newClient(t, options)
	require.ErrorIs(t, client.WaitForReady(context.Background()), context.DeadlineExceeded)
	ready := make(chan struct{})
	client.OnReady(func() { close(ready) })

	await(t, ready)

	require.True(t, client.Ready())
	require.True(t, client.BoolValue("new-checkout", false, noUser))
}

func TestPollingRetriesAtOnceWhenTheFirstPollGetsNoAnswer(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	server.dropNextConnection()
	options := server.options()
	options.Connection.Mode = configdirector.ConnectionModePolling

	client := newClient(t, options)
	waitForReady(t, client)

	await(t, server.requests)
	await(t, server.requests)
	require.True(t, client.BoolValue("new-checkout", false, noUser))
}

func TestARefusedSDKKeyLeavesTheClientNotReadyWithOneErrorLog(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	server.refuseNextRequest(http.StatusUnauthorized)
	logger, records := newLogger()
	options := server.options()
	options.Connection.Mode = configdirector.ConnectionModePolling
	options.Connection.Timeout = 50 * time.Millisecond
	options.Logger = logger
	client := newClient(t, options)

	err := client.WaitForReady(context.Background())

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.False(t, client.Ready())
	await(t, server.requests)
	require.Len(t, recordsAt(slog.LevelError, records()), 1)
}

func TestAnUnknownTopLevelFieldIsIgnored(t *testing.T) {
	t.Parallel()
	payload := fullPayload(textConfig("greeting", "Hello")).withField("tenantId", "tenant-1")
	client, records := newLoggedClient(t, startSDKServer(t, payload.json()))
	waitForReady(t, client)

	value := client.StringValue("greeting", "fallback", noUser)

	require.Equal(t, "Hello", value)
	require.Empty(t, recordsAt(slog.LevelWarn, records()))
	require.Empty(t, recordsAt(slog.LevelError, records()))
}

func TestAnUnknownConditionFieldIsIgnored(t *testing.T) {
	t.Parallel()
	payload := fullPayload(
		configOf("greeting", "string", "Hello", valueRule("greeting-pro", 0, "Hello, pro", withField(identifierEquals("u-pro"), "weight", 3))),
	)
	client, records := newLoggedClient(t, startSDKServer(t, payload.json()))
	waitForReady(t, client)

	value := client.StringValue("greeting", "fallback", configdirector.UserContext{ID: "u-pro"})

	require.Equal(t, "Hello, pro", value)
	require.Empty(t, recordsAt(slog.LevelWarn, records()))
	require.Empty(t, recordsAt(slog.LevelError, records()))
}

func TestRulesApplyInTheirOrderNotTheirPosition(t *testing.T) {
	t.Parallel()
	payload := fullPayload(configOf("greeting", "string", "Hello",
		valueRule("greeting-second", 1, "Hello, second", identifierEquals("u-pro")),
		valueRule("greeting-first", 0, "Hello, first", identifierEquals("u-pro")),
	))
	client := newClientAgainst(t, startSDKServer(t, payload.json()))
	waitForReady(t, client)

	value := client.StringValue("greeting", "fallback", configdirector.UserContext{ID: "u-pro"})

	require.Equal(t, "Hello, first", value)
}

func TestAConfigThatCannotBeReadIsSkippedWhileTheRestLoad(t *testing.T) {
	t.Parallel()
	payload := fullPayload(textConfig("a", "1"), textConfig("c", "3")).withConfig("b", map[string]any{"id": "config-b", "key": "b", "type": "string"})
	client, records := newLoggedClient(t, startSDKServer(t, payload.json()))
	waitForReady(t, client)

	_, details := client.StringValueDetails("b", "fallback", noUser)

	require.Equal(t, usedDefault(configdirector.EvaluationReasonConfigStateMissing), details)
	require.Equal(t, "1", client.StringValue("a", "fallback", noUser))
	require.Equal(t, "3", client.StringValue("c", "fallback", noUser))
	require.Len(t, recordsAt(slog.LevelWarn, records()), 1)
	require.Equal(t, "b", attributesOf(recordsAt(slog.LevelWarn, records())[0])["key"])
}

func TestAFullPayloadKeepsThePreviousDefinitionOfAConfigItCannotRead(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, fullPayload(textConfig("a", "1"), textConfig("b", "2")))
	updates := newRecorder[configdirector.ConfigsUpdated]()
	client.OnConfigsUpdated(updates.record)

	server.send(fullPayload(textConfig("a", "10")).withConfig("b", "not an object").json())

	require.Equal(t, configdirector.ConfigsUpdated{Keys: []string{"a"}, RemovedKeys: []string{}}, updates.next(t))
	require.Equal(t, "2", client.StringValue("b", "fallback", noUser))
}

func TestAConfigWithAConditionOfAnUnknownKindIsSkipped(t *testing.T) {
	t.Parallel()
	unknownCondition := withField(identifierEquals("u-pro"), "kind", "geofence")
	payload := fullPayload(
		configOf("greeting", "string", "Hello", valueRule("greeting-near", 0, "Hello, neighbour", unknownCondition)),
		textConfig("welcome", "Welcome"),
	)
	client, records := newLoggedClient(t, startSDKServer(t, payload.json()))
	waitForReady(t, client)

	_, details := client.StringValueDetails("greeting", "fallback", configdirector.UserContext{ID: "u-pro"})

	require.Equal(t, usedDefault(configdirector.EvaluationReasonConfigStateMissing), details)
	require.Equal(t, "Welcome", client.StringValue("welcome", "fallback", noUser))
	require.Len(t, recordsAt(slog.LevelWarn, records()), 1)
}

func TestASegmentThatCannotBeReadIsSkippedWhileTheRestLoad(t *testing.T) {
	t.Parallel()
	payload := fullPayload(boolConfigTrueInSegment("beta-banner", betaSegmentID)).withSegment(betaSegmentID, "beta")
	payload.fields["segments"].(map[string]any)["segment-broken"] = map[string]any{"groups": []any{[]any{inSegment(betaSegmentID)}}}
	client, records := newLoggedClient(t, startSDKServer(t, payload.json()))
	waitForReady(t, client)

	value := client.BoolValue("beta-banner", false, betaUser)

	require.True(t, value)
	require.Len(t, recordsAt(slog.LevelWarn, records()), 1)
	require.Equal(t, "segment-broken", attributesOf(recordsAt(slog.LevelWarn, records())[0])["segmentId"])
}

func TestAPayloadVersionOtherThanOneIsLoggedOnceAndReadAnyway(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, fullPayload(textConfig("a", "1")).withField("payloadVersion", 2).json())
	client, records := newLoggedClient(t, server)
	waitForReady(t, client)
	updates := newRecorder[configdirector.ConfigsUpdated]()
	client.OnConfigsUpdated(updates.record)

	server.send(deltaPayload(textConfig("a", "2")).withField("payloadVersion", 2).json())

	updates.next(t)
	require.Equal(t, "2", client.StringValue("a", "fallback", noUser))
	require.Len(t, recordsAt(slog.LevelWarn, records()), 1)
	require.Equal(t, "2", attributesOf(recordsAt(slog.LevelWarn, records())[0])["payloadVersion"])
}

func TestAMessageThatIsNotJSONIsSkippedAndTheStreamContinues(t *testing.T) {
	t.Parallel()
	client, records := newLoggedClient(t, startSDKServer(t, "not json", fullPayload(textConfig("a", "1")).json()))

	waitForReady(t, client)

	require.Equal(t, "1", client.StringValue("a", "fallback", noUser))
	require.Len(t, recordsAt(slog.LevelError, records()), 1)
}

func TestAMessageWithoutAConfigsObjectIsSkippedAndTheStreamContinues(t *testing.T) {
	t.Parallel()
	client, records := newLoggedClient(t, startSDKServer(t, `{"kind":"full"}`, fullPayload(textConfig("a", "1")).json()))

	waitForReady(t, client)

	require.Equal(t, "1", client.StringValue("a", "fallback", noUser))
	require.Len(t, recordsAt(slog.LevelError, records()), 1)
}
