package types

import "encoding/binary"

// ppcsCRC16Table is the lookup table for CRC-16/CCITT-FALSE (poly 0x11021, init 0x0000, no reflection, no xor-out).
var ppcsCRC16Table [256]uint16

func init() {
	const poly = 0x1021
	for i := 0; i < 256; i++ {
		crc := uint16(i) << 8
		for j := 0; j < 8; j++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ poly
			} else {
				crc <<= 1
			}
		}
		ppcsCRC16Table[i] = crc
	}
}

// PpcsCRC16 computes the CRC-16/CCITT checksum used by the PPPP protocol.
// Returns the checksum as 2 bytes in little-endian order (matching the Python implementation).
func PpcsCRC16(data []byte) []byte {
	crc := uint16(0)
	for _, b := range data {
		crc = (crc << 8) ^ ppcsCRC16Table[(crc>>8)^uint16(b)]
	}
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, crc)
	return b
}
