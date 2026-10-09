package evaluation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const instant = "2026-01-28T01:25:00.000Z"

func TestDatetimeIsBeforeTheTarget(t *testing.T) {
	require.True(t, datetimeMatches("2026-01-28T01:24:59.999Z", "is before", []string{instant}))
	require.False(t, datetimeMatches(instant, "is before", []string{instant}))
}

func TestDatetimeIsAfterTheTarget(t *testing.T) {
	require.True(t, datetimeMatches("2026-01-28T01:25:00.001", "is after", []string{instant}))
	require.False(t, datetimeMatches(instant, "is after", []string{instant}))
}

func TestDatetimeReadsABareYearAsJanuaryFirstUTC(t *testing.T) {
	require.True(t, datetimeMatches("2026", "is before", []string{"2026-01-01T00:00:00.001Z"}))
	require.False(t, datetimeMatches("2026", "is before", []string{"2026-01-01T00:00:00Z"}))
}

func TestDatetimeReadsAYearAndMonthAsTheFirstOfTheMonth(t *testing.T) {
	require.True(t, datetimeMatches("2026-01", "is before", []string{"2026-01-01T00:00:01Z"}))
}

func TestDatetimeReadsADateAsMidnightUTC(t *testing.T) {
	require.True(t, datetimeMatches("2026-01-28", "is before", []string{instant}))
	require.True(t, datetimeMatches("2026-01-29", "is after", []string{instant}))
}

func TestDatetimeWithoutAnOffsetIsReadAsUTC(t *testing.T) {
	sameInstantAtPlusTwo := "2026-01-28T03:25:00+02:00"

	require.False(t, datetimeMatches("2026-01-28T01:25:00", "is before", []string{sameInstantAtPlusTwo}))
	require.False(t, datetimeMatches("2026-01-28T01:25:00", "is after", []string{sameInstantAtPlusTwo}))
}

func TestDatetimeWithAnOffsetIsReadAtThatOffset(t *testing.T) {
	require.True(t, datetimeMatches("2026-01-28T01:25:00+02:00", "is before", []string{instant}))
	require.True(t, datetimeMatches("2026-01-28T01:25:00-02:00", "is after", []string{instant}))
}

func TestDatetimeTruncatesBeyondMilliseconds(t *testing.T) {
	require.False(t, datetimeMatches("2026-01-28T01:24:59.999999Z", "is after", []string{"2026-01-28T01:24:59.999Z"}))
	require.False(t, datetimeMatches("2026-01-28T01:25:00.123456Z", "is after", []string{"2026-01-28T01:25:00.123Z"}))
}

func TestDatetimeWithAnImpossibleDayIsUnparseable(t *testing.T) {
	require.False(t, datetimeMatches("2026-02-30", "is before", []string{instant}))
	require.False(t, datetimeMatches("2026-01-32", "is before", []string{instant}))
	require.False(t, datetimeMatches("2025-02-29", "is before", []string{instant}))
}

func TestDatetimeOnALeapDayIsParseable(t *testing.T) {
	require.True(t, datetimeMatches("2024-02-29", "is before", []string{instant}))
}

func TestDatetimeWithAnImpossibleMonthIsUnparseable(t *testing.T) {
	require.False(t, datetimeMatches("2026-13-01", "is before", []string{instant}))
}

func TestDatetimeWithAnImpossibleHourIsUnparseable(t *testing.T) {
	require.False(t, datetimeMatches("2026-01-28T24:00", "is before", []string{instant}))
}

func TestDatetimeWithAnImpossibleMinuteIsUnparseable(t *testing.T) {
	require.False(t, datetimeMatches("2026-01-28T01:60", "is before", []string{instant}))
}

func TestDatetimeWithAnImpossibleSecondIsUnparseable(t *testing.T) {
	require.False(t, datetimeMatches("2026-01-28T01:25:60", "is before", []string{instant}))
}

func TestDatetimeThatIsGarbageIsUnparseable(t *testing.T) {
	require.False(t, datetimeMatches("garbage", "is before", []string{instant}))
	require.False(t, datetimeMatches("", "is before", []string{instant}))
}

func TestDatetimeWithAnUnparseableTargetDoesNotMatch(t *testing.T) {
	require.False(t, datetimeMatches(instant, "is before", []string{"garbage"}))
	require.False(t, datetimeMatches(instant, "is after", []string{""}))
}

func TestDatetimeWithNoTargetsDoesNotMatch(t *testing.T) {
	require.False(t, datetimeMatches(instant, "is before", nil))
}

func TestDatetimeWithAnUnknownOperatorDoesNotMatch(t *testing.T) {
	require.False(t, datetimeMatches("2026-01-28", "=", []string{"2026-01-28"}))
}
