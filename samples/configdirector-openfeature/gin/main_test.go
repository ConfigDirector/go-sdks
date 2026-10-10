package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/open-feature/go-sdk/openfeature"
	"github.com/open-feature/go-sdk/openfeature/memprovider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var servedFlags = map[string]memprovider.InMemoryFlag{
	"temporary-feature-flag": enabledFlag("temporary-feature-flag", "off", map[string]any{"on": true, "off": false}),
	"permanent-kill-switch":  enabledFlag("permanent-kill-switch", "on", map[string]any{"on": true, "off": false}),
	"integer-config":         enabledFlag("integer-config", "twenty", map[string]any{"twenty": int64(20)}),
	"day-of-the-week-config": enabledFlag("day-of-the-week-config", "monday", map[string]any{"monday": "Monday"}),
	"json-value-config":      enabledFlag("json-value-config", "blue", map[string]any{"blue": map[string]any{"color": "blue"}}),
}

func TestConfigsReturnsTheFiveConfigsAsServed(t *testing.T) {
	router := routerReading(t, servedFlags)

	response := get(t, router, "/configs?id=user-123&plan=pro")

	require.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `{
		"temporary-feature-flag": false,
		"permanent-kill-switch": true,
		"integer-config": 20,
		"day-of-the-week-config": "Monday",
		"json-value-config": {"color": "blue"}
	}`, response.Body.String())
}

func TestConfigsReturnsTheInCodeDefaultValuesWhenNoFlagsAreServed(t *testing.T) {
	router := routerReading(t, map[string]memprovider.InMemoryFlag{})

	response := get(t, router, "/configs?id=user-123&plan=pro")

	require.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `{
		"temporary-feature-flag": true,
		"permanent-kill-switch": false,
		"integer-config": 10,
		"day-of-the-week-config": "Friday",
		"json-value-config": {}
	}`, response.Body.String())
}

func TestTheDetailsOfAFlagReportItsValueVariantAndReason(t *testing.T) {
	router := routerReading(t, servedFlags)

	response := get(t, router, "/configs/permanent-kill-switch?id=user-123")

	require.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `{
		"value": true,
		"variant": "on",
		"reason": "STATIC",
		"errorCode": "",
		"errorMessage": ""
	}`, response.Body.String())
}

func TestTheDetailsOfAnUnknownFlagExplainWhyTheDefaultWasServed(t *testing.T) {
	router := routerReading(t, servedFlags)

	response := get(t, router, "/configs/no-such-flag?id=user-123")

	require.Equal(t, http.StatusOK, response.Code)
	var details map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &details))
	assert.Equal(t, false, details["value"])
	assert.Equal(t, "ERROR", details["reason"])
	assert.Equal(t, "FLAG_NOT_FOUND", details["errorCode"])
}

func TestTheQueryStringBecomesTheEvaluationContextOfEveryEvaluation(t *testing.T) {
	contextsByKey := map[string]openfeature.FlattenedContext{}
	recordContext := func(flag memprovider.InMemoryFlag, flattenedContext openfeature.FlattenedContext) (any, openfeature.ProviderResolutionDetail) {
		contextsByKey[flag.Key] = flattenedContext
		return flag.Variants[flag.DefaultVariant], openfeature.ProviderResolutionDetail{Reason: openfeature.StaticReason, Variant: flag.DefaultVariant}
	}
	router := routerReading(t, flagsEvaluatedThrough(&recordContext))

	get(t, router, "/configs?id=user-123&name=Ada&anonymous=true&plan=pro&region=eu")

	ada := openfeature.FlattenedContext{
		openfeature.TargetingKey: "user-123",
		"name":                   "Ada",
		"anonymous":              true,
		"traits":                 map[string]any{"plan": "pro", "region": "eu"},
	}
	assert.Equal(t, map[string]openfeature.FlattenedContext{
		"temporary-feature-flag": ada,
		"permanent-kill-switch":  ada,
		"integer-config":         ada,
		"day-of-the-week-config": ada,
		"json-value-config":      ada,
	}, contextsByKey)
}

func TestAnUnknownRouteReturnsAHintToTheConfigsRoute(t *testing.T) {
	router := routerReading(t, servedFlags)

	response := get(t, router, "/other")

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
			Metadata: configdirector.Metadata{AppName: "openfeature-gin-sample", AppVersion: "1.0.0"},
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
			Metadata: configdirector.Metadata{AppName: "openfeature-gin-sample", AppVersion: "1.0.0"},
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

func routerReading(t *testing.T, flags map[string]memprovider.InMemoryFlag) http.Handler {
	t.Helper()
	require.NoError(t, openfeature.SetProviderAndWait(memprovider.NewInMemoryProvider(flags)))
	t.Cleanup(openfeature.Shutdown)
	return newRouter(openfeature.NewClient(clientDomain))
}

func enabledFlag(key string, defaultVariant string, variants map[string]any) memprovider.InMemoryFlag {
	return memprovider.InMemoryFlag{Key: key, State: memprovider.Enabled, DefaultVariant: defaultVariant, Variants: variants}
}

func flagsEvaluatedThrough(evaluator memprovider.ContextEvaluator) map[string]memprovider.InMemoryFlag {
	flags := map[string]memprovider.InMemoryFlag{}
	for key, flag := range servedFlags {
		flag.ContextEvaluator = evaluator
		flags[key] = flag
	}
	return flags
}

func get(t *testing.T, router http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil))
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
