package snap

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Z80Version identifies the Z80 file format version.
type Z80Version int

const (
	Z80v1 Z80Version = iota // Version 1 (48K, simple structure)
	Z80v2                   // Version 2 (extended headers, etc.)
	Z80v3                   // Version 3 (expanded hardware types)
)

// Z80Header represents the Z80 snapshot header structure following Z80_specs.txt
type Z80Header struct {
	AF    uint16 // A and F registers (bytes 0-1)
	BC    uint16 // B and C registers (bytes 2-3)
	HL    uint16 // H and L registers (bytes 4-5) - V1: HL, V2/V3: HL
	DE    uint16 // D and E registers (bytes 13-14 in V1, bytes 4-5 in V2/V3)
	PC    uint16 // Program counter (bytes 6-7) - V1: PC, V2/V3: 0 (PC at end)
	SP    uint16 // Stack pointer (bytes 8-9) - V1: SP, V2/V3: SP
	I     uint8  // Interrupt register (byte 10)
	R     uint8  // R register (byte 11) - bit 7 not significant in V1
	RBit7 uint8  // Bit 7 of R register (stored in byte 12 bit 0 in V1)
	Flags uint8  // Flags byte (byte 12) - bit 0 = R bit 7, bit 5 = compressed (V1)
	AF_   uint16 // Alternative AF (bytes 16-17 in V1, 11-12 in spec)
	BC_   uint16 // Alternative BC
	DE_   uint16 // Alternative DE
	HL_   uint16 // Alternative HL
	IY    uint16 // Index Y (bytes 23-24)
	IX    uint16 // Index X (bytes 25-26)
	IFF   uint8  // IFF (byte 27) - 0=DI, otherwiseEI
	IFF2  uint8  // IFF2 (byte 28)
	IM    uint8  // Interrupt mode (byte 29) - bits 0-1

	// V2/V3 specific fields
	LatestHardware uint8  // Hardware mode (byte 34 in v2/v3)
	EmuMode        uint8  // Emulation mode
	LatestIFF2     uint8  // IFF2 from extended header
	R_Byte6        uint8  // R register byte 6
	LatestSP       uint16 // Latest SP value
	EmuInfo        uint8  // Emulator info
	EmuData        []byte // Emulator specific data
}

// Z80Snapshot represents a complete ZX Spectrum machine state in Z80 format.
type Z80Snapshot struct {
	Header  Z80Header
	RAM     []byte     // RAM contents (may be compressed)
	Is128K  bool       // True for 128K snapshots
	Version Z80Version // Format version
}

