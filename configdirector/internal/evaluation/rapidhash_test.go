package evaluation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const vectorSeed = 0x397832987

func TestRapidhashOfAnEmptyMessageMatchesTheReferenceImplementation(t *testing.T) {
	require.Equal(t, uint64(5377612543505373799), rapidhash([]byte(""), vectorSeed))
}

func TestRapidhashOfOneByteMatchesTheReferenceImplementation(t *testing.T) {
	require.Equal(t, uint64(7674800498429868151), rapidhash([]byte("a"), vectorSeed))
}

func TestRapidhashOfTwoBytesMatchesTheReferenceImplementation(t *testing.T) {
	require.Equal(t, uint64(12048270741005468339), rapidhash([]byte("ab"), vectorSeed))
}

func TestRapidhashOfThreeBytesMatchesTheReferenceImplementation(t *testing.T) {
	require.Equal(t, uint64(8205525400821834274), rapidhash([]byte("abc"), vectorSeed))
}

func TestRapidhashOfFourBytesMatchesTheReferenceImplementation(t *testing.T) {
	require.Equal(t, uint64(2559843570930408943), rapidhash([]byte("abcd"), vectorSeed))
}

func TestRapidhashOfFiveBytesMatchesTheReferenceImplementation(t *testing.T) {
	require.Equal(t, uint64(17182111687207956362), rapidhash([]byte("abcde"), vectorSeed))
}

func TestRapidhashOfSevenBytesMatchesTheReferenceImplementation(t *testing.T) {
	require.Equal(t, uint64(13135472276134024436), rapidhash([]byte("abcdefg"), vectorSeed))
}

func TestRapidhashOfEightBytesMatchesTheReferenceImplementation(t *testing.T) {
	require.Equal(t, uint64(10825195283420988801), rapidhash([]byte("abcdefgh"), vectorSeed))
}

func TestRapidhashOfNineBytesMatchesTheReferenceImplementation(t *testing.T) {
	require.Equal(t, uint64(9324307379318471710), rapidhash([]byte("abcdefghi"), vectorSeed))
}

func TestRapidhashOfSixteenBytesMatchesTheReferenceImplementation(t *testing.T) {
	require.Equal(t, uint64(8410398172536096822), rapidhash([]byte("0123456789abcdef"), vectorSeed))
}

func TestRapidhashOfSeventeenBytesMatchesTheReferenceImplementation(t *testing.T) {
	require.Equal(t, uint64(8975841632926530338), rapidhash([]byte("0123456789abcdefg"), vectorSeed))
}

func TestRapidhashOfABucketingSaltMatchesTheReferenceImplementation(t *testing.T) {
	require.Equal(t, uint64(7715065197445012089), rapidhash([]byte("10-11111111-1111-4111-8111-111111111111"), vectorSeed))
}

func TestRapidhashOfFortyEightBytesMatchesTheReferenceImplementation(t *testing.T) {
	require.Equal(t, uint64(1185046273860983588), rapidhash([]byte(strings.Repeat("x", 48)), vectorSeed))
}

func TestRapidhashOfOneFullBlockMatchesTheReferenceImplementation(t *testing.T) {
	require.Equal(t, uint64(5679430438346846087), rapidhash([]byte(strings.Repeat("y", 112)), vectorSeed))
}

func TestRapidhashOfOneBlockAndOneByteMatchesTheReferenceImplementation(t *testing.T) {
	require.Equal(t, uint64(14103938338420400619), rapidhash([]byte(strings.Repeat("z", 113)), vectorSeed))
}

func TestRapidhashOfTwoBlocksAndARemainderMatchesTheReferenceImplementation(t *testing.T) {
	require.Equal(t, uint64(15088383192705595115), rapidhash([]byte(strings.Repeat("w", 240)), vectorSeed))
}

func TestRapidhashOfMultiByteUTF8MatchesTheReferenceImplementation(t *testing.T) {
	require.Equal(t, uint64(7481766294562949397), rapidhash([]byte("héllo wörld ✓"), vectorSeed))
}
