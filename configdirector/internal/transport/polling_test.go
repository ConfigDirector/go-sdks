package transport

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	firstPayload  = `{"timestamp":"2026-10-09T10:00:00Z","configs":{"greeting":{"key":"greeting"}}}`
	secondPayload = `{"timestamp":"2026-10-09T10:05:00Z","configs":{"greeting":{"key":"greeting"}}}`
)

type openedPolling struct {
	*Polling
	openedTransport
}

func (opened *openedPolling) letPoll(t *testing.T) time.Duration {
	t.Helper()
	return await(t, opened.waits)
}

func openPollingAgainst(t *testing.T, server *sdkServer, adjustments ...transportAdjustment) *openedPolling {
	t.Helper()
	opened := &openedPolling{openedTransport: newOpenedTransport()}
	setup := transportSetupAgainst(t, server, opened.openedTransport, adjustments)

	opened.Polling = openPolling(setup.options, setup.pollingInterval, setup.deliver, setup.timing)
	t.Cleanup(func() { _ = opened.Close() })
	return opened
}

func TestOpenPollingPostsTheSDKKeySessionAndMetaContext(t *testing.T) {
	server := startSDKServer(t, answer(http.StatusOK, firstPayload))

	openPollingAgainst(t, server, withApp("checkout", "7.1.0"))

	request := await(t, server.requests)
	require.Equal(t, http.MethodPost, request.method)
	require.Equal(t, "application/json", request.header.Get("Content-Type"))
	require.Equal(t, "go-server-sdk/1.2.3", request.header.Get("User-Agent"))
	require.Equal(t, testSDKKey, request.body.ServerSDKKey)
	require.Regexp(t, uuidVersion4, request.body.SessionID)
	require.Equal(t, map[string]string{
		"sdkName":    "go-server-sdk",
		"sdkVersion": "1.2.3",
		"appName":    "checkout",
		"appVersion": "7.1.0",
	}, request.body.MetaContext)
	require.ElementsMatch(t, []string{"serverSdkKey", "sessionId", "metaContext"}, request.body.fieldNames)
}

func TestTheFirstPollRunsAtOnceAndDeliversThePayload(t *testing.T) {
	server := startSDKServer(t, answer(http.StatusOK, firstPayload))

	opened := openPollingAgainst(t, server)

	require.Equal(t, firstPayload, await(t, opened.delivered))
	require.Equal(t, int64(1), server.requestCount.Load())
}

func TestPollsRepeatOnThePollingInterval(t *testing.T) {
	server := startSDKServer(t, answer(http.StatusOK, firstPayload), answer(http.StatusOK, secondPayload))

	opened := openPollingAgainst(t, server, withPollingInterval(90*time.Second))
	require.Equal(t, firstPayload, await(t, opened.delivered))

	require.Equal(t, 90*time.Second, opened.letPoll(t))
	require.Equal(t, secondPayload, await(t, opened.delivered))
	require.Equal(t, 90*time.Second, opened.letPoll(t))
	await(t, server.requests)
	await(t, server.requests)
	await(t, server.requests)
}

func TestEveryPollCarriesTheSameSessionID(t *testing.T) {
	server := startSDKServer(t, answer(http.StatusOK, firstPayload))

	opened := openPollingAgainst(t, server)
	opened.letPoll(t)

	first := await(t, server.requests)
	second := await(t, server.requests)
	require.Equal(t, first.body.SessionID, second.body.SessionID)
}

func TestA204DeliversNothingAndEchoesThePreviousTimestamp(t *testing.T) {
	server := startSDKServer(t, answer(http.StatusOK, firstPayload), answer(http.StatusNoContent, ""))

	opened := openPollingAgainst(t, server)
	require.Equal(t, firstPayload, await(t, opened.delivered))
	opened.letPoll(t)
	opened.letPoll(t)

	first := await(t, server.requests)
	second := await(t, server.requests)
	third := await(t, server.requests)
	require.Empty(t, first.body.LastUpdateTimestamp)
	require.Equal(t, "2026-10-09T10:00:00Z", second.body.LastUpdateTimestamp)
	require.Equal(t, "2026-10-09T10:00:00Z", third.body.LastUpdateTimestamp)
	expectNothing(t, opened.delivered, 100*time.Millisecond)
	require.Equal(t, 0, opened.logs.recordsAt(slog.LevelError))
}

