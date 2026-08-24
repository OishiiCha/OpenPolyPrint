// Package types provides low-level binary parsing and packing primitives
// for the AnkerMake M5 protocol messages.
//
// This file is the Go equivalent of libflagship/amtypes.py from the Python
// implementation. It provides types for reading and writing binary protocol
// data: integers of various sizes and endianness, strings, byte arrays,
// IP addresses, magic constants, and tail data.
package types

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strings"
)

// ErrTruncated is returned when there is not enough data to parse a field.
var ErrTruncated = errors.New("data truncated: not enough bytes to parse")

// ErrMagicMismatch is returned when a magic constant does not match the expected value.
var ErrMagicMismatch = errors.New("magic constant mismatch")

// ErrNonZero is returned when a zero-padding field contains non-zero bytes.
var ErrNonZero = errors.New("expected zero padding but found non-zero bytes")

// ─── Integer Types ───────────────────────────────────────────────────────────

// U8 parses and packs an unsigned 8-bit integer (big-endian, same as little-endian for 1 byte).
type U8 uint8

func ParseU8(p []byte) (U8, []byte, error) {
	if len(p) < 1 {
		return 0, nil, ErrTruncated
	}
	return U8(p[0]), p[1:], nil
}

func (v U8) Pack() []byte {
	return []byte{byte(v)}
}

// I8 parses and packs a signed 8-bit integer.
type I8 int8

func ParseI8(p []byte) (I8, []byte, error) {
	if len(p) < 1 {
		return 0, nil, ErrTruncated
	}
	return I8(p[0]), p[1:], nil
}

func (v I8) Pack() []byte {
	return []byte{byte(v)}
}

// U16BE parses and packs an unsigned 16-bit big-endian integer.
type U16BE uint16

func ParseU16BE(p []byte) (U16BE, []byte, error) {
	if len(p) < 2 {
		return 0, nil, ErrTruncated
	}
	return U16BE(binary.BigEndian.Uint16(p)), p[2:], nil
}

func (v U16BE) Pack() []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, uint16(v))
	return b
}

// U16LE parses and packs an unsigned 16-bit little-endian integer.
type U16LE uint16

func ParseU16LE(p []byte) (U16LE, []byte, error) {
	if len(p) < 2 {
		return 0, nil, ErrTruncated
	}
	return U16LE(binary.LittleEndian.Uint16(p)), p[2:], nil
}

func (v U16LE) Pack() []byte {
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, uint16(v))
	return b
}

// I16BE parses and packs a signed 16-bit big-endian integer.
type I16BE int16

func ParseI16BE(p []byte) (I16BE, []byte, error) {
	if len(p) < 2 {
		return 0, nil, ErrTruncated
	}
	return I16BE(int16(binary.BigEndian.Uint16(p))), p[2:], nil
}

func (v I16BE) Pack() []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, uint16(v))
	return b
}

// I16LE parses and packs a signed 16-bit little-endian integer.
type I16LE int16

func ParseI16LE(p []byte) (I16LE, []byte, error) {
	if len(p) < 2 {
		return 0, nil, ErrTruncated
	}
	return I16LE(int16(binary.LittleEndian.Uint16(p))), p[2:], nil
}

func (v I16LE) Pack() []byte {
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, uint16(v))
	return b
}

// U32BE parses and packs an unsigned 32-bit big-endian integer.
type U32BE uint32

func ParseU32BE(p []byte) (U32BE, []byte, error) {
	if len(p) < 4 {
		return 0, nil, ErrTruncated
	}
	return U32BE(binary.BigEndian.Uint32(p)), p[4:], nil
}

func (v U32BE) Pack() []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(v))
	return b
}

// U32LE parses and packs an unsigned 32-bit little-endian integer.
type U32LE uint32

func ParseU32LE(p []byte) (U32LE, []byte, error) {
	if len(p) < 4 {
		return 0, nil, ErrTruncated
	}
	return U32LE(binary.LittleEndian.Uint32(p)), p[4:], nil
}

func (v U32LE) Pack() []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(v))
	return b
}

// I32BE parses and packs a signed 32-bit big-endian integer.
type I32BE int32

func ParseI32BE(p []byte) (I32BE, []byte, error) {
	if len(p) < 4 {
		return 0, nil, ErrTruncated
	}
	return I32BE(int32(binary.BigEndian.Uint32(p))), p[4:], nil
}

func (v I32BE) Pack() []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(v))
	return b
}

// I32LE parses and packs a signed 32-bit little-endian integer.
type I32LE int32

func ParseI32LE(p []byte) (I32LE, []byte, error) {
	if len(p) < 4 {
		return 0, nil, ErrTruncated
	}
	return I32LE(int32(binary.LittleEndian.Uint32(p))), p[4:], nil
}

func (v I32LE) Pack() []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(v))
	return b
}

// ─── Composite Types ─────────────────────────────────────────────────────────

