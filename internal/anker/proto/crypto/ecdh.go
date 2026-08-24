package crypto

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// AnkerECPublicKeyX is the X coordinate of Anker's ECDH public key (secp256r1 / P-256).
var AnkerECPublicKeyX = []byte{
	0xC5, 0xC0, 0x0C, 0x4F, 0x8D, 0x11, 0x97, 0xCC,
	0x7C, 0x31, 0x67, 0xC5, 0x2B, 0xF7, 0xAC, 0xB0,
	0x54, 0xD7, 0x22, 0xF0, 0xEF, 0x08, 0xDC, 0xD7,
	0xE0, 0x88, 0x32, 0x36, 0xE0, 0xD7, 0x2A, 0x38,
}

// AnkerECPublicKeyY is the Y coordinate of Anker's ECDH public key (secp256r1 / P-256).
var AnkerECPublicKeyY = []byte{
	0x68, 0xD9, 0x75, 0x0C, 0xB4, 0x7F, 0xA4, 0x61,
	0x92, 0x48, 0xF3, 0xD8, 0x3F, 0x0F, 0x66, 0x26,
	0x71, 0xDA, 0xDC, 0x6E, 0x2D, 0x31, 0xC2, 0xF4,
	0x1D, 0xB0, 0x16, 0x16, 0x51, 0xC7, 0xC0, 0x76,
}

// ECDHEncryptResult contains the result of ECDH password encryption.
type ECDHEncryptResult struct {
	PublicKey  string // Hex-encoded ephemeral public key (04 + X + Y)
	Ciphertext string // Base64-encoded AES-CBC encrypted password
}

// ECDHEncryptLoginPassword encrypts a password using ECDH with Anker's public key.
// It generates a fresh ephemeral key pair, performs ECDH with Anker's public key,
// uses the shared secret as the AES key (first 32 bytes) and IV (first 16 bytes),
// then encrypts the password with AES-CBC.
func ECDHEncryptLoginPassword(password []byte) (*ECDHEncryptResult, error) {
	curve := ecdh.P256()

	// Parse Anker's public key
	ankerPubKeyBytes := make([]byte, 65)
	ankerPubKeyBytes[0] = 0x04 // Uncompressed point prefix
	copy(ankerPubKeyBytes[1:33], AnkerECPublicKeyX)
	copy(ankerPubKeyBytes[33:65], AnkerECPublicKeyY)

	ankerPubKey, err := curve.NewPublicKey(ankerPubKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("parse Anker public key: %w", err)
	}

	// Generate ephemeral key pair
	privKey, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ECDH key: %w", err)
	}

	// Perform ECDH to get shared secret
	sharedSecret, err := privKey.ECDH(ankerPubKey)
	if err != nil {
		return nil, fmt.Errorf("ECDH exchange: %w", err)
	}

	// Key is the full 32-byte shared secret, IV is the first 16 bytes
	key := sharedSecret
	iv := sharedSecret[:16]

	// Encrypt password with AES-CBC
	ciphertext, err := AESCBCEncrypt(password, key, iv)
	if err != nil {
		return nil, fmt.Errorf("AES encrypt password: %w", err)
	}

	// Export ephemeral public key as hex: "04" + X + Y
	pubKeyBytes := privKey.PublicKey().Bytes()
	pubKeyHex := hex.EncodeToString(pubKeyBytes)

	// Base64 encode the ciphertext
	ciphertextB64 := base64.StdEncoding.EncodeToString(ciphertext)

	return &ECDHEncryptResult{
		PublicKey:  pubKeyHex,
		Ciphertext: ciphertextB64,
	}, nil
}