func TestEachPayloadMovesTheEchoedTimestampForward(t *testing.T) {
	server := startSDKServer(t, answer(http.StatusOK, firstPayload), answer(http.StatusOK, secondPayload))

	opened := openPollingAgainst(t, server)
	opened.letPoll(t)
	opened.letPoll(t)

	await(t, server.requests)
	await(t, server.requests)
	third := await(t, server.requests)
	require.Equal(t, "2026-10-09T10:05:00Z", third.body.LastUpdateTimestamp)
}

func TestAPayloadWithoutATimestampKeepsThePreviousOne(t *testing.T) {
	server := startSDKServer(t, answer(http.StatusOK, firstPayload), answer(http.StatusOK, `{"configs":{}}`))

	opened := openPollingAgainst(t, server)
	opened.letPoll(t)
	opened.letPoll(t)

	await(t, server.requests)
	await(t, server.requests)
	third := await(t, server.requests)
	require.Equal(t, "2026-10-09T10:00:00Z", third.body.LastUpdateTimestamp)
	require.Equal(t, firstPayload, await(t, opened.delivered))
	require.Equal(t, `{"configs":{}}`, await(t, opened.delivered))
}

func TestAConnectionClosedWithoutAnAnswerIsRetriedAtOnce(t *testing.T) {
	server := startSDKServer(t, dropConnection(), answer(http.StatusOK, firstPayload))

	opened := openPollingAgainst(t, server)

	require.Equal(t, firstPayload, await(t, opened.delivered))
	require.Equal(t, int64(2), server.requestCount.Load())
	require.Equal(t, 0, opened.logs.recordsAt(slog.LevelError))
	require.Equal(t, 0, opened.logs.recordsAt(slog.LevelWarn))
	require.Equal(t, 0, opened.logs.recordsAt(slog.LevelInfo))
}

func TestAConnectionDroppedMidBodyIsRetriedAtOnce(t *testing.T) {
	server := startSDKServer(t, dropConnectionMidBody(`{"timestamp":`), answer(http.StatusOK, firstPayload))

	opened := openPollingAgainst(t, server)

	require.Equal(t, firstPayload, await(t, opened.delivered))
	require.Equal(t, int64(2), server.requestCount.Load())
	require.Equal(t, 0, opened.logs.recordsAt(slog.LevelError))
	require.Equal(t, 0, opened.logs.recordsAt(slog.LevelWarn))
	require.Equal(t, 0, opened.logs.recordsAt(slog.LevelInfo))
}

func TestARetrySendsTheSameTimestampAsTheRequestItRepeats(t *testing.T) {
	server := startSDKServer(t, answer(http.StatusOK, firstPayload), dropConnection(), answer(http.StatusOK, secondPayload))

	opened := openPollingAgainst(t, server)
	require.Equal(t, firstPayload, await(t, opened.delivered))
	opened.letPoll(t)
	require.Equal(t, secondPayload, await(t, opened.delivered))

	await(t, server.requests)
	dropped := await(t, server.requests)
	retried := await(t, server.requests)
	require.Equal(t, "2026-10-09T10:00:00Z", dropped.body.LastUpdateTimestamp)
	require.Equal(t, "2026-10-09T10:00:00Z", retried.body.LastUpdateTimestamp)
}

func TestAnUnreachableServerIsLoggedOnceAndPolledAgainOnTheInterval(t *testing.T) {
	server := startSDKServer(t, answer(http.StatusOK, firstPayload))
	server.Close()

	opened := openPollingAgainst(t, server)

	require.Equal(t, time.Minute, opened.letPoll(t))
	require.Equal(t, 1, opened.logs.recordsAt(slog.LevelError))
}

func TestAServerThatClosesEveryConnectionSeesTwoRequestsPerIntervalAndOneErrorLog(t *testing.T) {
	server := startSDKServer(t, dropConnection())

	opened := openPollingAgainst(t, server)
	await(t, server.requests)
	await(t, server.requests)
	require.Equal(t, time.Minute, opened.letPoll(t))

	await(t, server.requests)
	await(t, server.requests)
	expectNothing(t, server.requests, 100*time.Millisecond)
	require.Equal(t, time.Minute, opened.letPoll(t))
	require.Equal(t, 2, opened.logs.recordsAt(slog.LevelError))
}

func TestA503ProducesOneRequestAndOneErrorLogPerInterval(t *testing.T) {
	server := startSDKServer(t, refuse(http.StatusServiceUnavailable))

	opened := openPollingAgainst(t, server)
	await(t, server.requests)
	require.Equal(t, time.Minute, opened.letPoll(t))

	await(t, server.requests)
	expectNothing(t, server.requests, 100*time.Millisecond)
	require.Equal(t, time.Minute, opened.letPoll(t))
	require.Equal(t, 2, opened.logs.recordsAt(slog.LevelError))
}

