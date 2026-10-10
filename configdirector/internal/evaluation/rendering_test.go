package evaluation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTextOfAStringIsItself(t *testing.T) {
	text, hasText := textOf("abc")

	require.True(t, hasText)
	require.Equal(t, "abc", text)
}

func TestTextOfTrueIsItsJSONSpelling(t *testing.T) {
	text, hasText := textOf(true)

	require.True(t, hasText)
	require.Equal(t, "true", text)
}

func TestTextOfFalseIsItsJSONSpelling(t *testing.T) {
	text, hasText := textOf(false)

	require.True(t, hasText)
	require.Equal(t, "false", text)
}

func TestTextOfAWholeNumberHasNoFraction(t *testing.T) {
	text, _ := textOf(26.0)

	require.Equal(t, "26", text)
}

func TestTextOfAFractionalNumberKeepsItsFraction(t *testing.T) {
	text, _ := textOf(26.5)

	require.Equal(t, "26.5", text)
}

func TestTextOfANegativeNumberKeepsItsSign(t *testing.T) {
	text, _ := textOf(-5.0)

	require.Equal(t, "-5", text)
}

func TestTextOfALargeFractionalNumberIsNotInExponentForm(t *testing.T) {
	text, _ := textOf(1234567.5)

	require.Equal(t, "1234567.5", text)
}

func TestTextOfALargeWholeNumberIsSpelledOut(t *testing.T) {
	text, _ := textOf(1e20)

	require.Equal(t, "100000000000000000000", text)
}

func TestTextOfAnAbsentValueIsEmptyAndHasNoText(t *testing.T) {
	text, hasText := textOf(nil)

	require.False(t, hasText)
	require.Empty(t, text)
}

func TestTextOfAnArrayIsEmptyAndHasNoText(t *testing.T) {
	text, hasText := textOf([]any{"a"})

	require.False(t, hasText)
	require.Empty(t, text)
}

func TestTextOfAnObjectIsEmptyAndHasNoText(t *testing.T) {
	text, hasText := textOf(map[string]any{"a": "b"})

	require.False(t, hasText)
	require.Empty(t, text)
}