// LoadZ80 reads a ZX Spectrum Z80 snapshot file following Z80_specs.txt.
func LoadZ80(r io.Reader) (*Z80Snapshot, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read Z80 file: %w", err)
	}

	// Check for minimum header size (30 bytes for v1)
	if len(data) < 30 {
		return nil, fmt.Errorf("Z80 file too short: expected at least 30 bytes for header, got %d", len(data))
	}

	// Parse V1 header structure per Z80_specs.txt
	header := Z80Header{
		AF:    binary.LittleEndian.Uint16(data[0:2]),
		BC:    binary.LittleEndian.Uint16(data[2:4]),
		HL:    binary.LittleEndian.Uint16(data[4:6]),
		PC:    binary.LittleEndian.Uint16(data[6:8]),
		SP:    binary.LittleEndian.Uint16(data[8:10]),
		I:     data[10],
		R:     data[11] & 0x7F,
		Flags: data[12],
	}

	// Parse V1 specific fields per Z80_specs.txt
	header.RBit7 = (data[12] & 0x01) << 7               // Bit 0 of byte 12 contains R bit 7
	header.DE = binary.LittleEndian.Uint16(data[13:15]) // bytes 13-14 = DE in V1
	header.BC_ = binary.LittleEndian.Uint16(data[15:17])
	header.DE_ = binary.LittleEndian.Uint16(data[17:19])
	header.HL_ = binary.LittleEndian.Uint16(data[19:21])
	// bytes 21-22: A' and F' are single bytes
	header.AF_ = uint16(data[21])<<8 | uint16(data[22])
	header.IY = binary.LittleEndian.Uint16(data[23:25])
	header.IX = binary.LittleEndian.Uint16(data[25:27])
	header.IFF = data[27]
	header.IFF2 = data[28]
	header.IM = data[29] & 0x03 // Only bits 0-1 are IM

	// Reconstruct full R register
	header.R = header.R | header.RBit7

	// Determine version: V1 vs V2/V3
	// V2/V3: PC at bytes 6-7 is zero, has extended header
	var version Z80Version
	var ramData []byte
	var is128K bool

	if header.PC == 0 && len(data) >= 30 {
		// Likely V2/V3 - check for extended header
		if len(data) >= 32 {
			// Parse additional header block
			extHeaderLen := binary.LittleEndian.Uint16(data[30:32])
			if extHeaderLen >= 23 { // V2 minimum
				version = Z80v2
				header.PC = binary.LittleEndian.Uint16(data[32:34]) // PC from extended header
				header.LatestHardware = data[34]
				header.EmuMode = data[35]
				header.LatestIFF2 = data[36]
				header.R_Byte6 = data[37]
				header.LatestSP = binary.LittleEndian.Uint16(data[38:40])
				header.EmuInfo = data[40]

				// Parse V3 specific fields if present
				if extHeaderLen >= 54 && len(data) >= 86 {
					version = Z80v3
				}

				// Determine if 128K from hardware mode (needed before RAM decompression)
				is128K = (header.LatestHardware >= 3 && header.LatestHardware <= 15)

				// V2/V3 RAM: page-based compression, starts after extended header
				ramStart := 30 + 2 + int(extHeaderLen)
				if ramStart < len(data) {
					// For V2/V3, RAM data is page-based compressed
					// Format: [len_high, len_low, page_num, data...]
					// len = 0xFFFF means uncompressed 16K page
					// Otherwise compressed data needing decompression
					ramData = decompressV2V3Pages(data[ramStart:], is128K)
				}
			} else {
				// V1 format with PC=0 in bytes 6-7 (shouldn't happen in valid files)
				version = Z80v1
			}
		}
	} else {
		// V1 format
		version = Z80v1

		is128K = false
		// Check compression flag (bit 5 of byte 12)
		isCompressed := (header.Flags & 0x20) != 0

		if isCompressed {
			// Decompress V1 format RAM
			compressedRAM := data[30:]
			ramData = decompressV1(compressedRAM)
		} else {
			// Uncompressed RAM starts at byte 30
			ramData = data[30:]
		}
	}

	return &Z80Snapshot{
		Header:  header,
		RAM:     ramData,
		Is128K:  is128K,
		Version: version,
	}, nil
}

// decompressV1 decompresses Z80 v1 format RAM data following Z80_specs.txt
// Compression format: ED ED xx yy = repeat byte yy xx times
// End marker: 00 ED ED 00
// Special case: ED ED 02 ED = two ED bytes
// Single ED is not merged with following compression
func decompressV1(data []byte) []byte {
	var result []byte
	i := 0

	for i < len(data) {
		// Check for end marker: 00 ED ED 00
		if i+3 < len(data) && data[i] == 0x00 &&
			data[i+1] == 0xED && data[i+2] == 0xED && data[i+3] == 0x00 {
			break
		}

		// Check for compression marker: ED ED xx yy
		if i+3 < len(data) && data[i] == 0xED && data[i+1] == 0xED {
			// Special case: ED ED 02 ED = two ED bytes
			if data[i+2] == 0x02 && data[i+3] == 0xED {
				result = append(result, 0xED, 0xED)
				i += 4
				continue
			}

			// Regular compression: ED ED xx yy
			count := int(data[i+2])
			byteToRepeat := data[i+3]

			if count > 0 {
				for j := 0; j < count; j++ {
					result = append(result, byteToRepeat)
				}
			}

			i += 4
			continue
		}

		// Single ED (not followed by ED)
		if data[i] == 0xED && (i+1 >= len(data) || data[i+1] != 0xED) {
			result = append(result, data[i])
			i++
			continue
		}

		// Normal byte
		result = append(result, data[i])
		i++
	}

	return result
}