func TestA503OnTheRetryIsNotRetriedAgain(t *testing.T) {
	server := startSDKServer(t, dropConnection(), refuse(http.StatusServiceUnavailable))

	opened := openPollingAgainst(t, server)
	require.Equal(t, time.Minute, opened.letPoll(t))

	require.Equal(t, int64(2), server.requestCount.Load())
	require.Equal(t, 1, opened.logs.recordsAt(slog.LevelError))
}

func TestARequestThatRunsPastItsDeadlineIsNotRetried(t *testing.T) {
	server := startSDKServer(t, neverAnswer())

	opened := openPollingAgainst(t, server, withRequestTimeout(100*time.Millisecond))
	require.Equal(t, time.Minute, opened.letPoll(t))

	require.Equal(t, int64(1), server.requestCount.Load())
	require.Equal(t, 1, opened.logs.recordsAt(slog.LevelError))
}

func TestABodyThatNeverEndsIsNotRetried(t *testing.T) {
	server := startSDKServer(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(`{"timestamp":`))
		_ = http.NewResponseController(writer).Flush()
		<-request.Context().Done()
	})

	opened := openPollingAgainst(t, server, withRequestTimeout(100*time.Millisecond))
	require.Equal(t, time.Minute, opened.letPoll(t))

	require.Equal(t, int64(1), server.requestCount.Load())
	require.Equal(t, 1, opened.logs.recordsAt(slog.LevelError))
}

func TestAnUnparsableBodyIsNotRetried(t *testing.T) {
	server := startSDKServer(t, answer(http.StatusOK, "not json"))

	opened := openPollingAgainst(t, server)
	require.Equal(t, time.Minute, opened.letPoll(t))

	require.Equal(t, int64(1), server.requestCount.Load())
	require.Equal(t, 1, opened.logs.recordsAt(slog.LevelError))
	expectNothing(t, opened.delivered, 100*time.Millisecond)
}

func TestA401StopsPollingWithOneErrorLog(t *testing.T) {
	server := startSDKServer(t, refuse(http.StatusUnauthorized))

	opened := openPollingAgainst(t, server)

	await(t, server.requests)
	expectNothing(t, opened.waits, 200*time.Millisecond)
	require.Equal(t, int64(1), server.requestCount.Load())
	require.Equal(t, 1, opened.logs.recordsAt(slog.LevelError))
	require.NoError(t, opened.Close())
}

func TestAPollRefusalThatEchoesTheSDKKeyIsLoggedWithoutIt(t *testing.T) {
	server := startSDKServer(t, refuseWith(http.StatusUnauthorized, `{"error":"`+testSDKKey+` is not a server SDK key"}`))

	opened := openPollingAgainst(t, server)

	await(t, server.requests)
	require.Eventually(t, func() bool { return opened.logs.recordsAt(slog.LevelError) == 1 }, awaitTimeout, 10*time.Millisecond)
	require.Equal(t, []string{`transport: the SDK server answered with status 401: {"error":"[the SDK key] is not a server SDK key"}`}, opened.logs.errorCauses())
	require.NoError(t, opened.Close())
}

func TestA404OnTheRetryStopsPolling(t *testing.T) {
	server := startSDKServer(t, dropConnection(), refuse(http.StatusNotFound))

	opened := openPollingAgainst(t, server)

	await(t, server.requests)
	await(t, server.requests)
	expectNothing(t, opened.waits, 200*time.Millisecond)
	require.Equal(t, 1, opened.logs.recordsAt(slog.LevelError))
}

func TestA429WaitsTheRetryAfterWhenItIsLongerThanTheInterval(t *testing.T) {
	server := startSDKServer(t, refuseWithRetryAfter(http.StatusTooManyRequests, "300"), answer(http.StatusOK, firstPayload))

	opened := openPollingAgainst(t, server)

	require.Equal(t, 300*time.Second, opened.letPoll(t))
	require.Equal(t, firstPayload, await(t, opened.delivered))
	require.Equal(t, 1, opened.logs.recordsAt(slog.LevelError))
}

func TestA429WithARetryAfterShorterThanTheIntervalWaitsTheInterval(t *testing.T) {
	server := startSDKServer(t, refuseWithRetryAfter(http.StatusTooManyRequests, "2"))

	opened := openPollingAgainst(t, server)

	require.Equal(t, time.Minute, opened.letPoll(t))
}

func TestA429WithoutRetryAfterWaitsTheInterval(t *testing.T) {
	server := startSDKServer(t, refuse(http.StatusTooManyRequests))

	opened := openPollingAgainst(t, server)

	require.Equal(t, time.Minute, opened.letPoll(t))
	require.Equal(t, 1, opened.logs.recordsAt(slog.LevelError))
}

