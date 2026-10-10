package configdirectoropenfeature_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector"
	configdirectoropenfeature "github.com/ConfigDirector/go-sdks/configdirector-openfeature"
	"github.com/open-feature/go-sdk/openfeature"
	"github.com/open-feature/go-sdk/openfeature/isolated"
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
	polls chan recordedRequest
}

func startSDKServer(t *testing.T) *sdkServer {
	t.Helper()
	server := &sdkServer{polls: make(chan recordedRequest, 16)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+pollingPath, func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			MetaContext map[string]any `json:"metaContext"`
		}
		_ = json.NewDecoder(request.Body).Decode(&body)
		server.polls <- recordedRequest{userAgent: request.Header.Get("User-Agent"), metaContext: body.MetaContext}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, configState)
	})
	mux.HandleFunc("POST "+telemetryPath, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	})
	server.Server = httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func (server *sdkServer) options() configdirector.Options {
	return configdirector.Options{
		SDKKey:     "test-server-sdk-key",
		Connection: configdirector.ConnectionOptions{Mode: configdirector.ConnectionModePolling, URL: server.URL, HTTPClient: server.Client()},
	}
}

func registerProvider(t *testing.T, options configdirector.Options) *openfeature.Client {
	t.Helper()
	provider, err := configdirectoropenfeature.NewProvider(options)
	require.NoError(t, err)
	api := isolated.NewAPI()
	t.Cleanup(func() { require.NoError(t, api.Shutdown(context.Background())) })
	require.NoError(t, api.SetProviderAndWait(t.Context(), provider))
	return api.NewClient()
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

func TestTheProviderIdentifiesItselfToTheSDKServerAndResolvesThroughOpenFeature(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t)
	options := server.options()
	options.Metadata = configdirector.Metadata{AppName: "checkout", AppVersion: "7.1.0"}

	client := registerProvider(t, options)

	request := await(t, server.polls)
	require.Equal(t, "go-openfeature-server-provider/"+configdirectoropenfeature.Version, request.userAgent)
	require.Equal(t, map[string]any{
		"sdkName":    "go-openfeature-server-provider",
		"sdkVersion": configdirectoropenfeature.Version,
		"appName":    "checkout",
		"appVersion": "7.1.0",
	}, request.metaContext)
	require.Equal(t, "hello", client.String(t.Context(), "greeting", "fallback", openfeature.EvaluationContext{}))
}

func TestTheApplicationsHooksStillRun(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t)
	options := server.options()
	ready := make(chan struct{}, 1)
	updated := make(chan configdirector.ConfigsUpdated, 1)
	evaluated := make(chan configdirector.Evaluation, 1)
	options.Hooks = configdirector.Hooks{
		OnReady:           func() { ready <- struct{}{} },
		OnConfigsUpdated:  func(update configdirector.ConfigsUpdated) { updated <- update },
		OnConfigEvaluated: func(evaluation configdirector.Evaluation) { evaluated <- evaluation },
	}

	client := registerProvider(t, options)
	client.String(t.Context(), "greeting", "fallback", openfeature.EvaluationContext{})

	await(t, ready)
	require.Equal(t, []string{"greeting"}, await(t, updated).Keys)
	require.Equal(t, "greeting", await(t, evaluated).Key)
}

func TestNewProviderRejectsTheOptionsNewClientRejects(t *testing.T) {
	t.Parallel()

	provider, err := configdirectoropenfeature.NewProvider(configdirector.Options{SDKKey: " "})

	require.Nil(t, provider)
	require.ErrorContains(t, err, "the SDK key is blank")
}
