package evaluation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHashedPercentageHashesTheIdentifierThenTheConfigID(t *testing.T) {
	require.Equal(t, 61.8, hashedPercentage("00000000-0000-0000-0000-000000000001", "abc"))
}

func TestHashedPercentageChangesWithTheConfigID(t *testing.T) {
	require.Equal(t, 34.0, hashedPercentage("00000000-0000-0000-0000-0000000003e8", "abc"))
}

func TestHashedPercentageOfANumericIdentifierIsTakenFromItsText(t *testing.T) {
	require.Equal(t, 13.5, hashedPercentage("00000000-0000-0000-0000-0000000007d0", "378368375"))
}

func TestHashedPercentageOfTheNextIdentifierLandsElsewhere(t *testing.T) {
	require.Equal(t, 66.0, hashedPercentage("00000000-0000-0000-0000-0000000003e8", "378368376"))
}