// decompressV2V3Pages decompresses Z80 V2/V3 format page-based RAM data
// Format: [len_high, len_low, page_num, data...]
// len = 0xFFFF means uncompressed 16K page
// Otherwise compressed data needing decompression using the same ED ED xx yy format
func decompressV2V3Pages(data []byte, is128K bool) []byte {
	var result []byte
	ptr := 0

	// Page mappings for 48K: pages 4,5,8 are used (RAM2, RAM0, RAM5)
	// Page mappings for 128K: pages 3,4,5,6,7,10,11,12,13 are used
	// Following Unreal Speccy implementation patterns

	for ptr < len(data) {
		if ptr+3 > len(data) {
			break
		}

		// Read page header: length (2 bytes) + page number (1 byte)
		length := int(data[ptr]) | int(data[ptr+1])<<8
		pageNum := data[ptr+2]
		ptr += 3

		// Determine destination page size based on model
		pageSize := 16384 // 16K per page for ZX Spectrum

		// For 48K format, only certain pages matter
		validPage := false
		var pageOffset int

		if !is128K {
			// 48K format: pages 4,5,8 following Z80_specs.txt
			switch pageNum {
			case 4:
				validPage = true
				pageOffset = 16384 // Page 4: 8000-bfff -> offset 16384 in RAM 0x4000-0xFFFF
			case 5:
				validPage = true
				pageOffset = 32768 // Page 5: c000-ffff -> offset 32768 in RAM 0x4000-0xFFFF
			case 8:
				validPage = true
				pageOffset = 0 // Page 8: 4000-7fff -> offset 0 in RAM 0x4000-0xFFFF
			}
		} else {
			// 128K format: pages 3-10 map linearly to RAM banks 0-7 (each
			// 16 KB). Verified against testdata/3.z80, which stores pages
			// 3,4,5,6,7,8,9,10.
			if pageNum >= 3 && pageNum <= 10 {
				validPage = true
				pageOffset = int(pageNum-3) * 16384
			}
		}

		if !validPage {
			if length == 0xFFFF {
				ptr += pageSize
			} else {
				ptr += int(length)
			}
			continue
		}

		// Ensure result is large enough
		if pageOffset+pageSize > len(result) {
			newResult := make([]byte, pageOffset+pageSize)
			copy(newResult, result)
			result = newResult
		}

		if length == 0xFFFF {
			// Uncompressed 16K page
			if ptr+pageSize <= len(data) {
				copy(result[pageOffset:], data[ptr:ptr+pageSize])
			}
			ptr += pageSize
		} else {
			// Compressed page
			if ptr+int(length) <= len(data) {
				decompressed := decompressV1(data[ptr : ptr+int(length)])
				copy(result[pageOffset:], decompressed)
			}
			ptr += int(length)
		}
	}

	// Ensure we return exactly the expected size if we have partial data
	if !is128K && len(result) > 49152 {
		return result[:49152]
	}

	return result
}

