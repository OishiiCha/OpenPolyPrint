package types

import "fmt"

// CyclicU16 is a 16-bit unsigned counter with wraparound-aware comparison.
//
// When dealing with 16-bit counters, special care must be taken when
// overflow occurs. For example, consider two increasing counter variables
// x and y. At a point in time, we have:
//
//	x == 0xFFF2
//	y == 0xFFF5
//
// At this time x < y works as expected. However, 12 steps later:
//
//	x == 0xFFFE
//	y == 0x0001
//
// which inverts the result of x < y, even though the counters have
// increased at the same rate.
//
// To handle this, we define a wrap window (default 0x100) in which
// numbers are assumed to have recently wrapped around, such that
// CyclicU16(0xFFFE) < CyclicU16(0x0001) == true.
type CyclicU16 uint16

// DefaultWrap is the default wrap window for cyclic comparison.
const DefaultWrap uint16 = 0x100

// NewCyclicU16 creates a CyclicU16, truncating the value to 16 bits.
func NewCyclicU16(n int) CyclicU16 {
	return CyclicU16(n & 0xFFFF)
}

// Trunc truncates a value to 16 bits.
func CyclicTrunc(n int) uint16 {
	return uint16(n & 0xFFFF)
}

// Add returns a new CyclicU16 with the value added, wrapped to 16 bits.
func (c CyclicU16) Add(k int) CyclicU16 {
	return CyclicU16(int(c) + k)
}

// Sub returns a new CyclicU16 with the value subtracted, wrapped to 16 bits.
func (c CyclicU16) Sub(k int) CyclicU16 {
	return CyclicU16(int(c) - k)
}

// LessThan compares two CyclicU16 values with wraparound awareness,
// using the default wrap window of 0x100.
func (c CyclicU16) LessThan(other CyclicU16) bool {
	return c.lessThan(other, DefaultWrap)
}

// GreaterThan compares two CyclicU16 values with wraparound awareness,
// using the default wrap window of 0x100.
func (c CyclicU16) GreaterThan(other CyclicU16) bool {
	return c.greaterThan(other, DefaultWrap)
}

// LessThanOrEqual returns true if c <= other (with wraparound awareness).
func (c CyclicU16) LessThanOrEqual(other CyclicU16) bool {
	return !c.GreaterThan(other)
}

// GreaterThanOrEqual returns true if c >= other (with wraparound awareness).
func (c CyclicU16) GreaterThanOrEqual(other CyclicU16) bool {
	return !c.LessThan(other)
}

func (c CyclicU16) lessThan(other CyclicU16, wrap uint16) bool {
	// If sign bit differs, take wrap into account
	if (uint16(c)^uint16(other))&0x8000 != 0 {
		return CyclicTrunc(int(c)-int(wrap)) < CyclicTrunc(int(other)-int(wrap))
	}
	return uint16(c) < uint16(other)
}

func (c CyclicU16) greaterThan(other CyclicU16, wrap uint16) bool {
	// If sign bit differs, take wrap into account
	if (uint16(c)^uint16(other))&0x8000 != 0 {
		return CyclicTrunc(int(c)-int(wrap)) > CyclicTrunc(int(other)-int(wrap))
	}
	return uint16(c) > uint16(other)
}

// String returns the hex representation of the cyclic value.
func (c CyclicU16) String() string {
	return fmt.Sprintf("CyclicU16(0x%04x)", uint16(c))
}
