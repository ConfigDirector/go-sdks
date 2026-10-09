package transport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const testSDKKey = "test-server-sdk-key"

const awaitTimeout = 5 * time.Second

var uuidVersion4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type recordedRequest struct {
	method string
	header http.Header
	body   recordedBody
}

type recordedBody struct {
	ServerSDKKey string            `json:"serverSdkKey"`
	SessionID    string            `json:"sessionId"`
	MetaContext  map[string]string `json:"metaContext"`
	fieldNames   []string
}

func recordRequest(request *http.Request) recordedRequest {
	raw, _ := io.ReadAll(request.Body)

	var body recordedBody
	var fields map[string]json.RawMessage
	if err := errors.Join(json.Unmarshal(raw, &body), json.Unmarshal(raw, &fields)); err != nil {
		body.fieldNames = []string{"unparsable body: " + err.Error()}
	}
	for field := range fields {
		body.fieldNames = append(body.fieldNames, field)
	}
	return recordedRequest{method: request.Method, header: request.Header.Clone(), body: body}
}

type sdkServer struct {
	*httptest.Server
	streamRequests  chan recordedRequest
	heartbeats      chan recordedRequest
	requestCount    atomic.Int64
	heartbeatStatus atomic.Int64
}

func startSDKServer(t *testing.T, scripts ...http.HandlerFunc) *sdkServer {
	t.Helper()
	server := &sdkServer{
		streamRequests: make(chan recordedRequest, 1024),
		heartbeats:     make(chan recordedRequest, 1024),
	}
	server.heartbeatStatus.Store(http.StatusNoContent)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /server/sse/v1", func(writer http.ResponseWriter, request *http.Request) {
		server.streamRequests <- recordRequest(request)
		nth := int(server.requestCount.Add(1)) - 1
		scripts[min(nth, len(scripts)-1)](writer, request)
	})
	mux.HandleFunc("POST /server/heartbeat/v1", func(writer http.ResponseWriter, request *http.Request) {
		server.heartbeats <- recordRequest(request)
		writer.WriteHeader(int(server.heartbeatStatus.Load()))
	})
	server.Server = httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func refuse(status int) http.HandlerFunc {
	return func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(status)
		_, _ = io.WriteString(writer, "refused")
	}
}

func refuseWithRetryAfter(status int, retryAfter string) http.HandlerFunc {
	return func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Retry-After", retryAfter)
		writer.WriteHeader(status)
	}
}

func beginEventStream(writer http.ResponseWriter, frames ...string) {
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.WriteHeader(http.StatusOK)
	controller := http.NewResponseController(writer)
	_ = controller.Flush()
	for _, frame := range frames {
		_, _ = io.WriteString(writer, frame)
		_ = controller.Flush()
	}
}

func streamHeldOpen(frames ...string) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		beginEventStream(writer, frames...)
		<-request.Context().Done()
	}
}

func streamThenDrop(frames ...string) http.HandlerFunc {
	return func(writer http.ResponseWriter, _ *http.Request) {
		beginEventStream(writer, frames...)
	}
}

type logRecorder struct {
	mutex   sync.Mutex
	records []slog.Record
}

func (recorder *logRecorder) Enabled(context.Context, slog.Level) bool { return true }

func (recorder *logRecorder) Handle(_ context.Context, record slog.Record) error {
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	recorder.records = append(recorder.records, record)
	return nil
}

func (recorder *logRecorder) WithAttrs([]slog.Attr) slog.Handler { return recorder }

func (recorder *logRecorder) WithGroup(string) slog.Handler { return recorder }

func (recorder *logRecorder) recordsAt(level slog.Level) int {
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	count := 0
	for _, record := range recorder.records {
		if record.Level == level {
			count++
		}
	}
	return count
}

func (recorder *logRecorder) levelsByAttempt() map[int64]slog.Level {
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	levels := map[int64]slog.Level{}
	for _, record := range recorder.records {
		record.Attrs(func(attr slog.Attr) bool {
			if attr.Key == "attempt" {
				levels[attr.Value.Int64()] = record.Level
			}
			return true
		})
	}
	return levels
}

type openedStreaming struct {
	*Streaming
	delivered       chan string
	logs            *logRecorder
	reconnectDelays chan time.Duration
}

func (opened *openedStreaming) letReconnect(t *testing.T) time.Duration {
	t.Helper()
	return await(t, opened.reconnectDelays)
}

type streamingSetup struct {
	options Options
	timing  timing
	deliver func(data string)
}

