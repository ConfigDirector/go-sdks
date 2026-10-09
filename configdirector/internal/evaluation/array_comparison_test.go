package evaluation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestArrayContainsAnyTarget(t *testing.T) {
	require.True(t, arrayMatches([]any{"red", "blue"}, "contains any of", []string{"yellow", "blue"}))
	require.False(t, arrayMatches([]any{"red", "blue"}, "contains any of", []string{"yellow"}))
}

func TestArrayDoesNotContainAnyTarget(t *testing.T) {
	require.True(t, arrayMatches([]any{"red", "blue"}, "does not contain any of", []string{"yellow"}))
	require.False(t, arrayMatches([]any{"red", "blue"}, "does not contain any of", []string{"yellow", "blue"}))
}

func TestArrayElementsAreRenderedBeforeComparing(t *testing.T) {
	require.True(t, arrayMatches([]any{1.0, 2.0}, "contains any of", []string{"1"}))
	require.True(t, arrayMatches([]any{true}, "contains any of", []string{"true"}))
}

func TestArrayElementsWithoutATextFormAreDropped(t *testing.T) {
	require.False(t, arrayMatches([]any{nil, []any{}, map[string]any{}}, "contains any of", []string{""}))
	require.True(t, arrayMatches([]any{nil}, "does not contain any of", []string{""}))
}

func TestArrayContainsAnyOfNoTargetsDoesNotMatch(t *testing.T) {
	require.False(t, arrayMatches([]any{"red"}, "contains any of", nil))
}

func TestArrayDoesNotContainAnyOfNoTargetsMatches(t *testing.T) {
	require.True(t, arrayMatches([]any{"red"}, "does not contain any of", nil))
}

func TestAScalarValueContainsNothing(t *testing.T) {
	require.False(t, arrayMatches("red", "contains any of", []string{"red"}))
	require.True(t, arrayMatches("red", "does not contain any of", []string{"red"}))
}

func TestAnAbsentValueContainsNothing(t *testing.T) {
	require.False(t, arrayMatches(nil, "contains any of", []string{""}))
	require.True(t, arrayMatches(nil, "does not contain any of", []string{""}))
}

func TestArrayWithAnUnknownOperatorDoesNotMatch(t *testing.T) {
	require.False(t, arrayMatches([]any{"red"}, "is one of", []string{"red"}))
}