// SaveZ80 writes a ZX Spectrum Z80 snapshot file.
// This implementation saves in Z80 v1 format (simpler, widely compatible).
func SaveZ80(w io.Writer, snap *Z80Snapshot) error {
	// Prepare header data (30 bytes for v1)
	header := make([]byte, 30)

	// Write basic Z80 v1 header per Z80_specs.txt
	binary.LittleEndian.PutUint16(header[0:2], snap.Header.AF)
	binary.LittleEndian.PutUint16(header[2:4], snap.Header.BC)
	binary.LittleEndian.PutUint16(header[4:6], snap.Header.HL)
	binary.LittleEndian.PutUint16(header[6:8], snap.Header.PC)
	binary.LittleEndian.PutUint16(header[8:10], snap.Header.SP)

	header[10] = snap.Header.I
	header[11] = snap.Header.R & 0x7F // Bit 7 not significant in V1

	// Byte 12: flags - bit 0 = R bit 7, bit 5 = compressed (we use uncompressed)
	rBit7 := uint8(0)
	if (snap.Header.R & 0x80) != 0 {
		rBit7 = 0x01
	}
	header[12] = rBit7 // Uncompressed for simplicity

	binary.LittleEndian.PutUint16(header[13:15], snap.Header.DE) // bytes 13-14 = DE in V1
	binary.LittleEndian.PutUint16(header[15:17], snap.Header.BC_)
	binary.LittleEndian.PutUint16(header[17:19], snap.Header.DE_)
	binary.LittleEndian.PutUint16(header[19:21], snap.Header.HL_)

	// bytes 21-22: A' and F' are single bytes
	header[21] = uint8(snap.Header.AF_ >> 8)
	header[22] = uint8(snap.Header.AF_ & 0xFF)

	binary.LittleEndian.PutUint16(header[23:25], snap.Header.IY)
	binary.LittleEndian.PutUint16(header[25:27], snap.Header.IX)

	header[27] = snap.Header.IFF
	header[28] = snap.Header.IFF2
	header[29] = snap.Header.IM // Only bits 0-1 matter

	// Write header
	if _, err := w.Write(header); err != nil {
		return fmt.Errorf("write Z80 header: %w", err)
	}

	// Write RAM data (uncompressed for simplicity)
	if _, err := w.Write(snap.RAM); err != nil {
		return fmt.Errorf("write Z80 RAM: %w", err)
	}

	return nil
}

// GetRegisterValue returns the value of a named register from the Z80 snapshot.
func (z *Z80Snapshot) GetRegisterValue(name string) (uint16, error) {
	switch name {
	case "A":
		return uint16(z.Header.AF >> 8), nil
	case "F":
		return uint16(z.Header.AF & 0xFF), nil
	case "B":
		return uint16(z.Header.BC >> 8), nil
	case "C":
		return uint16(z.Header.BC & 0xFF), nil
	case "D":
		return uint16(z.Header.DE >> 8), nil
	case "E":
		return uint16(z.Header.DE & 0xFF), nil
	case "H":
		return uint16(z.Header.HL >> 8), nil
	case "L":
		return uint16(z.Header.HL & 0xFF), nil
	case "A'":
		return uint16(z.Header.AF_ >> 8), nil
	case "F'":
		return uint16(z.Header.AF_ & 0xFF), nil
	case "B'":
		return uint16(z.Header.BC_ >> 8), nil
	case "C'":
		return uint16(z.Header.BC_ & 0xFF), nil
	case "D'":
		return uint16(z.Header.DE_ >> 8), nil
	case "E'":
		return uint16(z.Header.DE_ & 0xFF), nil
	case "H'":
		return uint16(z.Header.HL_ >> 8), nil
	case "L'":
		return uint16(z.Header.HL_ & 0xFF), nil
	case "AF":
		return z.Header.AF, nil
	case "BC":
		return z.Header.BC, nil
	case "DE":
		return z.Header.DE, nil
	case "HL":
		return z.Header.HL, nil
	case "AF'":
		return z.Header.AF_, nil
	case "BC'":
		return z.Header.BC_, nil
	case "DE'":
		return z.Header.DE_, nil
	case "HL'":
		return z.Header.HL_, nil
	case "I":
		return uint16(z.Header.I), nil
	case "R":
		return uint16(z.Header.R), nil
	case "IX":
		return z.Header.IX, nil
	case "IY":
		return z.Header.IY, nil
	case "SP":
		return z.Header.SP, nil
	case "PC":
		return z.Header.PC, nil
	default:
		return 0, fmt.Errorf("unknown register: %s", name)
	}
}

// ValidFor48K returns true if the snapshot is valid for 48K ZX Spectrum.
func (z *Z80Snapshot) ValidFor48K() bool {
	return len(z.RAM) == 49152 && !z.Is128K && z.Version == Z80v1
}