type streamingAdjustment func(*streamingSetup)

func withHeartbeatInterval(interval time.Duration) streamingAdjustment {
	return func(setup *streamingSetup) { setup.timing.heartbeatInterval = interval }
}

func withStallTimeout(timeout time.Duration) streamingAdjustment {
	return func(setup *streamingSetup) { setup.timing.stallTimeout = timeout }
}

func withConnectTimeout(timeout time.Duration) streamingAdjustment {
	return func(setup *streamingSetup) { setup.timing.connectTimeout = timeout }
}

func withCloseTimeout(timeout time.Duration) streamingAdjustment {
	return func(setup *streamingSetup) { setup.timing.closeTimeout = timeout }
}

func withRealDelays() streamingAdjustment {
	return func(setup *streamingSetup) { setup.timing.wait = waitFor }
}

func withBaseURL(baseURL string) streamingAdjustment {
	return func(setup *streamingSetup) { setup.options.BaseURL = baseURL }
}

func withApp(name string, version string) streamingAdjustment {
	return func(setup *streamingSetup) {
		setup.options.MetaContext.AppName = name
		setup.options.MetaContext.AppVersion = version
	}
}

func withDeliveryBlockedUntil(release <-chan struct{}) streamingAdjustment {
	return func(setup *streamingSetup) {
		deliver := setup.deliver
		setup.deliver = func(data string) {
			<-release
			deliver(data)
		}
	}
}

