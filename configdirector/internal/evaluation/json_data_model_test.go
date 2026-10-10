package evaluation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestATraitNestedDeeperThanTheWalkedDepthIsStillRead(t *testing.T) {
	var deepest any = "x"
	for range deepestNestingWalked + 10 {
		deepest = map[string]any{"n": deepest}
	}
	traits := map[string]any{"deep": deepest}
	pointer := "/deep" + strings.Repeat("/n", deepestNestingWalked+10)

	require.Equal(t, "x", traitAtPointer(TraitsInJSONDataModel(traits), pointer))
}

func TestASelfReferencingTraitIsLeftOutWhileTheOthersAreKept(t *testing.T) {
	stream := map[string]any{"open": "yes"}
	stream["stream"] = stream
	traits := map[string]any{"stream": stream, "plan": "pro"}

	require.Equal(t, map[string]any{"plan": "pro"}, TraitsInJSONDataModel(traits))
}