// Zeroes parses a padding field of n zero bytes. Returns an error if any byte is non-zero.
func ParseZeroes(p []byte, n int) ([]byte, []byte, error) {
	if len(p) < n {
		return nil, nil, ErrTruncated
	}
	body := p[:n]
	for _, b := range body {
		if b != 0 {
			return nil, nil, fmt.Errorf("%w: found byte 0x%02x in zero padding", ErrNonZero, b)
		}
	}
	return body, p[n:], nil
}

// PackZeroes returns n zero bytes.
func PackZeroes(n int) []byte {
	return make([]byte, n)
}

// FixedBytes parses a fixed-length byte slice of size n.
func ParseFixedBytes(p []byte, n int) ([]byte, []byte, error) {
	if len(p) < n {
		return nil, nil, ErrTruncated
	}
	return p[:n], p[n:], nil
}

// FixedString parses a fixed-length null-terminated string of size n.
// The last byte must be 0 (null terminator). Returns the string without the terminator.
func ParseFixedString(p []byte, n int) (string, []byte, error) {
	if len(p) < n {
		return "", nil, ErrTruncated
	}
	body := p[:n]
	if body[n-1] != 0 {
		return "", nil, fmt.Errorf("expected null terminator at position %d, found 0x%02x", n-1, body[n-1])
	}
	// Find the actual end of the string (first null byte)
	end := n - 1
	for i := 0; i < end; i++ {
		if body[i] == 0 {
			end = i
			break
		}
	}
	return string(body[:end]), p[n:], nil
}

// PackFixedString packs a string into a fixed-size buffer of size n, null-terminated and padded with zeros.
func PackFixedString(s string, n int) []byte {
	b := make([]byte, n)
	copy(b, s)
	return b
}

// Magic parses a fixed-size byte sequence that must match the expected value.
func ParseMagic(p []byte, expected []byte) ([]byte, []byte, error) {
	if len(p) < len(expected) {
		return nil, nil, ErrTruncated
	}
	body := p[:len(expected)]
	if string(body) != string(expected) {
		return nil, nil, fmt.Errorf("%w: expected %x, found %x", ErrMagicMismatch, expected, body)
	}
	return body, p[len(expected):], nil
}

// PackMagic returns the expected magic bytes as-is.
func PackMagic(expected []byte) []byte {
	b := make([]byte, len(expected))
	copy(b, expected)
	return b
}

// Tail parses all remaining bytes as the payload.
func ParseTail(p []byte) ([]byte, []byte, error) {
	body := make([]byte, len(p))
	copy(body, p)
	return body, nil, nil
}

// PackTail returns the bytes as-is.
func PackTail(data []byte) []byte {
	b := make([]byte, len(data))
	copy(b, data)
	return b
}

// ─── IPv4 ────────────────────────────────────────────────────────────────────

// IPv4 represents an IP address stored in reversed byte order (as used by the PPPP protocol).
// The string form is the standard dotted-decimal notation.

// ParseIPv4 parses a 4-byte IP address in reversed byte order.
func ParseIPv4(p []byte) (string, []byte, error) {
	if len(p) < 4 {
		return "", nil, ErrTruncated
	}
	// The protocol stores IP bytes in reverse order
	addr := net.IPv4(p[3], p[2], p[1], p[0])
	return addr.String(), p[4:], nil
}

// PackIPv4 packs an IP address string into 4 bytes in reversed byte order.
func PackIPv4(addr string) ([]byte, error) {
	ip := net.ParseIP(addr)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP address: %s", addr)
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return nil, fmt.Errorf("not an IPv4 address: %s", addr)
	}
	// Reverse byte order as per protocol
	return []byte{ip4[3], ip4[2], ip4[1], ip4[0]}, nil
}

// ─── Array Helper ────────────────────────────────────────────────────────────

// ParseArray parses n elements using the provided parse function.
// The parse function takes the remaining bytes and returns the parsed value, the remaining bytes, and an error.
func ParseArray[T any](p []byte, n int, parse func([]byte) (T, []byte, error)) ([]T, []byte, error) {
	result := make([]T, 0, n)
	for i := 0; i < n; i++ {
		val, rest, err := parse(p)
		if err != nil {
			return nil, nil, fmt.Errorf("array element %d: %w", i, err)
		}
		result = append(result, val)
		p = rest
	}
	return result, p, nil
}

// ─── Utility ─────────────────────────────────────────────────────────────────

// EnHex returns the hexadecimal encoding of data as a string.
func EnHex(data []byte) string {
	var sb strings.Builder
	for _, b := range data {
		fmt.Fprintf(&sb, "%02x", b)
	}
	return sb.String()
}

// UnHex decodes a hexadecimal string to bytes.
func UnHex(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, errors.New("hex string must have even length")
	}
	b := make([]byte, len(s)/2)
	for i := 0; i < len(b); i++ {
		hi, ok1 := hexVal(s[i*2])
		lo, ok2 := hexVal(s[i*2+1])
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("invalid hex character at position %d", i*2)
		}
		b[i] = hi<<4 | lo
	}
	return b, nil
}

func hexVal(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	default:
		return 0, false
	}
}
