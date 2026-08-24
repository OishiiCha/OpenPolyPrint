package crypto

import (
	"crypto/aes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// CacheKey is the AES key used to decrypt the AnkerMake slicer login cache.
var CacheKey = []byte{0x1b, 0x55, 0xf9, 0x77, 0x93, 0xd5, 0x88, 0x64, 0x57, 0x1e, 0x10, 0x55, 0x83, 0x8c, 0xac, 0x97}

// USRegions is the set of country codes that map to the "us" region.
var USRegions = map[string]bool{
	"US": true, "CA": true, "MX": true, "BR": true, "AR": true,
	"CU": true, "BS": true, "AU": true, "NZ": true,
}

// GuessRegion determines the API region from a country code.
func GuessRegion(cc string) string {
	if USRegions[cc] {
		return "us"
	}
	return "eu"
}

// DecryptLoginCache decrypts base64-encoded AES-ECB encrypted login cache data.
func DecryptLoginCache(data []byte, key []byte) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return "", fmt.Errorf("base64 decode login cache: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("AES cipher creation: %w", err)
	}

	if len(raw)%block.BlockSize() != 0 {
		return "", fmt.Errorf("ciphertext length %d is not a multiple of block size", len(raw))
	}

	decrypted := make([]byte, len(raw))
	for i := 0; i < len(raw); i += block.BlockSize() {
		block.Decrypt(decrypted[i:i+block.BlockSize()], raw[i:i+block.BlockSize()])
	}

	// Strip trailing null bytes
	return strings.TrimRight(string(decrypted), "\x00"), nil
}

// LoadLoginCache loads and decrypts the login cache, returning the parsed JSON.
// If decryption fails, it attempts to parse the data as plain JSON (for older slicer versions).
func LoadLoginCache(data []byte, key []byte) (map[string]any, error) {
	// Try encrypted decryption first
	raw, err := DecryptLoginCache(data, key)
	if err != nil {
		// Fall back to treating data as plain JSON (older slicer versions)
		raw = string(data)
	}

	raw = strings.TrimSpace(raw)

	var result map[string]any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("parse login cache JSON: %w", err)
	}

	return result, nil
}

// LoadLoginCacheDefault loads login cache using the default cache key.
func LoadLoginCacheDefault(data []byte) (map[string]any, error) {
	return LoadLoginCache(data, CacheKey)
}

// ErrInvalidLoginCache is returned when the login cache cannot be decrypted or parsed.
var ErrInvalidLoginCache = errors.New("invalid login cache data")
