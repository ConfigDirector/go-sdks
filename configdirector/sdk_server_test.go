package configdirector_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector"
)

const (
	testSDKKey = "test-server-sdk-key"

	streamPath    = "/server/sse/v1"
	pollingPath   = "/server/polling/v1"
	heartbeatPath = "/server/heartbeat/v1"
	telemetryPath = "/server/telemetry/v1"

	awaitTimeout = 5 * time.Second
)

type recordedRequest struct {
	path     string
	header   http.Header
	body     map[string]any
	bodyJSON []byte
}

type sdkServer struct {
	*httptest.Server

	requests          chan recordedRequest
	telemetryRequests chan recordedRequest
	initialPayloads   []string
	mutex             sync.Mutex
	streams           map[chan string]struct{}
	pollsAnswered     int
	nextRefusal       int
	refusalBody       string
	dropNext          bool
	telemetryStatus   int
}

func startSDKServer(t testing.TB, initialPayloads ...string) *sdkServer {
	t.Helper()
	return startSDKServerUnder(t, "", initialPayloads...)
}

func startSDKServerUnder(t testing.TB, pathPrefix string, initialPayloads ...string) *sdkServer {
	t.Helper()
	return serveSDKServer(t, pathPrefix, httptest.NewServer, initialPayloads)
}

func startTLSSDKServer(t testing.TB, initialPayloads ...string) *sdkServer {
	t.Helper()
	return serveSDKServer(t, "", httptest.NewTLSServer, initialPayloads)
}

func serveSDKServer(t testing.TB, pathPrefix string, serve func(http.Handler) *httptest.Server, initialPayloads []string) *sdkServer {
	t.Helper()
	server := &sdkServer{
		requests:          make(chan recordedRequest, 1024),
		telemetryRequests: make(chan recordedRequest, 1024),
		initialPayloads:   initialPayloads,
		streams:           map[chan string]struct{}{},
		telemetryStatus:   http.StatusNoContent,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST "+pathPrefix+streamPath, server.serveStream)
	mux.HandleFunc("POST "+pathPrefix+pollingPath, server.servePoll)
	mux.HandleFunc("POST "+pathPrefix+heartbeatPath, func(writer http.ResponseWriter, request *http.Request) {
		server.record(request)
		writer.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST "+pathPrefix+telemetryPath, server.serveTelemetry)
	server.Server = serve(mux)
	t.Cleanup(server.Close)
	return server
}

func (server *sdkServer) options() configdirector.Options {
	return configdirector.Options{
		SDKKey:     testSDKKey,
		Connection: configdirector.ConnectionOptions{URL: server.URL, HTTPClient: server.Client()},
	}
}

func (server *sdkServer) send(payload string) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	for stream := range server.streams {
		stream <- payload
	}
}

func (server *sdkServer) refuseNextRequest(status int) {
	server.refuseNextRequestWith(status, "")
}

func (server *sdkServer) refuseNextRequestWith(status int, body string) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	server.nextRefusal = status
	server.refusalBody = body
}

func (server *sdkServer) answerTelemetryWith(status int) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	server.telemetryStatus = status
}

func (server *sdkServer) dropNextConnection() {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	server.dropNext = true
}

func (server *sdkServer) refused(writer http.ResponseWriter) bool {
	server.mutex.Lock()
	status := server.nextRefusal
	body := server.refusalBody
	drop := server.dropNext
	server.nextRefusal = 0
	server.refusalBody = ""
	server.dropNext = false
	server.mutex.Unlock()
	switch {
	case drop:
		if connection, _, err := http.NewResponseController(writer).Hijack(); err == nil {
			_ = connection.Close()
		}
		return true
	case status != 0:
		writer.WriteHeader(status)
		_, _ = io.WriteString(writer, body)
		return true
	default:
		return false
	}
}

func (server *sdkServer) record(request *http.Request) {
	server.requests <- recordedRequestFrom(request)
}

func recordedRequestFrom(request *http.Request) recordedRequest {
	raw, _ := io.ReadAll(request.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	return recordedRequest{path: request.URL.Path, header: request.Header.Clone(), body: body, bodyJSON: raw}
}

func (server *sdkServer) serveTelemetry(writer http.ResponseWriter, request *http.Request) {
	server.telemetryRequests <- recordedRequestFrom(request)
	server.mutex.Lock()
	status := server.telemetryStatus
	server.mutex.Unlock()
	writer.WriteHeader(status)
}

func (server *sdkServer) serveStream(writer http.ResponseWriter, request *http.Request) {
	if server.refused(writer) {
		server.record(request)
		return
	}
	stream := server.openStream()
	defer server.closeStream(stream)
	server.record(request)

	writer.Header().Set("Content-Type", "text/event-stream")
	writer.WriteHeader(http.StatusOK)
	controller := http.NewResponseController(writer)
	_ = controller.Flush()
	for _, payload := range server.initialPayloads {
		writeEvent(writer, controller, payload)
	}
	for {
		select {
		case payload := <-stream:
			writeEvent(writer, controller, payload)
		case <-request.Context().Done():
			return
		}
	}
}

func (server *sdkServer) openStream() chan string {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	stream := make(chan string, 1024)
	server.streams[stream] = struct{}{}
	return stream
}

func (server *sdkServer) closeStream(stream chan string) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	delete(server.streams, stream)
}

func writeEvent(writer io.Writer, controller *http.ResponseController, payload string) {
	_, _ = io.WriteString(writer, "data: "+payload+"\n\n")
	_ = controller.Flush()
}

func (server *sdkServer) servePoll(writer http.ResponseWriter, request *http.Request) {
	server.record(request)
	if server.refused(writer) {
		return
	}
	server.mutex.Lock()
	answered := server.pollsAnswered
	server.pollsAnswered++
	server.mutex.Unlock()

	if answered >= len(server.initialPayloads) {
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(writer, server.initialPayloads[answered])
}

func await[T any](t testing.TB, values <-chan T) T {
	t.Helper()
	return awaitFor(t, values, awaitTimeout)
}

func awaitFor[T any](t testing.TB, values <-chan T, timeout time.Duration) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(timeout):
		t.Fatal("nothing arrived within the await timeout")
		var zero T
		return zero
	}
}

type recorder[T any] struct {
	values chan T
}

func newRecorder[T any]() *recorder[T] {
	return &recorder[T]{values: make(chan T, 1024)}
}

func (recorder *recorder[T]) record(value T) {
	recorder.values <- value
}

func (recorder *recorder[T]) next(t testing.TB) T {
	t.Helper()
	return await(t, recorder.values)
}

func (recorder *recorder[T]) received() []T {
	received := []T{}
	for {
		select {
		case value := <-recorder.values:
			received = append(received, value)
		default:
			return received
		}
	}
}
