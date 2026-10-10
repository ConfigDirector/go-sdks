package telemetry

import (
	"crypto/sha256"
	"math/big"
)

const (
	valueIDDigestBytes = 16
	valueIDLength      = 22
	base62Digits       = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

var base62Radix = big.NewInt(int64(len(base62Digits)))

// ValueIDFor identifies a config value by a digest of its text, spelled exactly as every other
// ConfigDirector SDK spells it: the leading 16 bytes of the SHA-256 digest as a base62 number,
// zero-padded to 22 characters.
func ValueIDFor(text string) string {
	digest := sha256.Sum256([]byte(text))
	number := new(big.Int).SetBytes(digest[:valueIDDigestBytes])

	id := make([]byte, valueIDLength)
	remainder := new(big.Int)
	for index := valueIDLength - 1; index >= 0; index-- {
		number.QuoRem(number, base62Radix, remainder)
		id[index] = base62Digits[remainder.Int64()]
	}
	return string(id)
}
