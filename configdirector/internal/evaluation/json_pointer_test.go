package evaluation

import (
	"strings"
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

func escapedPointerToken(member string) string {
	return strings.ReplaceAll(strings.ReplaceAll(member, "~", "~0"), "/", "~1")
}

// FuzzTraitAtPointer checks that a lookup never panics, selects nothing in nil traits or
// through a pointer without a leading slash, and that a member of the traits is selected by
// exactly one pointer: the slash followed by its escaped name.
func FuzzTraitAtPointer(f *testing.F) {
	f.Add("/a/b", "a")
	f.Add("/a~1b~0c/d~0~1e", "a/b~c")
	f.Add("/a~1b", "a/b")
	f.Add("/a/b", "a/b")
	f.Add("/list/0", "list")
	f.Add("", "")
	f.Add("/", "")
	f.Add("/a~", "a~")
	f.Add("/a~2", "a~2")
	f.Add("#/foo", "foo")
	f.Add("/c%25d", "c%d")

	f.Fuzz(func(t *testing.T, pointer string, member string) {
		traits := map[string]any{member: "found"}
		onlyPointerToMember := "/" + escapedPointerToken(member)

		require.Nil(t, traitAtPointer(nil, pointer))
		if !strings.HasPrefix(pointer, "/") {
			require.Nil(t, traitAtPointer(traits, pointer))
		}
		require.Equal(t, "found", traitAtPointer(traits, onlyPointerToMember))
		selected := traitAtPointer(traits, pointer)
		if pointer == onlyPointerToMember {
			require.Equal(t, "found", selected)
		} else {
			require.Nil(t, selected)
		}
	})
}
