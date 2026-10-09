package evaluation

import (
	"encoding/binary"
	"math/bits"
)

var rapidhashSecret = [8]uint64{
	0x2D358DCCAA6C78A5,
	0x8BB84B93962EACC9,
	0x4B33A62ED433D4A3,
	0x4D5A2DA51DE1AA47,
	0xA0761D6478BD642F,
	0xE7037ED1A0B428DB,
	0x90ED1765281C388C,
	0xAAAAAAAAAAAAAAAA,
}

func rapidhash(data []byte, seed uint64) uint64 {
	length := uint64(len(data))
	seed ^= rapidhashMix(seed^rapidhashSecret[2], rapidhashSecret[1])

	var a, b, lastBlockLength uint64
	if length <= 16 {
		a, b = rapidhashShortInput(data, length)
		if length >= 4 {
			seed ^= length
		}
		lastBlockLength = length
	} else {
		var remaining []byte
		remaining, seed = rapidhashBlocks(data, seed)
		seed = rapidhashRemainderPairs(remaining, seed)
		lastBlockLength = uint64(len(remaining))
		a = rapidhashRead64(data[len(data)-16:]) ^ lastBlockLength
		b = rapidhashRead64(data[len(data)-8:])
	}

	a ^= rapidhashSecret[1]
	b ^= seed
	return rapidhashEpilogue(a, b, lastBlockLength)
}

func rapidhashShortInput(data []byte, length uint64) (a, b uint64) {
	switch {
	case length >= 8:
		return rapidhashRead64(data), rapidhashRead64(data[length-8:])
	case length >= 4:
		return rapidhashRead32(data), rapidhashRead32(data[length-4:])
	case length > 0:
		first := uint64(data[0])
		a = uint64(data[length-1]) | (((first << 5) & 0xFF) << 40) | ((first >> 3) << 48)
		return a, uint64(data[length>>1])
	default:
		return 0, 0
	}
}

func rapidhashBlocks(data []byte, seed uint64) ([]byte, uint64) {
	if len(data) <= 112 {
		return data, seed
	}
	lanes := [7]uint64{seed, seed, seed, seed, seed, seed, seed}
	for {
		for lane := range lanes {
			lanes[lane] = rapidhashMix(
				rapidhashRead64(data[lane*16:])^rapidhashSecret[lane],
				rapidhashRead64(data[lane*16+8:])^lanes[lane],
			)
		}
		data = data[112:]
		if len(data) <= 112 {
			break
		}
	}
	seed = lanes[0] ^ lanes[1]
	seed ^= lanes[6]
	seed ^= lanes[2] ^ lanes[3] ^ lanes[4] ^ lanes[5]
	return data, seed
}

func rapidhashRemainderPairs(data []byte, seed uint64) uint64 {
	secrets := [6]uint64{
		rapidhashSecret[2], rapidhashSecret[2],
		rapidhashSecret[1], rapidhashSecret[1],
		rapidhashSecret[2], rapidhashSecret[1],
	}
	for pair, secret := range secrets {
		if len(data) <= 16+pair*16 {
			break
		}
		seed = rapidhashMix(rapidhashRead64(data[pair*16:])^secret, rapidhashRead64(data[pair*16+8:])^seed)
	}
	return seed
}

func rapidhashMix(a, b uint64) uint64 {
	high, low := bits.Mul64(a, b)
	return low ^ high
}

func rapidhashEpilogue(a, b, lastBlockLength uint64) uint64 {
	high, low := bits.Mul64(a, b)
	return rapidhashMix(low^rapidhashSecret[7], high^rapidhashSecret[1]^lastBlockLength)
}

func rapidhashRead64(data []byte) uint64 {
	return binary.LittleEndian.Uint64(data)
}

func rapidhashRead32(data []byte) uint64 {
	return uint64(binary.LittleEndian.Uint32(data))
}
