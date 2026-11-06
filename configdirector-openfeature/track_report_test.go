package configdirectoropenfeature_test

import (
	"encoding/json"
	"testing"

	configdirectoropenfeature "github.com/ConfigDirector/go-sdks/configdirector-openfeature"
	"github.com/open-feature/go-sdk/openfeature"
	"github.com/open-feature/go-sdk/openfeature/isolated"
	"github.com/stretchr/testify/require"
)

func TestAnEventTrackedThroughOpenFeatureArrivesInTheTelemetryReportTheProviderSends(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t)
	provider, err := configdirectoropenfeature.NewProvider(server.options())
	require.NoError(t, err)
	api := isolated.NewAPI()
	require.NoError(t, api.SetProviderAndWait(t.Context(), provider))

	api.NewClient().Track(t.Context(), "checkout-completed", openfeature.NewEvaluationContext("user-123", nil), openfeature.NewTrackingEventDetails(49.9).Add("plan", "<pro>"))
	provider.Shutdown()

	var report struct {
		MetaContext    map[string]string `json:"metaContext"`
		DiscreteEvents struct {
			TrackedEvents []struct {
				Name              string          `json:"name"`
				ContextKind       string          `json:"contextKind"`
				ContextIdentifier string          `json:"contextIdentifier"`
				Value             json.RawMessage `json:"value"`
				Details           json.RawMessage `json:"details"`
			} `json:"trackedEvents"`
		} `json:"discreteEvents"`
	}
	require.NoError(t, json.Unmarshal(await(t, server.telemetryReports), &report))
	require.Equal(t, "go-openfeature-server-provider", report.MetaContext["sdkName"])
	require.Len(t, report.DiscreteEvents.TrackedEvents, 1)
	entry := report.DiscreteEvents.TrackedEvents[0]
	require.Equal(t, "checkout-completed", entry.Name)
	require.Equal(t, "user", entry.ContextKind)
	require.Equal(t, "user-123", entry.ContextIdentifier)
	require.Equal(t, `49.9`, string(entry.Value))
	require.Equal(t, `{"plan":"<pro>"}`, string(entry.Details))
}
