package crypto

import (
	"errors"
	"fmt"
)

// XORBytes computes the XOR of all bytes in data.
func XORBytes(data []byte) byte {
	var s byte
	for _, b := range data {
		s ^= b
	}
	return s
}

// MQTTChecksumAdd appends a checksum byte (XOR of all bytes) to the message.
func MQTTChecksumAdd(msg []byte) []byte {
	result := make([]byte, len(msg)+1)
	copy(result, msg)
	result[len(msg)] = XORBytes(msg)
	return result
}

// MQTTChecksumRemove verifies the checksum and strips it from the end.
// Returns an error if the checksum is invalid (bug fix: Python version only prints).
func MQTTChecksumRemove(payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return nil, ErrEmptyPayload
	}
	checksum := XORBytes(payload)
	if checksum != 0 {
		return nil, fmt.Errorf("malformed message: checksum mismatch (expected 0, got 0x%02x)", checksum)
	}
	return payload[:len(payload)-1], nil
}

var ErrEmptyPayload = errors.New("empty payload")