func TestAnUnsupportedSchemeStopsPollingWithOneErrorLogAndNoRequest(t *testing.T) {
	server := startSDKServer(t, answer(http.StatusOK, firstPayload))

	opened := openPollingAgainst(t, server, withBaseURL("ftp://example.invalid"))

	require.Eventually(t, func() bool { return opened.logs.recordsAt(slog.LevelError) == 1 }, awaitTimeout, 10*time.Millisecond)
	expectNothing(t, server.requests, 100*time.Millisecond)
	require.NoError(t, opened.Close())
}

func TestAProxyBaseURLKeepsItsPathForPolling(t *testing.T) {
	proxied := startSDKServer(t, answer(http.StatusOK, firstPayload))
	mux := http.NewServeMux()
	mux.Handle("/proxy/sdk/", http.StripPrefix("/proxy/sdk", proxied.Config.Handler))
	proxy := httptest.NewServer(mux)
	t.Cleanup(proxy.Close)

	openPollingAgainst(t, proxied, withBaseURL(proxy.URL+"/proxy/sdk"))

	await(t, proxied.requests)
}

func TestClosingThePollingTransportLeavesNoGoroutineBehind(t *testing.T) {
	server := startSDKServer(t, answer(http.StatusOK, firstPayload))
	opened := openPollingAgainst(t, server)
	require.Equal(t, firstPayload, await(t, opened.delivered))
	require.Positive(t, transportGoroutines())

	require.NoError(t, opened.Close())

	require.Eventually(t, func() bool { return transportGoroutines() == 0 }, awaitTimeout, 10*time.Millisecond)
}

func TestCloseWhileAPollIsInFlightReturnsAtOnceWithoutAnErrorLog(t *testing.T) {
	server := startSDKServer(t, neverAnswer())
	opened := openPollingAgainst(t, server)
	await(t, server.requests)
	closingAt := time.Now()

	err := opened.Close()

	require.NoError(t, err)
	require.Less(t, time.Since(closingAt), 500*time.Millisecond)
	require.Equal(t, 0, opened.logs.recordsAt(slog.LevelError))
	require.Eventually(t, func() bool { return transportGoroutines() == 0 }, awaitTimeout, 10*time.Millisecond)
}

func TestCloseWhileWaitingForTheNextPollReturnsAtOnce(t *testing.T) {
	server := startSDKServer(t, answer(http.StatusOK, firstPayload))
	opened := openPollingAgainst(t, server, withRealDelays())
	require.Equal(t, firstPayload, await(t, opened.delivered))
	closingAt := time.Now()

	err := opened.Close()

	require.NoError(t, err)
	require.Less(t, time.Since(closingAt), 500*time.Millisecond)
	require.Eventually(t, func() bool { return transportGoroutines() == 0 }, awaitTimeout, 10*time.Millisecond)
}

func TestClosingThePollingTransportReportsATimeoutWhenADeliveryHandlerDoesNotReturn(t *testing.T) {
	server := startSDKServer(t, answer(http.StatusOK, firstPayload))
	release := make(chan struct{})
	opened := openPollingAgainst(t, server, withCloseTimeout(100*time.Millisecond), withDeliveryBlockedUntil(release))
	await(t, server.requests)
	time.Sleep(50 * time.Millisecond)

	err := opened.Close()

	require.ErrorIs(t, err, errCloseTimedOut)
	close(release)
	require.Eventually(t, func() bool { return transportGoroutines() == 0 }, awaitTimeout, 10*time.Millisecond)
}

func TestClosingThePollingTransportAfterAFatalStopReturnsNil(t *testing.T) {
	server := startSDKServer(t, refuse(http.StatusUnauthorized))
	opened := openPollingAgainst(t, server)
	require.Eventually(t, func() bool { return opened.logs.recordsAt(slog.LevelError) == 1 }, awaitTimeout, 10*time.Millisecond)

	require.NoError(t, opened.Close())
	require.NoError(t, opened.Close())
}

func TestNothingIsDeliveredAfterThePollingTransportIsClosed(t *testing.T) {
	server := startSDKServer(t, answer(http.StatusOK, firstPayload))
	opened := openPollingAgainst(t, server)
	require.Equal(t, firstPayload, await(t, opened.delivered))

	require.NoError(t, opened.Close())

	expectNothing(t, opened.delivered, 100*time.Millisecond)
}