func openStreamingAgainst(t *testing.T, server *sdkServer, adjustments ...streamingAdjustment) *openedStreaming {
	t.Helper()
	opened := &openedStreaming{
		delivered:       make(chan string, 1024),
		logs:            &logRecorder{},
		reconnectDelays: make(chan time.Duration),
	}
	httpClient := &http.Client{}
	t.Cleanup(httpClient.CloseIdleConnections)

	setup := streamingSetup{
		options: Options{
			SDKKey:      testSDKKey,
			BaseURL:     server.URL,
			MetaContext: MetaContext{SDKName: "go-server-sdk", SDKVersion: "1.2.3"},
			HTTPClient:  httpClient,
			Logger:      slog.New(opened.logs),
		},
		timing:  protocolTiming(),
		deliver: func(data string) { opened.delivered <- data },
	}
	setup.timing.wait = func(ctx context.Context, delay time.Duration) bool {
		select {
		case opened.reconnectDelays <- delay:
			return true
		case <-ctx.Done():
			return false
		}
	}
	for _, adjust := range adjustments {
		adjust(&setup)
	}

	opened.Streaming = openStreaming(setup.options, setup.deliver, setup.timing)
	t.Cleanup(func() { _ = opened.Close() })
	return opened
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

func expectNothing[T any](t *testing.T, values <-chan T, within time.Duration) {
	t.Helper()
	select {
	case value := <-values:
		t.Fatalf("expected nothing, received %v", value)
	case <-time.After(within):
	}
}

func seconds(count float64) time.Duration {
	return time.Duration(count * float64(time.Second))
}

func shareOfTheRandomHalf(delay time.Duration, fixedHalf time.Duration) float64 {
	return float64(delay-fixedHalf) / float64(fixedHalf)
}

func distinct(values []float64) map[float64]bool {
	set := map[float64]bool{}
	for _, value := range values {
		set[value] = true
	}
	return set
}

func requireBetween(t *testing.T, value time.Duration, low time.Duration, high time.Duration) {
	t.Helper()
	require.GreaterOrEqual(t, value, low)
	require.LessOrEqual(t, value, high)
}

func transportGoroutines() int {
	buffer := make([]byte, 1<<16)
	for {
		written := runtime.Stack(buffer, true)
		if written < len(buffer) {
			count := 0
			for _, goroutine := range strings.Split(string(buffer[:written]), "\n\n") {
				if strings.Contains(goroutine, "internal/transport.(*Streaming)") || strings.Contains(goroutine, "internal/eventsource.(*Reader)") {
					count++
				}
			}
			return count
		}
		buffer = make([]byte, 2*len(buffer))
	}
}

func TestOpenStreamingPostsTheSDKKeySessionAndMetaContext(t *testing.T) {
	server := startSDKServer(t, streamHeldOpen())

	openStreamingAgainst(t, server, withApp("checkout", "7.1.0"))

	request := await(t, server.streamRequests)
	require.Equal(t, http.MethodPost, request.method)
	require.Equal(t, "application/json", request.header.Get("Content-Type"))
	require.Equal(t, "text/event-stream", request.header.Get("Accept"))
	require.Equal(t, "go-server-sdk/1.2.3", request.header.Get("User-Agent"))
	require.Empty(t, request.header.Get("Last-Event-ID"))
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

func TestOpenStreamingOmitsAnUnsetAppNameAndVersion(t *testing.T) {
	server := startSDKServer(t, streamHeldOpen())

	openStreamingAgainst(t, server)

	request := await(t, server.streamRequests)
	require.Equal(t, map[string]string{"sdkName": "go-server-sdk", "sdkVersion": "1.2.3"}, request.body.MetaContext)
}

func TestEveryEventOnTheStreamIsDeliveredInOrder(t *testing.T) {
	server := startSDKServer(t, streamHeldOpen("data: first\n\n", "id: 2\ndata: second\ndata: line\n\n"))

	opened := openStreamingAgainst(t, server)

	require.Equal(t, "first", await(t, opened.delivered))
	require.Equal(t, "second\nline", await(t, opened.delivered))
}

func TestADroppedStreamIsReconnectedWithTheLastEventID(t *testing.T) {
	server := startSDKServer(t, streamThenDrop("id: 41\ndata: first\n\n"), streamHeldOpen("data: second\n\n"))

	opened := openStreamingAgainst(t, server)
	require.Equal(t, "first", await(t, opened.delivered))
	opened.letReconnect(t)

	first := await(t, server.streamRequests)
	second := await(t, server.streamRequests)
	require.Empty(t, first.header.Get("Last-Event-ID"))
	require.Equal(t, "41", second.header.Get("Last-Event-ID"))
	require.Equal(t, "second", await(t, opened.delivered))
}

func TestEveryConnectionCarriesANewSessionID(t *testing.T) {
	server := startSDKServer(t, streamThenDrop("data: first\n\n"), streamHeldOpen())

	opened := openStreamingAgainst(t, server)
	opened.letReconnect(t)

	first := await(t, server.streamRequests)
	second := await(t, server.streamRequests)
	require.Regexp(t, uuidVersion4, second.body.SessionID)
	require.NotEqual(t, first.body.SessionID, second.body.SessionID)
}

func TestAStreamThatEndsWithoutAnEventReconnects(t *testing.T) {
	server := startSDKServer(t, streamThenDrop(), streamHeldOpen("data: later\n\n"))

	opened := openStreamingAgainst(t, server)
	opened.letReconnect(t)

	require.Equal(t, "later", await(t, opened.delivered))
}

func TestASilentStreamIsDroppedAndReconnected(t *testing.T) {
	server := startSDKServer(t, streamHeldOpen(), streamHeldOpen("data: alive\n\n"))

	opened := openStreamingAgainst(t, server, withStallTimeout(100*time.Millisecond))
	opened.letReconnect(t)

	require.Equal(t, "alive", await(t, opened.delivered))
	require.Equal(t, int64(2), server.requestCount.Load())
}

func TestAStreamThatKeepsSendingCommentsIsNotDropped(t *testing.T) {
	server := startSDKServer(t, func(writer http.ResponseWriter, request *http.Request) {
		beginEventStream(writer)
		controller := http.NewResponseController(writer)
		for range 8 {
			time.Sleep(30 * time.Millisecond)
			_, _ = io.WriteString(writer, ": keepalive\n")
			_ = controller.Flush()
		}
		_, _ = io.WriteString(writer, "data: late\n\n")
		_ = controller.Flush()
		<-request.Context().Done()
	})

	opened := openStreamingAgainst(t, server, withStallTimeout(200*time.Millisecond))

	require.Equal(t, "late", await(t, opened.delivered))
	require.Equal(t, int64(1), server.requestCount.Load())
}

func TestTheFirstReconnectAfterAnOpenedStreamWaitsBetweenOneAndTwoSeconds(t *testing.T) {
	server := startSDKServer(t, streamThenDrop("data: first\n\n"), streamHeldOpen())

	opened := openStreamingAgainst(t, server)

	requireBetween(t, opened.letReconnect(t), seconds(1), seconds(2))
}

func TestBackoffDelaysStayBetweenHalfAndTheFullPowerOfTwo(t *testing.T) {
	server := startSDKServer(t, refuse(http.StatusServiceUnavailable))

	opened := openStreamingAgainst(t, server)

	requireBetween(t, opened.letReconnect(t), seconds(1), seconds(2))
	requireBetween(t, opened.letReconnect(t), seconds(2), seconds(4))
	requireBetween(t, opened.letReconnect(t), seconds(4), seconds(8))
	requireBetween(t, opened.letReconnect(t), seconds(8), seconds(16))
	requireBetween(t, opened.letReconnect(t), seconds(16), seconds(32))
}

func TestBackoffDelaysAreJitteredRatherThanAFixedShareOfTheCeiling(t *testing.T) {
	server := startSDKServer(t, refuse(http.StatusServiceUnavailable))

	opened := openStreamingAgainst(t, server)

	shares := []float64{
		shareOfTheRandomHalf(opened.letReconnect(t), seconds(1)),
		shareOfTheRandomHalf(opened.letReconnect(t), seconds(2)),
		shareOfTheRandomHalf(opened.letReconnect(t), seconds(4)),
		shareOfTheRandomHalf(opened.letReconnect(t), seconds(8)),
		shareOfTheRandomHalf(opened.letReconnect(t), seconds(16)),
		shareOfTheRandomHalf(opened.letReconnect(t), seconds(32)),
	}
	require.Greater(t, len(distinct(shares)), 1)
}

func TestAServerThatNeverAnswersIsReconnectedAfterTheConnectTimeout(t *testing.T) {
	server := startSDKServer(t, func(_ http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}, streamHeldOpen("data: answered\n\n"))

	opened := openStreamingAgainst(t, server, withConnectTimeout(100*time.Millisecond))
	requireBetween(t, opened.letReconnect(t), seconds(1), seconds(2))

	require.Equal(t, "answered", await(t, opened.delivered))
}

func TestARefusalWhoseBodyNeverEndsIsReconnectedAfterTheConnectTimeout(t *testing.T) {
	server := startSDKServer(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
		_ = http.NewResponseController(writer).Flush()
		<-request.Context().Done()
	}, streamHeldOpen("data: answered\n\n"))

	opened := openStreamingAgainst(t, server, withConnectTimeout(100*time.Millisecond))
	requireBetween(t, opened.letReconnect(t), seconds(1), seconds(2))

	require.Equal(t, "answered", await(t, opened.delivered))
}

func TestAnOpenStreamOutlivesTheConnectTimeout(t *testing.T) {
	server := startSDKServer(t, func(writer http.ResponseWriter, request *http.Request) {
		beginEventStream(writer)
		time.Sleep(250 * time.Millisecond)
		_, _ = io.WriteString(writer, "data: late\n\n")
		_ = http.NewResponseController(writer).Flush()
		<-request.Context().Done()
	})

	opened := openStreamingAgainst(t, server, withConnectTimeout(100*time.Millisecond))

	require.Equal(t, "late", await(t, opened.delivered))
	require.Equal(t, int64(1), server.requestCount.Load())
}

func TestBackoffDelaysAreCappedAtFiveHundredAndTwelveSeconds(t *testing.T) {
	server := startSDKServer(t, refuse(http.StatusServiceUnavailable))

	opened := openStreamingAgainst(t, server)
	for range 9 {
		opened.letReconnect(t)
	}

	requireBetween(t, opened.letReconnect(t), seconds(256), seconds(512))
	requireBetween(t, opened.letReconnect(t), seconds(256), seconds(512))
}

func TestAnOpenedStreamResetsTheAttemptCount(t *testing.T) {
	server := startSDKServer(t,
		refuse(http.StatusServiceUnavailable),
		refuse(http.StatusServiceUnavailable),
		streamThenDrop("data: back\n\n"),
		streamHeldOpen(),
	)

	opened := openStreamingAgainst(t, server)
	requireBetween(t, opened.letReconnect(t), seconds(1), seconds(2))
	requireBetween(t, opened.letReconnect(t), seconds(2), seconds(4))
	require.Equal(t, "back", await(t, opened.delivered))

	requireBetween(t, opened.letReconnect(t), seconds(1), seconds(2))
}

func TestReconnectsAreLoggedAtInfoForFiveAttemptsThenAtWarn(t *testing.T) {
	server := startSDKServer(t, refuse(http.StatusServiceUnavailable))

	opened := openStreamingAgainst(t, server)
	for range 7 {
		opened.letReconnect(t)
	}

	require.Equal(t, map[int64]slog.Level{
		1: slog.LevelInfo,
		2: slog.LevelInfo,
		3: slog.LevelInfo,
		4: slog.LevelInfo,
		5: slog.LevelInfo,
		6: slog.LevelWarn,
		7: slog.LevelWarn,
	}, opened.logs.levelsByAttempt())
}

func TestA401StopsRetryingWithOneErrorLog(t *testing.T) {
	server := startSDKServer(t, refuse(http.StatusUnauthorized))

	opened := openStreamingAgainst(t, server)

	await(t, server.streamRequests)
	expectNothing(t, opened.reconnectDelays, 200*time.Millisecond)
	require.Equal(t, int64(1), server.requestCount.Load())
	require.Equal(t, 1, opened.logs.recordsAt(slog.LevelError))
}

func TestA404StopsRetryingWithOneErrorLog(t *testing.T) {
	server := startSDKServer(t, refuse(http.StatusNotFound))

	opened := openStreamingAgainst(t, server)

	await(t, server.streamRequests)
	expectNothing(t, opened.reconnectDelays, 200*time.Millisecond)
	require.Equal(t, int64(1), server.requestCount.Load())
	require.Equal(t, 1, opened.logs.recordsAt(slog.LevelError))
}

func TestA503IsRetriedWithoutAnErrorLog(t *testing.T) {
	server := startSDKServer(t, refuse(http.StatusServiceUnavailable), streamHeldOpen("data: recovered\n\n"))

	opened := openStreamingAgainst(t, server)
	opened.letReconnect(t)

	require.Equal(t, "recovered", await(t, opened.delivered))
	require.Equal(t, 0, opened.logs.recordsAt(slog.LevelError))
}

func TestAnUnreachableServerIsRetried(t *testing.T) {
	server := startSDKServer(t, streamHeldOpen())
	server.Close()

	opened := openStreamingAgainst(t, server)

	requireBetween(t, opened.letReconnect(t), seconds(1), seconds(2))
	requireBetween(t, opened.letReconnect(t), seconds(2), seconds(4))
	require.Equal(t, 0, opened.logs.recordsAt(slog.LevelError))
}

func TestA429WaitsTheRetryAfterSeconds(t *testing.T) {
	server := startSDKServer(t, refuseWithRetryAfter(http.StatusTooManyRequests, "2"), streamHeldOpen("data: admitted\n\n"))

	opened := openStreamingAgainst(t, server)

	require.Equal(t, 2*time.Second, opened.letReconnect(t))
	require.Equal(t, "admitted", await(t, opened.delivered))
	require.Equal(t, 0, opened.logs.recordsAt(slog.LevelError))
}

func TestA429WithoutRetryAfterBacksOff(t *testing.T) {
	server := startSDKServer(t, refuse(http.StatusTooManyRequests))

	opened := openStreamingAgainst(t, server)

	requireBetween(t, opened.letReconnect(t), seconds(1), seconds(2))
	requireBetween(t, opened.letReconnect(t), seconds(2), seconds(4))
}

func TestTheRetryFieldOfTheStreamSetsTheReconnectDelay(t *testing.T) {
	server := startSDKServer(t, streamThenDrop("retry: 3000\ndata: first\n\n"), streamHeldOpen())

	opened := openStreamingAgainst(t, server)

	require.Equal(t, 3*time.Second, opened.letReconnect(t))
}

func TestTheRetryFieldAppliesToTheStreamThatSentItOnly(t *testing.T) {
	server := startSDKServer(t, streamThenDrop("retry: 3000\ndata: first\n\n"), refuse(http.StatusServiceUnavailable))

	opened := openStreamingAgainst(t, server)
	require.Equal(t, 3*time.Second, opened.letReconnect(t))

	requireBetween(t, opened.letReconnect(t), seconds(2), seconds(4))
}

func TestAnUnsupportedSchemeStopsWithOneErrorLogAndNoRequest(t *testing.T) {
	server := startSDKServer(t, streamHeldOpen())

	opened := openStreamingAgainst(t, server, withBaseURL("ftp://example.invalid"))

	require.Eventually(t, func() bool { return opened.logs.recordsAt(slog.LevelError) == 1 }, awaitTimeout, 10*time.Millisecond)
	expectNothing(t, server.streamRequests, 100*time.Millisecond)
	require.NoError(t, opened.Close())
}

func TestAnUnparsableURLStopsWithOneErrorLogAndNoRequest(t *testing.T) {
	server := startSDKServer(t, streamHeldOpen())

	opened := openStreamingAgainst(t, server, withBaseURL("http://[::1]:namedport"))

	require.Eventually(t, func() bool { return opened.logs.recordsAt(slog.LevelError) == 1 }, awaitTimeout, 10*time.Millisecond)
	expectNothing(t, server.streamRequests, 100*time.Millisecond)
}

func TestAProxyBaseURLKeepsItsPath(t *testing.T) {
	proxied := startSDKServer(t, streamHeldOpen())
	mux := http.NewServeMux()
	mux.Handle("/proxy/sdk/", http.StripPrefix("/proxy/sdk", proxied.Config.Handler))
	proxy := httptest.NewServer(mux)
	t.Cleanup(proxy.Close)

	openStreamingAgainst(t, proxied, withBaseURL(proxy.URL+"/proxy/sdk"))

	await(t, proxied.streamRequests)
}

func TestHeartbeatsCarryTheSessionIDOfTheOpenStream(t *testing.T) {
	server := startSDKServer(t, streamHeldOpen())

	openStreamingAgainst(t, server, withHeartbeatInterval(30*time.Millisecond))

	streamRequest := await(t, server.streamRequests)
	heartbeat := await(t, server.heartbeats)
	require.Equal(t, http.MethodPost, heartbeat.method)
	require.Equal(t, "application/json", heartbeat.header.Get("Content-Type"))
	require.Equal(t, "go-server-sdk/1.2.3", heartbeat.header.Get("User-Agent"))
	require.Equal(t, testSDKKey, heartbeat.body.ServerSDKKey)
	require.Equal(t, streamRequest.body.SessionID, heartbeat.body.SessionID)
	require.ElementsMatch(t, []string{"serverSdkKey", "sessionId"}, heartbeat.body.fieldNames)
}

func TestHeartbeatsKeepComingWhileTheStreamStaysOpen(t *testing.T) {
	server := startSDKServer(t, streamHeldOpen())

	openStreamingAgainst(t, server, withHeartbeatInterval(20*time.Millisecond))

	first := await(t, server.heartbeats)
	second := await(t, server.heartbeats)
	third := await(t, server.heartbeats)
	require.Equal(t, first.body.SessionID, second.body.SessionID)
	require.Equal(t, first.body.SessionID, third.body.SessionID)
}

func TestHeartbeatsSwitchToTheNewSessionAfterAReconnect(t *testing.T) {
	server := startSDKServer(t, streamThenDrop("data: first\n\n"), streamHeldOpen())

	opened := openStreamingAgainst(t, server, withHeartbeatInterval(30*time.Millisecond))
	opened.letReconnect(t)
	await(t, server.streamRequests)
	second := await(t, server.streamRequests)

	require.Eventually(t, func() bool {
		select {
		case heartbeat := <-server.heartbeats:
			return heartbeat.body.SessionID == second.body.SessionID
		default:
			return false
		}
	}, awaitTimeout, 5*time.Millisecond)
}

func TestNoHeartbeatIsSentWhileTheStreamIsDown(t *testing.T) {
	server := startSDKServer(t, refuse(http.StatusServiceUnavailable))

	opened := openStreamingAgainst(t, server, withHeartbeatInterval(10*time.Millisecond))
	opened.letReconnect(t)

	expectNothing(t, server.heartbeats, 100*time.Millisecond)
}

func TestNoHeartbeatIsSentAfterTheStreamDropped(t *testing.T) {
	server := startSDKServer(t, streamThenDrop("data: first\n\n"), refuse(http.StatusServiceUnavailable))

	opened := openStreamingAgainst(t, server, withHeartbeatInterval(10*time.Millisecond))
	opened.letReconnect(t)
	opened.letReconnect(t)
	time.Sleep(50 * time.Millisecond)
	for len(server.heartbeats) > 0 {
		<-server.heartbeats
	}

	expectNothing(t, server.heartbeats, 100*time.Millisecond)
}

func TestAFailedHeartbeatLeavesTheStreamAlone(t *testing.T) {
	server := startSDKServer(t, streamHeldOpen("data: first\n\n"))
	server.heartbeatStatus.Store(http.StatusInternalServerError)

	opened := openStreamingAgainst(t, server, withHeartbeatInterval(20*time.Millisecond))
	require.Equal(t, "first", await(t, opened.delivered))
	await(t, server.heartbeats)
	await(t, server.heartbeats)

	require.Equal(t, int64(1), server.requestCount.Load())
	require.Equal(t, 0, opened.logs.recordsAt(slog.LevelError))
	require.Equal(t, 0, opened.logs.recordsAt(slog.LevelWarn))
}

func TestCloseLeavesNoGoroutineBehind(t *testing.T) {
	server := startSDKServer(t, streamHeldOpen("data: first\n\n"))
	opened := openStreamingAgainst(t, server, withHeartbeatInterval(10*time.Millisecond))
	require.Equal(t, "first", await(t, opened.delivered))
	await(t, server.heartbeats)
	require.Positive(t, transportGoroutines())

	require.NoError(t, opened.Close())

	require.Eventually(t, func() bool { return transportGoroutines() == 0 }, awaitTimeout, 10*time.Millisecond)
}

func TestCloseWhileWaitingToReconnectReturnsAtOnce(t *testing.T) {
	server := startSDKServer(t, refuse(http.StatusServiceUnavailable))
	opened := openStreamingAgainst(t, server, withRealDelays())
	await(t, server.streamRequests)
	closingAt := time.Now()

	err := opened.Close()

	require.NoError(t, err)
	require.Less(t, time.Since(closingAt), 500*time.Millisecond)
	require.Eventually(t, func() bool { return transportGoroutines() == 0 }, awaitTimeout, 10*time.Millisecond)
}

func TestCloseWhileAStreamIsOpenReturnsAtOnce(t *testing.T) {
	server := startSDKServer(t, streamHeldOpen("data: first\n\n"))
	opened := openStreamingAgainst(t, server)
	require.Equal(t, "first", await(t, opened.delivered))
	closingAt := time.Now()

	err := opened.Close()

	require.NoError(t, err)
	require.Less(t, time.Since(closingAt), 500*time.Millisecond)
}

func TestCloseReportsATimeoutWhenADeliveryHandlerDoesNotReturn(t *testing.T) {
	server := startSDKServer(t, streamHeldOpen("data: first\n\n"))
	release := make(chan struct{})
	opened := openStreamingAgainst(t, server, withCloseTimeout(100*time.Millisecond), withDeliveryBlockedUntil(release))
	await(t, server.streamRequests)
	time.Sleep(50 * time.Millisecond)

	err := opened.Close()

	require.ErrorIs(t, err, errCloseTimedOut)
	close(release)
	require.Eventually(t, func() bool { return transportGoroutines() == 0 }, awaitTimeout, 10*time.Millisecond)
}

func TestCloseAfterAFatalStopReturnsNil(t *testing.T) {
	server := startSDKServer(t, refuse(http.StatusUnauthorized))
	opened := openStreamingAgainst(t, server)
	require.Eventually(t, func() bool { return opened.logs.recordsAt(slog.LevelError) == 1 }, awaitTimeout, 10*time.Millisecond)

	require.NoError(t, opened.Close())
	require.NoError(t, opened.Close())
}

func TestNothingIsDeliveredAfterClose(t *testing.T) {
	server := startSDKServer(t, streamHeldOpen("data: first\n\n"))
	opened := openStreamingAgainst(t, server)
	require.Equal(t, "first", await(t, opened.delivered))

	require.NoError(t, opened.Close())

	expectNothing(t, opened.delivered, 100*time.Millisecond)
}

func TestAnEventBufferedBeforeCloseIsNotDeliveredAfterIt(t *testing.T) {
	server := startSDKServer(t, streamHeldOpen("data: first\n\ndata: second\n\n"))
	release := make(chan struct{})
	opened := openStreamingAgainst(t, server, withDeliveryBlockedUntil(release))
	await(t, server.streamRequests)
	time.Sleep(50 * time.Millisecond)
	closed := make(chan error, 1)
	go func() { closed <- opened.Close() }()
	time.Sleep(50 * time.Millisecond)

	close(release)

	require.NoError(t, await(t, closed))
	require.Equal(t, "first", await(t, opened.delivered))
	expectNothing(t, opened.delivered, 100*time.Millisecond)
}

func TestTheProtocolTimingIsNinetySecondHeartbeatsAndAFortyFiveSecondStall(t *testing.T) {
	timing := protocolTiming()

	require.Equal(t, 90*time.Second, timing.heartbeatInterval)
	require.Equal(t, 45*time.Second, timing.stallTimeout)
	require.Equal(t, 30*time.Second, timing.connectTimeout)
	require.Equal(t, 5*time.Second, timing.closeTimeout)
}

func TestBackoffDelayIsHalfTheCeilingAtZeroRandomness(t *testing.T) {
	require.Equal(t, 4*time.Second, backoffDelay(3, 0))
}

func TestBackoffDelayApproachesTheCeilingAtFullRandomness(t *testing.T) {
	requireBetween(t, backoffDelay(3, 0.999999), seconds(7.99), seconds(8))
}

func TestBackoffDelayTreatsAttemptZeroAsAttemptOne(t *testing.T) {
	require.Equal(t, time.Second, backoffDelay(0, 0))
}

func TestBackoffDelayCapsTheExponentAtNine(t *testing.T) {
	require.Equal(t, backoffDelay(9, 0.5), backoffDelay(40, 0.5))
	require.Equal(t, 384*time.Second, backoffDelay(40, 0.5))
}

func TestRetryAfterDelayReadsWholeSeconds(t *testing.T) {
	delay, known := retryAfterDelay("300", time.Now())

	require.True(t, known)
	require.Equal(t, 300*time.Second, delay)
}

func TestRetryAfterDelayReadsAnHTTPDate(t *testing.T) {
	now := time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)

	delay, known := retryAfterDelay(now.Add(90*time.Second).Format(http.TimeFormat), now)

	require.True(t, known)
	require.Equal(t, 90*time.Second, delay)
}

