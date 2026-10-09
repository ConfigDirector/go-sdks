package evaluation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSemverEqualsTheFirstTarget(t *testing.T) {
	require.True(t, semverMatches("1.0.0", "=", []string{"1.0.0", "2.0.0"}))
	require.False(t, semverMatches("1.0.0", "=", []string{"2.0.0", "1.0.0"}))
}

func TestSemverCoercesAMissingPatchAndMinor(t *testing.T) {
	require.True(t, semverMatches("1.0", "=", []string{"1.0.0"}))
	require.True(t, semverMatches("1", "=", []string{"1.0.0"}))
}

func TestSemverCoercesALeadingPrefix(t *testing.T) {
	require.True(t, semverMatches("v2.3.4", "=", []string{"2.3.4"}))
}

func TestSemverDropsAFourthComponent(t *testing.T) {
	require.True(t, semverMatches("1.2.3.4", "=", []string{"1.2.3"}))
}

func TestSemverDropsAPrereleaseSuffix(t *testing.T) {
	require.True(t, semverMatches("0.1.645-a", "=", []string{"0.1.645"}))
}

func TestSemverComparesComponentsNumerically(t *testing.T) {
	require.True(t, semverMatches("10.0.1", ">", []string{"9.0.0"}))
	require.True(t, semverMatches("1.0.0", "<", []string{"10.0.1"}))
}

func TestSemverComparesTheMinorComponentWhenMajorsAreEqual(t *testing.T) {
	require.True(t, semverMatches("1.2.0", ">", []string{"1.1.9"}))
	require.False(t, semverMatches("1.1.9", ">", []string{"1.2.0"}))
}

func TestSemverComparesThePatchComponentWhenMajorsAndMinorsAreEqual(t *testing.T) {
	require.True(t, semverMatches("1.1.9", ">", []string{"1.1.8"}))
	require.False(t, semverMatches("1.1.8", ">", []string{"1.1.9"}))
}

func TestSemverIsAtMostTheTarget(t *testing.T) {
	require.True(t, semverMatches("1.0.0", "<=", []string{"1.0.0"}))
	require.False(t, semverMatches("1.0.1", "<=", []string{"1.0.0"}))
}

func TestSemverIsAtLeastTheTarget(t *testing.T) {
	require.True(t, semverMatches("1.0.0", ">=", []string{"1.0.0"}))
	require.False(t, semverMatches("0.9.9", ">=", []string{"1.0.0"}))
}

func TestSemverIsOneOfTheTargets(t *testing.T) {
	require.True(t, semverMatches("1.0", "is one of", []string{"2.0.0", "1.0.0"}))
	require.False(t, semverMatches("1.0", "is one of", []string{"2.0.0"}))
}

func TestSemverIsNotOneOfTheTargets(t *testing.T) {
	require.True(t, semverMatches("1.0", "is not one of", []string{"2.0.0"}))
	require.False(t, semverMatches("1.0", "is not one of", []string{"2.0.0", "1.0.0"}))
}

func TestSemverIsNotOneOfNoTargetsMatches(t *testing.T) {
	require.True(t, semverMatches("1.0", "is not one of", nil))
}

func TestSemverWithNoTargetsDoesNotMatchOtherOperators(t *testing.T) {
	require.False(t, semverMatches("1.0", "=", nil))
	require.False(t, semverMatches("1.0", ">=", nil))
	require.False(t, semverMatches("1.0", "is one of", nil))
}

func TestSemverThatIsBlankMatchesOnlyIsNotOneOf(t *testing.T) {
	require.True(t, semverMatches("   ", "is not one of", []string{"1.0.0"}))
	require.False(t, semverMatches("", "=", []string{"1.0.0"}))
	require.False(t, semverMatches("", "<", []string{"1.0.0"}))
}

func TestSemverThatCannotBeCoercedNeverEqualsATarget(t *testing.T) {
	require.False(t, semverMatches("abc", "=", []string{"abc"}))
	require.False(t, semverMatches("abc", "is one of", []string{"abc"}))
}

func TestSemverThatCannotBeCoercedNeverSatisfiesAnOrdering(t *testing.T) {
	require.False(t, semverMatches("abc", "<", []string{"1.0.0"}))
}

func TestSemverThatCannotBeCoercedIsNotOneOfAnything(t *testing.T) {
	require.True(t, semverMatches("abc", "is not one of", []string{"abc"}))
}

func TestSemverWithAnUncoercibleTargetNeverCompares(t *testing.T) {
	require.False(t, semverMatches("1.0.0", ">", []string{"abc"}))
	require.False(t, semverMatches("1.0.0", "=", []string{"abc"}))
}

func TestSemverCapsEachComponentAtSixteenDigits(t *testing.T) {
	require.False(t, semverMatches("12345678901234567.0.0", ">", []string{"1.0.0"}))
	require.True(t, semverMatches("1234567890123456.0.0", ">", []string{"1.0.0"}))
}

func TestSemverWithAnUnknownOperatorDoesNotMatch(t *testing.T) {
	require.False(t, semverMatches("1.0.0", "!=", []string{"2.0.0"}))
}
