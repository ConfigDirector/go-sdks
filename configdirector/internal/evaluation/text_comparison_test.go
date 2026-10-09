package evaluation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTextEqualsTheFirstTarget(t *testing.T) {
	require.True(t, textMatches("abc", "equals", []string{"abc", "xyz"}))
	require.False(t, textMatches("abc", "=", []string{"xyz", "abc"}))
}

func TestTextEqualsIsCaseSensitive(t *testing.T) {
	require.False(t, textMatches("abc", "equals", []string{"ABC"}))
}

func TestTextEqualsWithNoTargetsDoesNotMatch(t *testing.T) {
	require.False(t, textMatches("", "equals", nil))
}

func TestTextDoesNotEqualTheFirstTarget(t *testing.T) {
	require.True(t, textMatches("abc", "does not equal", []string{"xyz"}))
	require.False(t, textMatches("abc", "!=", []string{"abc"}))
}

func TestTextDoesNotEqualWithNoTargetsDoesNotMatch(t *testing.T) {
	require.False(t, textMatches("abc", "does not equal", nil))
}

func TestTextIsOneOfTheTargets(t *testing.T) {
	require.True(t, textMatches("abc", "is one of", []string{"xyz", "abc"}))
	require.False(t, textMatches("abc", "is one of", []string{"xyz"}))
}

func TestTextIsOneOfNoTargetsDoesNotMatch(t *testing.T) {
	require.False(t, textMatches("abc", "is one of", nil))
}

func TestTextIsNotOneOfTheTargets(t *testing.T) {
	require.True(t, textMatches("abc", "is not one of", []string{"xyz"}))
	require.False(t, textMatches("abc", "is not one of", []string{"xyz", "abc"}))
}

func TestTextIsNotOneOfNoTargetsMatches(t *testing.T) {
	require.True(t, textMatches("abc", "is not one of", nil))
}

func TestTextStartsWithAnyTarget(t *testing.T) {
	require.True(t, textMatches("abc", "starts with any of", []string{"x", "ab"}))
	require.False(t, textMatches("abc", "starts with any of", []string{"bc"}))
}

func TestTextStartsWithAnyOfNoTargetsDoesNotMatch(t *testing.T) {
	require.False(t, textMatches("abc", "starts with any of", nil))
}

func TestTextDoesNotStartWithAnyTarget(t *testing.T) {
	require.True(t, textMatches("abc", "does not start with any of", []string{"bc"}))
	require.False(t, textMatches("abc", "does not start with any of", []string{"x", "ab"}))
}

func TestTextDoesNotStartWithAnyOfNoTargetsMatches(t *testing.T) {
	require.True(t, textMatches("abc", "does not start with any of", nil))
}

func TestTextEndsWithAnyTarget(t *testing.T) {
	require.True(t, textMatches("abc", "ends with any of", []string{"x", "bc"}))
	require.False(t, textMatches("abc", "ends with any of", []string{"ab"}))
}

func TestTextEndsWithAnyOfNoTargetsDoesNotMatch(t *testing.T) {
	require.False(t, textMatches("abc", "ends with any of", nil))
}

func TestTextDoesNotEndWithAnyTarget(t *testing.T) {
	require.True(t, textMatches("abc", "does not end with any of", []string{"ab"}))
	require.False(t, textMatches("abc", "does not end with any of", []string{"x", "bc"}))
}

func TestTextDoesNotEndWithAnyOfNoTargetsMatches(t *testing.T) {
	require.True(t, textMatches("abc", "does not end with any of", nil))
}

func TestTextWithAnUnknownOperatorDoesNotMatch(t *testing.T) {
	require.False(t, textMatches("abc", "matches regex", []string{"abc"}))
}