func TestRetryAfterDelayTreatsADateInThePastAsZero(t *testing.T) {
	now := time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)

	delay, known := retryAfterDelay(now.Add(-time.Minute).Format(http.TimeFormat), now)

	require.True(t, known)
	require.Equal(t, time.Duration(0), delay)
}

func TestRetryAfterDelayIsUnknownForAnEmptyHeader(t *testing.T) {
	_, known := retryAfterDelay("", time.Now())

	require.False(t, known)
}

func TestRetryAfterDelayIsUnknownForANegativeNumber(t *testing.T) {
	_, known := retryAfterDelay("-5", time.Now())

	require.False(t, known)
}

func TestRetryAfterDelayIsUnknownForText(t *testing.T) {
	_, known := retryAfterDelay("soon", time.Now())

	require.False(t, known)
}

func TestNextStepStopsQuietlyOnceClosed(t *testing.T) {
	step := nextStepAfter(streamEnd{kind: streamEndClosed}, 3, 0.5)

	require.Equal(t, nextStep{kind: nextStepStopClosed}, step)
}

func TestNextStepStopsForGoodOnAnUnusableURL(t *testing.T) {
	step := nextStepAfter(streamEnd{kind: streamEndURLUnusable}, 0, 0.5)

	require.Equal(t, nextStep{kind: nextStepStopFatal}, step)
}

