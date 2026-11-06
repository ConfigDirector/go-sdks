package configdirectoropenfeature

import (
	"strings"
	"testing"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/ConfigDirector/go-sdks/configdirector/configdirectortest"
	"github.com/open-feature/go-sdk/openfeature"
	"github.com/stretchr/testify/require"
)

func TestTrackReachesTheSDKWithTheValueDetailsAndContext(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{})
	user := openfeature.NewEvaluationContext("user-123", map[string]any{"name": "Ada", "traits": map[string]any{"plan": "pro"}})

	fixture.client.Track(t.Context(), "checkout-completed", user, openfeature.NewTrackingEventDetails(49.9).Add("plan", "pro").Add("seats", 3))

	require.Equal(t, []configdirectortest.TrackedEvent{{
		Name:        "checkout-completed",
		UserContext: configdirector.UserContext{ID: "user-123", Name: "Ada", Traits: map[string]any{"plan": "pro"}},
		Value:       49.9,
		Details:     map[string]any{"plan": "pro", "seats": 3},
	}}, fixture.testClient.TrackedEvents())
}

func TestAValueOfZeroIsTrackedAsNoValueBecauseOpenFeatureCannotSayThereIsNone(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{})

	fixture.client.Track(t.Context(), "plan-chosen", noContext, openfeature.NewTrackingEventDetails(0).Add("plan", "pro"))

	require.Equal(t, []configdirectortest.TrackedEvent{{Name: "plan-chosen", Details: map[string]any{"plan": "pro"}}}, fixture.testClient.TrackedEvents())
}

func TestEmptyTrackingDetailsAreTrackedWithoutAValueOrDetails(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{})

	fixture.client.Track(t.Context(), "nightly-sync-finished", noContext, openfeature.TrackingEventDetails{})

	require.Equal(t, []configdirectortest.TrackedEvent{{Name: "nightly-sync-finished"}}, fixture.testClient.TrackedEvents())
}

func TestAValueWithoutAttributesIsTrackedWithoutDetails(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{})

	fixture.client.Track(t.Context(), "checkout-completed", noContext, openfeature.NewTrackingEventDetails(49.9))

	require.Equal(t, []configdirectortest.TrackedEvent{{Name: "checkout-completed", Value: 49.9}}, fixture.testClient.TrackedEvents())
}

func TestAnAttributeNamedValueStaysInTheDetails(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{})

	fixture.client.Track(t.Context(), "checkout-completed", noContext, openfeature.NewTrackingEventDetails(49.9).Add("value", "gold"))

	require.Equal(t, []configdirectortest.TrackedEvent{{Name: "checkout-completed", Value: 49.9, Details: map[string]any{"value": "gold"}}}, fixture.testClient.TrackedEvents())
}

func TestTrackMapsTheEvaluationContextAsAnEvaluationDoes(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{})
	device := openfeature.NewTargetlessEvaluationContext(map[string]any{"id": "device-42", "kind": "device", "anonymous": true})

	fixture.client.Track(t.Context(), "kiosk-opened", device, openfeature.TrackingEventDetails{})

	require.Equal(t, []configdirectortest.TrackedEvent{{
		Name:        "kiosk-opened",
		UserContext: configdirector.UserContext{ID: "device-42", Kind: "device", Anonymous: true},
	}}, fixture.testClient.TrackedEvents())
}

func TestTrackUsesTheEvaluationContextOpenFeatureMerged(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{})
	fixture.client.SetEvaluationContext(openfeature.NewEvaluationContext("user-from-the-client", map[string]any{"name": "Ada"}))

	fixture.client.Track(t.Context(), "checkout-completed", openfeature.NewTargetlessEvaluationContext(map[string]any{"kind": "member"}), openfeature.TrackingEventDetails{})

	require.Equal(t, []configdirectortest.TrackedEvent{{
		Name:        "checkout-completed",
		UserContext: configdirector.UserContext{ID: "user-from-the-client", Kind: "member", Name: "Ada"},
	}}, fixture.testClient.TrackedEvents())
}

func TestAnEventForAnInvalidKindIsTrackedWithoutAContext(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{})

	fixture.client.Track(t.Context(), "checkout-completed", openfeature.NewEvaluationContext("user-123", map[string]any{"kind": "Device"}), openfeature.TrackingEventDetails{})

	require.Equal(t, []configdirectortest.TrackedEvent{{Name: "checkout-completed"}}, fixture.testClient.TrackedEvents())
}

func TestAnEventWhoseNameTheSDKWouldNotReportIsNotTracked(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{})

	fixture.client.Track(t.Context(), strings.Repeat("n", 151), noContext, openfeature.TrackingEventDetails{})

	require.Empty(t, fixture.testClient.TrackedEvents())
}

func TestAnEventTrackedAfterShutdownIsNotReported(t *testing.T) {
	t.Parallel()
	fixture := registeredProviderServing(t, map[string]any{})
	fixture.provider.Shutdown()

	fixture.client.Track(t.Context(), "checkout-completed", noContext, openfeature.TrackingEventDetails{})

	require.Empty(t, fixture.testClient.TrackedEvents())
}
