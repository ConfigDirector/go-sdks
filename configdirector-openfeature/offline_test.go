package configdirectoropenfeature_test

import (
	"testing"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/open-feature/go-sdk/openfeature"
	"github.com/stretchr/testify/require"
)

func TestAnOfflineProviderResolvesItsValuesWithoutAnSDKKey(t *testing.T) {
	t.Parallel()
	client := registerProvider(t, configdirector.Options{Connection: configdirector.ConnectionOptions{
		Mode:   configdirector.ConnectionModeOffline,
		Values: map[string]any{"greeting": "hello offline"},
	}})

	details, err := client.StringValueDetails(t.Context(), "greeting", "fallback", openfeature.EvaluationContext{})

	require.NoError(t, err)
	require.Equal(t, "hello offline", details.Value)
	require.Equal(t, openfeature.TargetingMatchReason, details.Reason)
}
