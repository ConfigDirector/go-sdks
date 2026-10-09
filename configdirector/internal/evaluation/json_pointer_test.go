package evaluation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTraitAtPointerSelectsANestedMember(t *testing.T) {
	traits := map[string]any{"a": map[string]any{"b": "x"}}

	require.Equal(t, "x", traitAtPointer(traits, "/a/b"))
}

func TestTraitAtPointerDecodesEscapedSlashAndTilde(t *testing.T) {
	traits := map[string]any{"a/b": 1.0, "m~n": 2.0}

	require.Equal(t, 1.0, traitAtPointer(traits, "/a~1b"))
	require.Equal(t, 2.0, traitAtPointer(traits, "/m~0n"))
}

func TestTraitAtPointerDecodesSlashBeforeTilde(t *testing.T) {
	traits := map[string]any{"~1": "x", "/": "y"}

	require.Equal(t, "x", traitAtPointer(traits, "/~01"))
}

func TestTraitAtPointerSelectsEmptyMemberNames(t *testing.T) {
	traits := map[string]any{"": map[string]any{"": "x"}}

	require.Equal(t, map[string]any{"": "x"}, traitAtPointer(traits, "/"))
	require.Equal(t, "x", traitAtPointer(traits, "//"))
}

func TestTraitAtPointerWithAnInvalidEscapeSelectsNothing(t *testing.T) {
	traits := map[string]any{"~2": "x"}

	require.Nil(t, traitAtPointer(traits, "/~2"))
}

func TestTraitAtPointerWithATrailingTildeSelectsNothing(t *testing.T) {
	traits := map[string]any{"a~": "x"}

	require.Nil(t, traitAtPointer(traits, "/a~"))
}

func TestTraitAtPointerWithoutALeadingSlashSelectsNothing(t *testing.T) {
	traits := map[string]any{"foo": "x"}

	require.Nil(t, traitAtPointer(traits, "foo"))
	require.Nil(t, traitAtPointer(traits, "#/foo"))
}

func TestTraitAtPointerDoesNotDecodePercentEncoding(t *testing.T) {
	traits := map[string]any{"c%d": "x", "c%25d": "y"}

	require.Equal(t, "y", traitAtPointer(traits, "/c%25d"))
}

func TestTraitAtPointerSelectsAnArrayElementByIndex(t *testing.T) {
	traits := map[string]any{"list": []any{"a", "b"}}

	require.Equal(t, "b", traitAtPointer(traits, "/list/1"))
}

func TestTraitAtPointerRejectsAnIndexWithALeadingZero(t *testing.T) {
	traits := map[string]any{"list": []any{"a", "b"}}

	require.Nil(t, traitAtPointer(traits, "/list/01"))
}

func TestTraitAtPointerRejectsASignedIndex(t *testing.T) {
	traits := map[string]any{"list": []any{"a", "b"}}

	require.Nil(t, traitAtPointer(traits, "/list/+1"))
	require.Nil(t, traitAtPointer(traits, "/list/-0"))
}

func TestTraitAtPointerRejectsTheEndOfArrayMarker(t *testing.T) {
	traits := map[string]any{"list": []any{"a", "b"}}

	require.Nil(t, traitAtPointer(traits, "/list/-"))
}

func TestTraitAtPointerRejectsAnIndexPastTheEnd(t *testing.T) {
	traits := map[string]any{"list": []any{"a", "b"}}

	require.Nil(t, traitAtPointer(traits, "/list/2"))
}

func TestTraitAtPointerRejectsAnIndexTooLargeForAnInteger(t *testing.T) {
	traits := map[string]any{"list": []any{"a", "b"}}

	require.Nil(t, traitAtPointer(traits, "/list/99999999999999999999999"))
}

func TestTraitAtPointerRejectsNonASCIIDigits(t *testing.T) {
	traits := map[string]any{"list": []any{"a", "b"}}

	require.Nil(t, traitAtPointer(traits, "/list/١"))
}

func TestTraitAtPointerDoesNotIndexAString(t *testing.T) {
	traits := map[string]any{"s": "abc"}

	require.Nil(t, traitAtPointer(traits, "/s/0"))
	require.Nil(t, traitAtPointer(traits, "/s/length"))
}

func TestTraitAtPointerSelectsNothingBelowAMissingMember(t *testing.T) {
	traits := map[string]any{}

	require.Nil(t, traitAtPointer(traits, "/missing/x"))
}

func TestTraitAtPointerOnNilTraitsSelectsNothing(t *testing.T) {
	require.Nil(t, traitAtPointer(nil, "/x"))
}
