package evaluation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNumberEqualsTheFirstTarget(t *testing.T) {
	require.True(t, numberMatches(26.0, "=", []string{"26", "99"}))
	require.False(t, numberMatches(26.0, "equals", []string{"99", "26"}))
}

func TestNumberDoesNotEqualTheFirstTarget(t *testing.T) {
	require.True(t, numberMatches(26.0, "!=", []string{"25"}))
	require.False(t, numberMatches(26.0, "does not equal", []string{"26"}))
}

func TestNumberIsLessThanTheTarget(t *testing.T) {
	require.True(t, numberMatches(25.0, "<", []string{"26"}))
	require.False(t, numberMatches(26.0, "<", []string{"26"}))
}

func TestNumberIsAtMostTheTarget(t *testing.T) {
	require.True(t, numberMatches(26.0, "<=", []string{"26"}))
	require.False(t, numberMatches(27.0, "<=", []string{"26"}))
}

func TestNumberIsGreaterThanTheTarget(t *testing.T) {
	require.True(t, numberMatches(27.0, ">", []string{"26"}))
	require.False(t, numberMatches(26.0, ">", []string{"26"}))
}

func TestNumberIsAtLeastTheTarget(t *testing.T) {
	require.True(t, numberMatches(26.0, ">=", []string{"26"}))
	require.False(t, numberMatches(25.0, ">=", []string{"26"}))
}

func TestNumberInAStringIsParsedStrictly(t *testing.T) {
	require.True(t, numberMatches("10.5", ">", []string{"10"}))
	require.True(t, numberMatches("1e3", "=", []string{"1000"}))
	require.True(t, numberMatches("-5", "<", []string{"0"}))
}

func TestNumberWithTrailingCharactersIsUnparseable(t *testing.T) {
	require.False(t, numberMatches("26abc", "=", []string{"26"}))
	require.True(t, numberMatches("26abc", "!=", []string{"26"}))
}

func TestNumberWithSurroundingWhitespaceIsUnparseable(t *testing.T) {
	require.False(t, numberMatches(" 42 ", "=", []string{"42"}))
}

func TestNumberWithDigitSeparatorsIsUnparseable(t *testing.T) {
	require.False(t, numberMatches("1_000", "=", []string{"1000"}))
}

func TestNumberSpelledAsInfinityIsUnparseable(t *testing.T) {
	require.False(t, numberMatches("Infinity", ">", []string{"0"}))
	require.False(t, numberMatches("inf", ">", []string{"0"}))
}

func TestNumberSpelledAsNaNIsUnparseable(t *testing.T) {
	require.False(t, numberMatches("NaN", "=", []string{"0"}))
	require.True(t, numberMatches("NaN", "!=", []string{"0"}))
}

func TestNumberBeyondTheFloatRangeIsUnparseable(t *testing.T) {
	require.False(t, numberMatches("1e400", ">", []string{"0"}))
}

func TestNumberThatIsEmptyIsUnparseable(t *testing.T) {
	require.False(t, numberMatches("", "=", []string{""}))
	require.True(t, numberMatches("", "does not equal", []string{""}))
}

func TestNumberThatIsAbsentIsUnparseable(t *testing.T) {
	require.False(t, numberMatches(nil, "=", []string{"0"}))
	require.True(t, numberMatches(nil, "!=", []string{"0"}))
}

func TestNumberThatIsABooleanIsUnparseable(t *testing.T) {
	require.False(t, numberMatches(true, "=", []string{"1"}))
	require.True(t, numberMatches(true, "!=", []string{"1"}))
}

func TestNumberThatIsAnArrayIsUnparseable(t *testing.T) {
	require.False(t, numberMatches([]any{1.0}, "=", []string{"1"}))
}

func TestNumberWithNoTargetsDoesNotMatch(t *testing.T) {
	require.False(t, numberMatches(26.0, "=", nil))
	require.False(t, numberMatches(26.0, "!=", nil))
}

func TestNumberWithAnUnparseableTargetDoesNotMatch(t *testing.T) {
	require.False(t, numberMatches(26.0, "=", []string{"abc"}))
	require.False(t, numberMatches(26.0, "!=", []string{"abc"}))
}

func TestNumberWithAnUnknownOperatorDoesNotMatch(t *testing.T) {
	require.False(t, numberMatches(26.0, "is one of", []string{"26"}))
}
