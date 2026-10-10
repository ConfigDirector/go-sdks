package wrapper_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/ConfigDirector/go-sdks/configdirector/wrapper"
	"github.com/stretchr/testify/require"
)

const (
	pollingPath   = "/server/polling/v1"
	telemetryPath = "/server/telemetry/v1"

	configState = `{"payloadVersion":1,"kind":"full","configs":{"greeting":{"id":"config-1","key":"greeting","type":"string","target":{"rules":[],"defaultValue":"hello","defaultValueId":"value-1"}}},"segments":{}}`
)

type recordedRequest struct {
	userAgent   string
	metaContext map[string]any
}

type sdkServer struct {
	*httptest.Server
	polls     chan recordedRequest
	telemetry chan recordedRequest
}

func startSDKServer(t *testing.T) *sdkServer {
	t.Helper()
	server := &sdkServer{polls: make(chan recordedRequest, 16), telemetry: make(chan recordedRequest, 16)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+pollingPath, func(writer http.ResponseWriter, request *http.Request) {
		server.polls <- recordedRequestFrom(request)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, configState)
	})
	mux.HandleFunc("POST "+telemetryPath, func(writer http.ResponseWriter, request *http.Request) {
		server.telemetry <- recordedRequestFrom(request)
		writer.WriteHeader(http.StatusNoContent)
	})
	server.Server = httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func recordedRequestFrom(request *http.Request) recordedRequest {
	var body struct {
		MetaContext map[string]any `json:"metaContext"`
	}
	_ = json.NewDecoder(request.Body).Decode(&body)
	return recordedRequest{userAgent: request.Header.Get("User-Agent"), metaContext: body.MetaContext}
}

func (server *sdkServer) options() configdirector.Options {
	return configdirector.Options{
		SDKKey:     "test-server-sdk-key",
		Connection: configdirector.ConnectionOptions{Mode: configdirector.ConnectionModePolling, URL: server.URL, HTTPClient: server.Client()},
	}
}

func await[T any](t *testing.T, values <-chan T) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("nothing arrived within the await timeout")
		var zero T
		return zero
	}
}

func newClient(t *testing.T, module wrapper.Module, version string, options configdirector.Options) *configdirector.Client {
	t.Helper()
	client, err := wrapper.NewClient(module, version, options)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return client
}

func TestAWrapperModuleIdentifiesItselfToTheSDKServerInTheSDKsPlace(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t)
	options := server.options()
	options.Metadata = configdirector.Metadata{AppName: "checkout", AppVersion: "7.1.0"}

	newClient(t, wrapper.OpenFeatureProvider, "1.2.3", options)

	request := await(t, server.polls)
	require.Equal(t, "go-openfeature-server-provider/1.2.3", request.userAgent)
	require.Equal(t, map[string]any{
		"sdkName":    "go-openfeature-server-provider",
		"sdkVersion": "1.2.3",
		"appName":    "checkout",
		"appVersion": "7.1.0",
	}, request.metaContext)
}

func TestAWrapperModuleReportsTelemetryInTheSDKsPlace(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t)
	client := newClient(t, wrapper.OpenFeatureProvider, "1.2.3", server.options())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, client.WaitForReady(ctx))
	require.Equal(t, "hello", client.StringValue("greeting", "fallback", configdirector.UserContext{}))

	require.NoError(t, client.Close())

	request := await(t, server.telemetry)
	require.Equal(t, "go-openfeature-server-provider/1.2.3", request.userAgent)
	require.Equal(t, map[string]any{"sdkName": "go-openfeature-server-provider", "sdkVersion": "1.2.3"}, request.metaContext)
}

func TestTheClientOfAWrapperModuleIsTheSDKsClient(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t)
	client := newClient(t, wrapper.OpenFeatureProvider, "1.2.3", server.options())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, client.WaitForReady(ctx))

	value, details := client.StringValueDetails("greeting", "fallback", configdirector.UserContext{})
	require.Equal(t, "hello", value)
	require.Equal(t, configdirector.EvaluationDetails{Reason: configdirector.EvaluationReasonFoundMatch, ValueID: "value-1"}, details)
}

func TestAModuleThatIsNotDeclaredIsRejected(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t)

	client, err := wrapper.NewClient(wrapper.Module{}, "1.2.3", server.options())

	require.Nil(t, client)
	require.ErrorContains(t, err, "not a module this package declares")
}

func TestAVersionThatIsNotASemanticVersionIsRejected(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t)

	client, err := wrapper.NewClient(wrapper.OpenFeatureProvider, "v1.2.3", server.options())

	require.Nil(t, client)
	require.ErrorContains(t, err, `the version "v1.2.3" is not a semantic version`)
}

func TestTheOptionsAreValidatedAsNewClientValidatesThem(t *testing.T) {
	t.Parallel()
	options := startSDKServer(t).options()
	options.SDKKey = " "

	client, err := wrapper.NewClient(wrapper.OpenFeatureProvider, "1.2.3", options)

	require.Nil(t, client)
	require.ErrorContains(t, err, "the SDK key is blank")
}
