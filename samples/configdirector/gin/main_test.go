package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/ConfigDirector/go-sdks/configdirector/configdirectortest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var servedValues = map[string]any{
	"temporary-feature-flag": false,
	"permanent-kill-switch":  true,
	"integer-config":         20,
	"day-of-the-week-config": "Monday",
	"json-value-config":      map[string]any{"color": "blue"},
}

func TestConfigsReturnsTheFiveConfigsAsServed(t *testing.T) {
	t.Parallel()
	router := newRouter(newTestClient(t, servedValues).Client())

	response := get(router, "/configs?id=user-123&plan=pro")

	require.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `{
		"temporary-feature-flag": false,
		"permanent-kill-switch": true,
		"integer-config": 20,
		"day-of-the-week-config": "Monday",
		"json-value-config": {"color": "blue"}
	}`, response.Body.String())
}

func TestConfigsReturnsTheInCodeDefaultValuesWhenNoneAreServed(t *testing.T) {
	t.Parallel()
	router := newRouter(newTestClient(t, nil).Client())

	response := get(router, "/configs?id=user-123&plan=pro")

	require.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `{
		"temporary-feature-flag": true,
		"permanent-kill-switch": false,
		"integer-config": 10,
		"day-of-the-week-config": "Friday",
		"json-value-config": {}
	}`, response.Body.String())
}

func TestConfigsFollowAValueChangedAfterTheFirstRequest(t *testing.T) {
	t.Parallel()
	testClient := newTestClient(t, servedValues)
	router := newRouter(testClient.Client())
	get(router, "/configs?id=user-123")

	require.NoError(t, testClient.SetValue("integer-config", 30))
	response := get(router, "/configs?id=user-123")

	require.Equal(t, http.StatusOK, response.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	assert.Equal(t, float64(30), body["integer-config"])
}

func TestTheQueryStringBecomesTheUserContextOfEveryEvaluation(t *testing.T) {
	t.Parallel()
	evaluationsByKey := map[string]configdirector.Evaluation{}
	recordEvaluation := func(evaluation configdirector.Evaluation) {
		evaluationsByKey[evaluation.Key] = evaluation
	}
	router := newRouter(newTestClient(t, nil, configdirectortest.WithHooks(configdirector.Hooks{OnConfigEvaluated: recordEvaluation})).Client())

	get(router, "/configs?id=user-123&name=Ada&anonymous=true&plan=pro&region=eu")

	ada := configdirector.UserContext{
		ID:        "user-123",
		Name:      "Ada",
		Traits:    map[string]any{"plan": "pro", "region": "eu"},
		Anonymous: true,
	}
	assert.Equal(t, map[string]configdirector.Evaluation{
		"temporary-feature-flag": evaluatedToTheDefaultFor(ada, "temporary-feature-flag", true),
		"permanent-kill-switch":  evaluatedToTheDefaultFor(ada, "permanent-kill-switch", false),
		"integer-config":         evaluatedToTheDefaultFor(ada, "integer-config", int64(10)),
		"day-of-the-week-config": evaluatedToTheDefaultFor(ada, "day-of-the-week-config", "Friday"),
		"json-value-config":      evaluatedToTheDefaultFor(ada, "json-value-config", map[string]any{}),
	}, evaluationsByKey)
}

func TestAnUnknownRouteReturnsAHintToTheConfigsRoute(t *testing.T) {
	t.Parallel()
	router := newRouter(newTestClient(t, nil).Client())

	response := get(router, "/other")

	require.Equal(t, http.StatusNotFound, response.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	assert.Contains(t, body["error"], "GET /configs")
}

func TestSettingsAreTheDefaultsWhenTheEnvironmentIsEmpty(t *testing.T) {
	clearEnvironment(t)

	settings, err := settingsFromEnvironment()

	require.NoError(t, err)
	assert.Equal(t, environmentSettings{
		options: configdirector.Options{
			SDKKey:   "fake-sample-key",
			Metadata: configdirector.Metadata{AppName: "gin-sample", AppVersion: "1.0.0"},
		},
		logLevel: slog.LevelInfo,
	}, settings)
}

func TestSettingsReadTheEnvironment(t *testing.T) {
	clearEnvironment(t)
	t.Setenv("CONFIGDIRECTOR_SERVER_KEY", "sk-live")
	t.Setenv("CONFIGDIRECTOR_MODE", "polling")
	t.Setenv("CONFIGDIRECTOR_TIMEOUT", "1500ms")
	t.Setenv("CONFIGDIRECTOR_BASE_URL", "https://proxy.internal/configdirector")
	t.Setenv("CONFIGDIRECTOR_LOG_LEVEL", "DEBUG")

	settings, err := settingsFromEnvironment()

	require.NoError(t, err)
	assert.Equal(t, environmentSettings{
		options: configdirector.Options{
			SDKKey:   "sk-live",
			Metadata: configdirector.Metadata{AppName: "gin-sample", AppVersion: "1.0.0"},
			Connection: configdirector.ConnectionOptions{
				Mode:    configdirector.ConnectionModePolling,
				Timeout: 1500 * time.Millisecond,
				URL:     "https://proxy.internal/configdirector",
			},
		},
		logLevel: slog.LevelDebug,
	}, settings)
}

func TestATimeoutThatIsNotADurationIsRejected(t *testing.T) {
	clearEnvironment(t)
	t.Setenv("CONFIGDIRECTOR_TIMEOUT", "soon")

	_, err := settingsFromEnvironment()

	assert.ErrorContains(t, err, "CONFIGDIRECTOR_TIMEOUT")
}

func TestALogLevelThatIsNotALevelIsRejected(t *testing.T) {
	clearEnvironment(t)
	t.Setenv("CONFIGDIRECTOR_LOG_LEVEL", "LOUD")

	_, err := settingsFromEnvironment()

	assert.ErrorContains(t, err, "CONFIGDIRECTOR_LOG_LEVEL")
}

func newTestClient(t *testing.T, values map[string]any, options ...configdirectortest.Option) *configdirectortest.TestClient {
	t.Helper()
	testClient := configdirectortest.NewTestClient(values, options...)
	t.Cleanup(func() { require.NoError(t, testClient.Close()) })
	return testClient
}

func evaluatedToTheDefaultFor(user configdirector.UserContext, key string, defaultValue any) configdirector.Evaluation {
	return configdirector.Evaluation{
		Key:         key,
		Value:       defaultValue,
		UsedDefault: true,
		Reason:      configdirector.EvaluationReasonConfigStateMissing,
		UserContext: user,
	}
}

func get(router http.Handler, target string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
	return response
}

func clearEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"CONFIGDIRECTOR_SERVER_KEY",
		"CONFIGDIRECTOR_MODE",
		"CONFIGDIRECTOR_TIMEOUT",
		"CONFIGDIRECTOR_BASE_URL",
		"CONFIGDIRECTOR_LOG_LEVEL",
	} {
		t.Setenv(name, "")
	}
}