func TestNextPollStopsQuietlyOnceClosed(t *testing.T) {
	step := nextPollAfter(pollEnd{kind: pollEndClosed}, false, time.Minute)

	require.Equal(t, pollStep{kind: pollStepStopClosed}, step)
}

func TestNextPollStopsForGoodOnAnUnusableURL(t *testing.T) {
	step := nextPollAfter(pollEnd{kind: pollEndURLUnusable}, false, time.Minute)

	require.Equal(t, pollStep{kind: pollStepStopFatal}, step)
}

func TestNextPollStopsForGoodOnA403(t *testing.T) {
	step := nextPollAfter(pollEnd{kind: pollEndRefused, status: http.StatusForbidden}, false, time.Minute)

	require.Equal(t, pollStep{kind: pollStepStopFatal}, step)
}

func TestNextPollRetriesAtOnceAfterTheFirstNetworkFailure(t *testing.T) {
	step := nextPollAfter(pollEnd{kind: pollEndNetworkFailure}, false, time.Minute)

	require.Equal(t, pollStep{kind: pollStepRetryAtOnce}, step)
}

func TestNextPollWaitsTheIntervalAfterTheSecondNetworkFailure(t *testing.T) {
	step := nextPollAfter(pollEnd{kind: pollEndNetworkFailure}, true, time.Minute)

	require.Equal(t, pollStep{kind: pollStepWaitAfterFailure, delay: time.Minute}, step)
}

func TestNextPollWaitsTheIntervalAfterA500(t *testing.T) {
	step := nextPollAfter(pollEnd{kind: pollEndRefused, status: http.StatusInternalServerError}, false, time.Minute)

	require.Equal(t, pollStep{kind: pollStepWaitAfterFailure, delay: time.Minute}, step)
}

func TestNextPollWaitsTheLongerOfTheIntervalAndTheRetryAfterOfA429(t *testing.T) {
	end := pollEnd{kind: pollEndRefused, status: http.StatusTooManyRequests, delayNamedByServer: 300 * time.Second, delayNamedByServerKnown: true}

	step := nextPollAfter(end, false, time.Minute)

	require.Equal(t, pollStep{kind: pollStepWaitAfterFailure, delay: 300 * time.Second}, step)
}

func TestNextPollWaitsTheIntervalAfterATimedOutRequest(t *testing.T) {
	step := nextPollAfter(pollEnd{kind: pollEndTimedOut}, false, time.Minute)

	require.Equal(t, pollStep{kind: pollStepWaitAfterFailure, delay: time.Minute}, step)
}

func TestNextPollWaitsTheIntervalAfterAnUnparsableBody(t *testing.T) {
	step := nextPollAfter(pollEnd{kind: pollEndUnparsable}, false, time.Minute)

	require.Equal(t, pollStep{kind: pollStepWaitAfterFailure, delay: time.Minute}, step)
}

func TestNextPollWaitsTheIntervalAfterA204(t *testing.T) {
	step := nextPollAfter(pollEnd{kind: pollEndUnchanged}, false, time.Minute)

	require.Equal(t, pollStep{kind: pollStepWaitForTheNext, delay: time.Minute}, step)
}

func TestNextPollWaitsTheIntervalAfterADeliveredPayload(t *testing.T) {
	step := nextPollAfter(pollEnd{kind: pollEndDelivered}, true, time.Minute)

	require.Equal(t, pollStep{kind: pollStepWaitForTheNext, delay: time.Minute}, step)
}

func TestARequestErrorAfterTheDeadlineIsATimedOutPoll(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-ctx.Done()

	end := pollEndAfterRequestError(ctx, errors.New("read tcp: i/o timeout"), "2026-10-09T10:00:00Z")

	require.Equal(t, pollEndTimedOut, end.kind)
	require.Equal(t, "2026-10-09T10:00:00Z", end.lastUpdateTimestamp)
}

func TestARequestErrorBeforeTheDeadlineIsANetworkFailure(t *testing.T) {
	end := pollEndAfterRequestError(context.Background(), errors.New("connection reset by peer"), "")

	require.Equal(t, pollEndNetworkFailure, end.kind)
}

func TestABodyLongerThanThirtyTwoMiBIsNotRetried(t *testing.T) {
	server := startSDKServer(t, answer(http.StatusOK, `{"timestamp":"`+strings.Repeat("x", maxResponseBodyBytes)+`"}`))

	opened := openPollingAgainst(t, server)
	require.Equal(t, time.Minute, opened.letPoll(t))

	require.Equal(t, int64(1), server.requestCount.Load())
	require.Equal(t, 1, opened.logs.recordsAt(slog.LevelError))
	expectNothing(t, opened.delivered, 100*time.Millisecond)
}
