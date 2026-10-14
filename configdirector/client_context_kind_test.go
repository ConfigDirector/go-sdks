package configdirector_test

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/stretchr/testify/require"
)

func proUserWithContextKind(kind string) configdirector.UserContext {
	return configdirector.UserContext{ID: "u-pro", Kind: kind, Traits: map[string]any{"plan": "pro"}}
}

func TestAKindOf64CharactersIsAccepted(t *testing.T) {
	t.Parallel()
	kind := strings.Repeat("k", 64)
	client := newReadyClientServing(t, boolConfigTrueWhen("kind-banner", contextKindIsOneOf(kind)))

	require.True(t, client.BoolValue("kind-banner", false, configdirector.UserContext{ID: "42", Kind: kind}))
}

func TestAKindOf65CharactersIgnoresTheContext(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	greeting, details := client.StringValueDetails("greeting", "fallback", proUserWithContextKind(strings.Repeat("k", 65)))

	require.Equal(t, "Hello", greeting)
	require.Equal(t, foundMatch("value-greeting-default"), details)
}

func TestAnUppercaseKindIgnoresTheContext(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	greeting, details := client.StringValueDetails("greeting", "fallback", proUserWithContextKind("Device"))

	require.Equal(t, "Hello", greeting)
	require.Equal(t, foundMatch("value-greeting-default"), details)
}

func TestAKindWithSurroundingSpaceIgnoresTheContext(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	greeting, details := client.StringValueDetails("greeting", "fallback", proUserWithContextKind(" device "))

	require.Equal(t, "Hello", greeting)
	require.Equal(t, foundMatch("value-greeting-default"), details)
}

func TestAnInvalidContextKindOnAJSONReadIgnoresTheContext(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, configOf("limits", "json", `{"max":20}`, valueRule("limits-pro", 0, `{"max":99}`, identifierEquals("u-pro"))))
	target := limits{Max: 5}

	require.NoError(t, client.JSONValue("limits", &target, proUserWithContextKind("Device")))

	require.Equal(t, limits{Max: 20}, target)
}

func TestAnInvalidContextKindLogsAnErrorNamingTheKind(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	client, records := newLoggedClient(t, server)
	waitForReady(t, client)

	client.StringValue("greeting", "fallback", proUserWithContextKind("Device"))

	errors := recordsAt(slog.LevelError, records())
	require.Len(t, errors, 1)
	require.Equal(t, "Device", attributesOf(errors[0])["contextKind"])
}

func TestAnInvalidContextKindReachesOnConfigEvaluatedWithNoContext(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)
	evaluations := newRecorder[configdirector.Evaluation]()
	client.OnConfigEvaluated(evaluations.record)

	client.StringValue("greeting", "fallback", proUserWithContextKind("Device"))

	evaluated := evaluations.next(t)
	require.Equal(t, configdirector.UserContext{}, evaluated.UserContext)
	require.Equal(t, "Hello", evaluated.Value)
}

func TestAllConfigsIgnoresAContextWithAnInvalidContextKind(t *testing.T) {
	t.Parallel()
	client := newReadyClient(t)

	states := client.AllConfigs(proUserWithContextKind("Device"), "greeting")

	require.Len(t, states, 1)
	require.Equal(t, "Hello", states[0].Value)
}

func TestAWatchWithAnInvalidContextKindFiresWithTheNoContextValue(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())
	fired := newRecorder[string]()
	client.WatchString("greeting", "fallback", proUserWithContextKind("Device"), fired.record)

	server.send(deltaPayload(configOf("greeting", "string", "Hello", valueRule("greeting-pro", 0, "Hello, pro", identifierEquals("u-pro")))).json())

	require.Equal(t, "Hello", fired.next(t))
}

func TestTheSameInvalidContextKindIsLoggedOncePerFlushInterval(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	logger, records := newLogger()
	options := server.options()
	options.Telemetry.FlushInterval = 300 * time.Millisecond
	options.Logger = logger
	client := newClient(t, options)
	waitForReady(t, client)

	client.StringValue("greeting", "fallback", proUserWithContextKind("Device"))
	client.StringValue("greeting", "fallback", proUserWithContextKind("Device"))
	first := server.telemetryReportWithin(t, firstScheduledFlushTimeout)
	client.StringValue("greeting", "fallback", proUserWithContextKind("Device"))
	second := server.nextTelemetryReport(t)

	require.Equal(t, 2, first.AggregatedEvents.InvalidContext[0].Count)
	require.Equal(t, 1, second.AggregatedEvents.InvalidContext[0].Count)
	require.Len(t, recordsAt(slog.LevelError, records()), 2)
}

func TestTheTwentyFirstDistinctInvalidContextKindLogsThatFurtherKindsAreNotLoggedIndividually(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	client, records := newLoggedClient(t, server)
	waitForReady(t, client)

	for index := range 22 {
		client.BoolValue("new-checkout", false, configdirector.UserContext{ID: "42", Kind: "Kind-" + string(rune('A'+index))})
	}

	errors := recordsAt(slog.LevelError, records())
	require.Len(t, errors, 21)
	require.Equal(t, "Kind-T", attributesOf(errors[19])["contextKind"])
	require.NotContains(t, attributesOf(errors[20]), "contextKind")
}