func TestNextStepStopsForGoodOnA403(t *testing.T) {
	step := nextStepAfter(streamEnd{kind: streamEndRefused, status: http.StatusForbidden}, 0, 0.5)

	require.Equal(t, nextStep{kind: nextStepStopFatal}, step)
}

func TestNextStepReconnectsAfterA500(t *testing.T) {
	step := nextStepAfter(streamEnd{kind: streamEndRefused, status: http.StatusInternalServerError}, 2, 0)

	require.Equal(t, nextStep{kind: nextStepReconnect, attempt: 3, delay: 4 * time.Second, logLevel: slog.LevelInfo}, step)
}

func TestNextStepReconnectsAfterA429WithItsRetryAfter(t *testing.T) {
	end := streamEnd{kind: streamEndRefused, status: http.StatusTooManyRequests, delayNamedByServer: 300 * time.Second, delayNamedByServerKnown: true}

	step := nextStepAfter(end, 7, 0)

	require.Equal(t, nextStep{kind: nextStepReconnect, attempt: 8, delay: 300 * time.Second, logLevel: slog.LevelWarn}, step)
}

func TestNextStepStartsOverAtAttemptOneAfterADroppedStream(t *testing.T) {
	step := nextStepAfter(streamEnd{kind: streamEndDropped}, 7, 0)

	require.Equal(t, nextStep{kind: nextStepReconnect, attempt: 1, delay: time.Second, logLevel: slog.LevelInfo}, step)
}

func TestNextStepCountsAnUnreachableServerAsAnotherAttempt(t *testing.T) {
	step := nextStepAfter(streamEnd{kind: streamEndUnreachable}, 5, 0)

	require.Equal(t, nextStep{kind: nextStepReconnect, attempt: 6, delay: 32 * time.Second, logLevel: slog.LevelWarn}, step)
}

func TestResponseDetailIsCappedAtThirtyTwoMiB(t *testing.T) {
	body := strings.NewReader(strings.Repeat("x", maxResponseBodyBytes+1))

	detail := responseDetail(body)

	require.Len(t, detail, maxResponseBodyBytes)
}

func TestResponseDetailTrimsSurroundingWhitespace(t *testing.T) {
	detail := responseDetail(strings.NewReader("  Service suspended\n"))

	require.Equal(t, "Service suspended", detail)
}
