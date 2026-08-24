package crypto

import (
	"fmt"
	"strings"
)

// ppppInitShuffle is the shuffle table for decoding PPPP init strings.
var ppppInitShuffle = [0x36]byte{
	0x49, 0x59, 0x43, 0x3d, 0xb5, 0xbf, 0x6d, 0xa3, 0x47, 0x53,
	0x4f, 0x61, 0x65, 0xe3, 0x71, 0xe9, 0x67, 0x7f, 0x02, 0x03,
	0x0b, 0xad, 0xb3, 0x89, 0x2b, 0x2f, 0x35, 0xc1, 0x6b, 0x8b,
	0x95, 0x97, 0x11, 0xe5, 0xa7, 0x0d, 0xef, 0xf1, 0x05, 0x07,
	0x83, 0xfb, 0x9d, 0x3b, 0xc5, 0xc7, 0x13, 0x17, 0x1d, 0x1f,
	0x25, 0x29, 0xd3, 0xdf,
}

// PPPPDecodeInitStringRaw decodes a raw PPPP init string to bytes.
func PPPPDecodeInitStringRaw(input []byte) ([]byte, error) {
	olen := len(input) >> 1
	output := make([]byte, olen)

	for q := 0; q < olen; q++ {
		var xor = 0x39 ^ ppppInitShuffle[q%0x36]

		for p := 0; p <= q; p++ {
			xor ^= output[p]
		}

		l := input[q*2+1] - 0x41
		h := input[q*2+0] - 0x41
		output[q] = xor ^ (l + (h << 4))
	}

	return output, nil
}

// PPPPDecodeInitString decodes a PPPP init string and splits it by commas.
func PPPPDecodeInitString(input string) ([]string, error) {
	raw, err := PPPPDecodeInitStringRaw([]byte(input))
	if err != nil {
		return nil, err
	}
	decoded := string(raw)
	decoded = strings.TrimRight(decoded, ",")
	return strings.Split(decoded, ","), nil
}

// FormatInitStringForDebug returns a debug representation of the decoded init string.
func FormatInitStringForDebug(input string) string {
	parts, err := PPPPDecodeInitString(input)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	return fmt.Sprintf("%v", parts)
}
