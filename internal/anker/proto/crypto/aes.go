// Package crypto implements the cryptographic functions used by the AnkerMake M5 protocol.
//
// This includes:
//   - AES-256-CBC encryption/decryption for MQTT messages
//   - XOR-based checksum for MQTT messages
//   - "Curse" cipher for PPPP protocol encryption
//   - "Simple" cipher (lib32100 variant) for PPPP protocol encryption
//   - PPPP init string decoder
//   - ECDH key exchange for password encryption
//   - Security code generation (v1/v2)
//   - Login cache decryption (AES-ECB)
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"fmt"
)

// pkcs7Pad pads data to blockSize using PKCS7 padding.
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	pad := make([]byte, padding)
	for i := range pad {
		pad[i] = byte(padding)
	}
	return append(data, pad...)
}

// pkcs7Unpad removes PKCS7 padding from data.
func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty data, cannot unpad")
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > len(data) || padding > 16 {
		return nil, fmt.Errorf("invalid PKCS7 padding: %d", padding)
	}
	return data[:len(data)-padding], nil
}

// AESCBCEncrypt encrypts data using AES-CBC with PKCS7 padding.
func AESCBCEncrypt(msg, key, iv []byte) ([]byte, error) {
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return nil, fmt.Errorf("invalid AES key size: %d", len(key))
	}
	if len(iv) != 16 {
		return nil, fmt.Errorf("invalid IV size: %d", len(iv))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("AES cipher creation: %w", err)
	}

	padded := pkcs7Pad(msg, block.BlockSize())
	encrypted := make([]byte, len(padded))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(encrypted, padded)
	return encrypted, nil
}

// AESCBCDecrypt decrypts data using AES-CBC with PKCS7 padding removal.
func AESCBCDecrypt(cmsg, key, iv []byte) ([]byte, error) {
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return nil, fmt.Errorf("invalid AES key size: %d", len(key))
	}
	if len(iv) != 16 {
		return nil, fmt.Errorf("invalid IV size: %d", len(iv))
	}
	if len(cmsg)%16 != 0 {
		return nil, fmt.Errorf("ciphertext length %d is not a multiple of block size", len(cmsg))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("AES cipher creation: %w", err)
	}

	decrypted := make([]byte, len(cmsg))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(decrypted, cmsg)

	return pkcs7Unpad(decrypted)
}

// Default MQTT AES IV
var defaultMQTTIV = []byte("3DPrintAnkerMake")

// MQTTAESEncrypt encrypts a message using AES-CBC with the default MQTT IV.
func MQTTAESEncrypt(msg, key []byte) ([]byte, error) {
	return AESCBCEncrypt(msg, key, defaultMQTTIV)
}

// MQTTAESDecrypt decrypts a message using AES-CBC with the default MQTT IV.
func MQTTAESDecrypt(cmsg, key []byte) ([]byte, error) {
	return AESCBCDecrypt(cmsg, key, defaultMQTTIV)
}

// MQTTAESDecryptWithIV decrypts a message using AES-CBC with a custom IV.
func MQTTAESDecryptWithIV(cmsg, key, iv []byte) ([]byte, error) {
	return AESCBCDecrypt(cmsg, key, iv)
}

// AESDecryptBlock decrypts a single 16-byte AES block in ECB mode (no padding).
func AESDecryptBlock(block, key []byte) ([]byte, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("AES cipher creation: %w", err)
	}
	dst := make([]byte, 16)
	b.Decrypt(dst, block)
	return dst, nil
}
