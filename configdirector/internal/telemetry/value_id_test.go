package telemetry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValueIDMatchesTheOtherSDKsForHello(t *testing.T) {
	t.Parallel()
	require.Equal(t, "1MoOW7eqAPjhZeoELVwO9G", ValueIDFor("hello"))
}

func TestValueIDMatchesTheOtherSDKsForWorld(t *testing.T) {
	t.Parallel()
	require.Equal(t, "2Cg0gndCS8p6nDE5aa6LcI", ValueIDFor("world"))
}

func TestValueIDMatchesTheOtherSDKsForANumber(t *testing.T) {
	t.Parallel()
	require.Equal(t, "3VWjGpOwynZPh07ivDC56c", ValueIDFor("42"))
}

func TestValueIDMatchesTheOtherSDKsForTheEmptyString(t *testing.T) {
	t.Parallel()
	require.Equal(t, "6ve2WrOl3mnciB6WIL2fIa", ValueIDFor(""))
}

func TestValueIDIsZeroPaddedToTwentyTwoCharacters(t *testing.T) {
	t.Parallel()
	require.Equal(t, "00FwZBdJhz9Xmmqwie9qZ7", ValueIDFor("value-301"))
}

func TestValueIDUsesOnlyBase62Characters(t *testing.T) {
	t.Parallel()
	require.Regexp(t, "^[0-9A-Za-z]{22}$", ValueIDFor("unicode ☂ café"))
}

func TestValueIDHashesTheUTF8Encoding(t *testing.T) {
	t.Parallel()
	require.NotEqual(t, ValueIDFor("cafe"), ValueIDFor("café"))
}
