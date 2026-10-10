package configdirector_test

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/stretchr/testify/require"
)

func TestAnIntTraitSatisfiesANumberCondition(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, boolConfigTrueWhen("discount", traitCondition("/score", ">", "number", "80")))

	discount := client.BoolValue("discount", false, configdirector.UserContext{ID: "u-1", Traits: map[string]any{"score": 92}})

	require.True(t, discount)
}

func TestAnInt64TraitSatisfiesATextConditionOnItsDecimalText(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, boolConfigTrueWhen("discount", traitCondition("/score", "=", "text", "92")))

	discount := client.BoolValue("discount", false, configdirector.UserContext{ID: "u-1", Traits: map[string]any{"score": int64(92)}})

	require.True(t, discount)
}

func TestAFloat32TraitSatisfiesANumberConditionOnItsShortestDecimal(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, boolConfigTrueWhen("discount", traitCondition("/ratio", "=", "number", "0.1")))

	discount := client.BoolValue("discount", false, configdirector.UserContext{ID: "u-1", Traits: map[string]any{"ratio": float32(0.1)}})

	require.True(t, discount)
}

func TestAJSONNumberTraitSatisfiesANumberCondition(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, boolConfigTrueWhen("discount", traitCondition("/score", ">", "number", "80")))

	discount := client.BoolValue("discount", false, configdirector.UserContext{ID: "u-1", Traits: map[string]any{"score": json.Number("92")}})

	require.True(t, discount)
}

func TestAStringSliceTraitSatisfiesContainsAnyOf(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, boolConfigTrueWhen("discount", traitCondition("/tags", "contains any of", "array", "beta")))

	discount := client.BoolValue("discount", false, configdirector.UserContext{ID: "u-1", Traits: map[string]any{"tags": []string{"alpha", "beta"}}})

	require.True(t, discount)
}

func TestAPointerIntoAStringMapTraitFindsTheNestedValue(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, boolConfigTrueWhen("discount", traitEquals("/plan/tier", "gold")))

	discount := client.BoolValue("discount", false, configdirector.UserContext{ID: "u-1", Traits: map[string]any{"plan": map[string]string{"tier": "gold"}}})

	require.True(t, discount)
}

func TestAPointerIntoAStructTraitFindsTheFieldByItsJSONName(t *testing.T) {
	t.Parallel()
	type plan struct {
		Tier string `json:"tier"`
	}
	client := newReadyClientServing(t, boolConfigTrueWhen("discount", traitEquals("/plan/tier", "gold")))

	discount := client.BoolValue("discount", false, configdirector.UserContext{ID: "u-1", Traits: map[string]any{"plan": plan{Tier: "gold"}}})

	require.True(t, discount)
}

func TestATimeTraitSatisfiesADatetimeCondition(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, boolConfigTrueWhen("discount", traitCondition("/signedUpAt", "is after", "datetime", "2026-01-01T00:00:00Z")))
	signedUpAt := time.Date(2026, time.March, 1, 12, 30, 0, 0, time.UTC)

	discount := client.BoolValue("discount", false, configdirector.UserContext{ID: "u-1", Traits: map[string]any{"signedUpAt": signedUpAt}})

	require.True(t, discount)
}

func TestAChannelTraitMatchesNoConditionWhileTheOtherTraitsStillMatch(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t,
		boolConfigTrueWhen("stream-open", traitEquals("/stream", "open")),
		boolConfigTrueWhen("discount", traitEquals("/plan", "pro")),
	)
	user := configdirector.UserContext{ID: "u-1", Traits: map[string]any{"stream": make(chan string), "plan": "pro"}}

	streamOpen := client.BoolValue("stream-open", false, user)
	discount := client.BoolValue("discount", false, user)

	require.False(t, streamOpen)
	require.True(t, discount)
}

func TestASelfReferencingMapTraitMatchesNoConditionWhileTheOtherTraitsStillMatch(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t,
		boolConfigTrueWhen("stream-open", traitEquals("/stream/stream/stream", "open")),
		boolConfigTrueWhen("discount", traitEquals("/plan", "pro")),
	)
	stream := map[string]any{"open": "yes"}
	stream["stream"] = stream
	user := configdirector.UserContext{ID: "u-1", Traits: map[string]any{"stream": stream, "plan": "pro"}}

	streamOpen := client.BoolValue("stream-open", false, user)
	discount := client.BoolValue("discount", false, user)

	require.False(t, streamOpen)
	require.True(t, discount)
}

func TestANaNTraitMatchesNoTextCondition(t *testing.T) {
	t.Parallel()
	client := newReadyClientServing(t, boolConfigTrueWhen("discount", traitEquals("/score", "NaN")))

	discount := client.BoolValue("discount", false, configdirector.UserContext{ID: "u-1", Traits: map[string]any{"score": math.NaN()}})

	require.False(t, discount)
}
