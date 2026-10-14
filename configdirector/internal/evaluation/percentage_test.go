package evaluation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHashedPercentageHashesTheIdentifierThenTheConfigID(t *testing.T) {
	require.Equal(t, 61.8, hashedPercentage("00000000-0000-0000-0000-000000000001", Context{Identifier: "abc"}))
}

func TestHashedPercentageChangesWithTheConfigID(t *testing.T) {
	require.Equal(t, 34.0, hashedPercentage("00000000-0000-0000-0000-0000000003e8", Context{Identifier: "abc"}))
}

func TestHashedPercentageOfANumericIdentifierIsTakenFromItsText(t *testing.T) {
	require.Equal(t, 13.5, hashedPercentage("00000000-0000-0000-0000-0000000007d0", Context{Identifier: "378368375"}))
}

func TestHashedPercentageOfTheNextIdentifierLandsElsewhere(t *testing.T) {
	require.Equal(t, 66.0, hashedPercentage("00000000-0000-0000-0000-0000000003e8", Context{Identifier: "378368376"}))
}

func TestHashedPercentageOfADeviceAppendsTheKindToTheUserInput(t *testing.T) {
	require.Equal(t, 59.0, hashedPercentage("11111111-1111-4111-8111-111111111111", Context{Identifier: "user-1", Kind: "device"}))
}

func TestHashedPercentageOfATenantLandsElsewhereThanADevice(t *testing.T) {
	require.Equal(t, 8.5, hashedPercentage("11111111-1111-4111-8111-111111111111", Context{Identifier: "user-1", Kind: "tenant"}))
}

func TestHashedPercentageOfAnExplicitUserKindIsTheInputWithoutAKind(t *testing.T) {
	require.Equal(t, 7.8, hashedPercentage("11111111-1111-4111-8111-111111111111", Context{Identifier: "user-1", Kind: "user"}))
	require.Equal(t, 7.8, hashedPercentage("11111111-1111-4111-8111-111111111111", Context{Identifier: "user-1"}))
}
