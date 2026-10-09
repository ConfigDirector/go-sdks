package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigsReturnsTheFiveConfigs(t *testing.T) {
	t.Parallel()
	router := newRouter(readyClient(t, configdirector.Hooks{}))

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

func TestTheQueryStringBecomesTheUserContextOfEveryEvaluation(t *testing.T) {
	t.Parallel()
	evaluationsByKey := map[string]configdirector.Evaluation{}
	recordEvaluation := func(evaluation configdirector.Evaluation) {
		evaluationsByKey[evaluation.Key] = evaluation
	}
	router := newRouter(readyClient(t, configdirector.Hooks{OnConfigEvaluated: recordEvaluation}))

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
	router := newRouter(readyClient(t, configdirector.Hooks{}))

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

func readyClient(t *testing.T, hooks configdirector.Hooks) *configdirector.Client {
	t.Helper()
	client, err := configdirector.NewClient(configdirector.Options{SDKKey: "test-key", Hooks: hooks})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, client.WaitForReady(ctx))
	return client
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
