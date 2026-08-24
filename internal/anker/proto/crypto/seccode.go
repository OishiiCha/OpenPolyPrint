package crypto

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
)

// CalcCheckCode computes the old v1 check code (MD5-based).
// Input: serial number (DUID string) and MAC address (without colons).
func CalcCheckCode(sn, mac string) string {
	input := fmt.Sprintf("%s+%s+%s", sn, sn[len(sn)-4:], mac)
	hash := md5.Sum([]byte(input))
	return hex.EncodeToString(hash[:])
}

// calHwIDSuffix computes the hardware ID suffix from the MAC address bytes.
func calHwIDSuffix(mac []byte) int {
	if len(mac) < 4 {
		return 0
	}
	return hexDigitToInt(mac[len(mac)-1]) +
		hexDigitToInt(mac[len(mac)-2]) +
		hexDigitToInt(mac[len(mac)-3]) +
		hexDigitToInt(mac[len(mac)-4])
}

func hexDigitToInt(b byte) int {
	switch {
	case b >= '0' && b <= '9':
		return int(b - '0')
	case b >= 'a' && b <= 'f':
		return int(b-'a') + 10
	case b >= 'A' && b <= 'F':
		return int(b-'A') + 10
	default:
		return 0
	}
}

// genBaseCode generates the base code for security code v1.
func genBaseCode(sn, mac []byte) []byte {
	lastDigit := hexDigitToInt(sn[len(sn)-1])
	offset := (lastDigit + 10) % 10
	suffix := fmt.Sprintf("%d", calHwIDSuffix(mac))
	return append([]byte(sn[offset:]), []byte(suffix)...)
}

// GenCheckCodeV1 generates the v1 security check code.
func GenCheckCodeV1(baseCode, seed []byte) string {
	base := append([]byte("01"), baseCode...)
	base = append(base, seed...)

	sha := sha256.Sum256(base)

	// str = sha + sha[10:12] (34 bytes total)
	str := make([]byte, 34)
	copy(str[:32], sha[:])
	str[32] = sha[10]
	str[33] = sha[11]

	if str[32] < 0x7d || str[33] < 0x7d {
		str[32] = (str[32] + str[33]) & 0xFF
	}

	for x := 0; x < 32; x += 2 {
		if str[x] < 0x7d || str[x+1] < 0x7d {
			str[x] = (str[x] + str[x+1]) & 0xFF
		}

		if max(0x7d, str[x+1]) < str[x+2] {
			str[x+1] = str[x+2] - str[x+1]
		}

		if str[x+1] > 0x7d && str[x+1] > str[x+2] {
			str[x+1] = str[x+1] - str[x+2]
		}
	}

	result := hex.EncodeToString(str[0x10:0x20])
	return toUpper(result)
}

func max(a, b byte) byte {
	if a > b {
		return a
	}
	return b
}

func toUpper(s string) string {
	result := make([]byte, len(s))
	for i, c := range []byte(s) {
		if c >= 'a' && c <= 'z' {
			result[i] = c - 32
		} else {
			result[i] = c
		}
	}
	return string(result)
}

// GenRandSeed generates a random seed and security timestamp.
func GenRandSeed(mac []byte) (secTs string, secCode []byte, err error) {
	// Generate random number between 10000000 and 99999999
	rnd, err := rand.Int(rand.Reader, big.NewInt(90000000))
	if err != nil {
		return "", nil, fmt.Errorf("generate random seed: %w", err)
	}
	rnd.Add(rnd, big.NewInt(10000000))
	rndInt := rnd.Int64()

	suffix := calHwIDSuffix(mac)
	txtbuf := fmt.Sprintf("%d%d", 1000-suffix, rndInt)

	secTs = fmt.Sprintf("01%d", rndInt)
	hash := md5.Sum([]byte(txtbuf))
	secCode = []byte(toUpper(hex.EncodeToString(hash[:])))

	return secTs, secCode, nil
}

// CreateCheckCodeV1 creates a complete v1 security code pair (timestamp + code).
func CreateCheckCodeV1(sn, mac []byte) (secTs string, secCode string, err error) {
	baseCode := genBaseCode(sn, mac)
	ts, seed, err := GenRandSeed(mac)
	if err != nil {
		return "", "", err
	}
	code := GenCheckCodeV1(baseCode, seed)
	return ts, code, nil
}
