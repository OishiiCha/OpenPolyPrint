package crypto

import (
	"errors"
	"fmt"
)

// PPPPSeed is the key used for the curse cipher.
var PPPPSeed = []byte("EUPRAKM")

// PPPPShuffle is the shuffle table used by the curse cipher.
var PPPPShuffle = [8][8]byte{
	{0x95, 0xe5, 0x61, 0x97, 0x83, 0x0d, 0xa7, 0xf1},
	{0xd3, 0x05, 0x95, 0x8b, 0xdf, 0x13, 0x6d, 0xef},
	{0x07, 0x61, 0x0d, 0x6d, 0x7f, 0x67, 0x17, 0x2b},
	{0xc1, 0xb5, 0x13, 0x0b, 0xdf, 0x8b, 0x49, 0x3b},
	{0x7f, 0x07, 0xd3, 0x02, 0x6d, 0x2f, 0x13, 0xc5},
	{0x6d, 0x3d, 0xfb, 0x0d, 0x0b, 0x29, 0xe9, 0x4f},
	{0x89, 0x2f, 0xe3, 0xe9, 0x0d, 0x83, 0x6d, 0xe5},
	{0x07, 0x53, 0x8b, 0x25, 0x95, 0x47, 0x1f, 0x29},
}

// cryptoCurseState holds the running state (a, b, c, d) of the curse cipher.
type curseState struct {
	a, b, c, d byte
}

// advance updates the curse cipher state based on a byte value.
func (s *curseState) advance(x byte, shuffle *[8][8]byte) {
	a, b, c, d := s.a, s.b, s.c, s.d
	s.a = shuffle[(b+(x%a))&7][(x+(c%d))&7]
	s.b = shuffle[(c+(x%b))&7][(x+(d%a))&7]
	s.c = shuffle[(d+(x%c))&7][(x+(a%b))&7]
	s.d = shuffle[(a+(x%d))&7][(x+(b%c))&7]
	_ = a
	_ = b
	_ = c
	_ = d
}

// initCurseState initializes the curse cipher state from the key.
func initCurseState(key []byte, shuffle *[8][8]byte) curseState {
	s := curseState{a: 1, b: 3, c: 5, d: 7}
	for _, q := range key {
		s.advance(q, shuffle)
	}
	return s
}

// CryptoCurse encrypts the input using the curse cipher with the given key and shuffle table.
// Returns the encrypted data plus 4 trailing checksum bytes.
func CryptoCurse(input, key []byte, shuffle *[8][8]byte) []byte {
	s := initCurseState(key, shuffle)
	output := make([]byte, len(input)+4)

	for p, x := range input {
		x = x ^ (s.a ^ s.b ^ s.c ^ s.d)
		output[p] = x
		s.advance(x, shuffle)
	}

	for p := len(input); p < len(input)+4; p++ {
		x := s.a ^ s.b ^ s.c ^ s.d ^ 0x43
		output[p] = x
		s.advance(x, shuffle)
	}

	return output
}

// CryptoDecurse decrypts the input using the curse cipher with the given key and shuffle table.
// The input should include the 4 trailing checksum bytes.
func CryptoDecurse(input, key []byte, shuffle *[8][8]byte) []byte {
	s := initCurseState(key, shuffle)
	output := make([]byte, len(input))

	for p, x := range input {
		output[p] = x ^ (s.a ^ s.b ^ s.c ^ s.d)
		s.advance(x, shuffle)
	}

	return output
}

// CryptoCurseString encrypts input using the default PPPP seed and shuffle.
func CryptoCurseString(input []byte) []byte {
	return CryptoCurse(input, PPPPSeed, &PPPPShuffle)
}

// CryptoDecurseString decrypts input using the default PPPP seed and shuffle.
// Verifies the 4 trailing checksum bytes are all 0x43.
func CryptoDecurseString(input []byte) ([]byte, error) {
	output := CryptoDecurse(input, PPPPSeed, &PPPPShuffle)

	if len(output) < 4 {
		return nil, errors.New("input too short for curse decryption")
	}

	tail := output[len(output)-4:]
	expected := []byte{0x43, 0x43, 0x43, 0x43}
	for i := range tail {
		if tail[i] != expected[i] {
			return nil, fmt.Errorf("invalid curse decode: checksum mismatch at byte %d (expected 0x43, got 0x%02x)", i, tail[i])
		}
	}

	return output[:len(output)-4], nil
}
