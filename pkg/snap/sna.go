package snap

import (
	"encoding/binary"
	"fmt"
	"io"
)

// SNAHeader represents the standard ZX Spectrum snapshot header.
// The SNA format is well-documented and has a 27-byte header for 48K snapshots.
type SNAHeader struct {
	I      uint8  // Interrupt register
	HL_    uint16 // Alternative HL
	DE_    uint16 // Alternative DE
	BC_    uint16 // Alternative BC
	AF_    uint16 // Alternative AF
	HL     uint16 // Normal HL
	DE     uint16 // Normal DE
	BC     uint16 // Normal BC
	IY     uint16 // Index Y
	IX     uint16 // Index X
	IFF2   uint8  // Interrupt flag (bit 2)
	R      uint8  // R register
	SP     uint16 // Stack pointer
	A      uint8  // Accumulator
	F      uint8  // Flags
	IM     uint8  // Interrupt mode (0, 1, or 2)
	Border uint8  // Border color (0-7)
}

// Snapshot represents a complete ZX Spectrum machine state.
type Snapshot struct {
	Header    SNAHeader
	RAM       []byte   // 48K RAM (49152 bytes, starting at 0x4000)
	Is128K    bool     // True for 128K extended SNA format
	P7FFD     uint8    // 0x7FFD register value for 128K
	TRDOSPage uint8    // TR-DOS page (for +3)
	RAMBanks  [][]byte // Additional RAM banks (for 128K: 5 banks of 16K each)
}

// LoadSNA reads a ZX Spectrum SNA snapshot file.
//
// Standard 48K SNA: 27-byte header + 49152 bytes RAM = 49179 bytes total
//
// Extended 128K SNA: Additional bytes for bank information and extra RAM banks
//
// Format:
//   - Byte 0: I register
//   - Bytes 1-2: HL' (alternate HL, little-endian)
//   - Bytes 3-4: DE' (little-endian)
//   - Bytes 5-6: BC' (little-endian)
//   - Bytes 7-8: AF' (little-endian)
//   - Bytes 9-10: HL (little-endian)
//   - Bytes 11-12: DE (little-endian)
//   - Bytes 13-14: BC (little-endian)
//   - Bytes 15-16: IY (little-endian)
//   - Bytes 17-18: IX (little-endian)
//   - Byte 19: IFF2 (interrupt flag, bit 2)
//   - Byte 20: R register
//   - Bytes 21-22: SP (little-endian)
//   - Byte 23: Combined: (IM << 1) & 0x07 for IM, bits 1-7 for border
//   - Byte 24: A register (accumulator)
//   - Byte 25: F register (flags)
//   - Byte 26: R register: lower 7 bits, bit 7 = (R bits 7)
//   - Bytes 27-49178: 48K RAM starting at 0x4000
func LoadSNA(r io.Reader) (*Snapshot, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read SNA file: %w", err)
	}

	// Check for minimum 48K SNA size
	if len(data) < 49179 {
		return nil, fmt.Errorf("SNA file too short: expected at least 49179 bytes for 48K, got %d", len(data))
	}

	// Parse 27-byte header
	header := SNAHeader{
		I:      data[0],
		HL_:    binary.LittleEndian.Uint16(data[1:3]),
		DE_:    binary.LittleEndian.Uint16(data[3:5]),
		BC_:    binary.LittleEndian.Uint16(data[5:7]),
		AF_:    binary.LittleEndian.Uint16(data[7:9]),
		HL:     binary.LittleEndian.Uint16(data[9:11]),
		DE:     binary.LittleEndian.Uint16(data[11:13]),
		BC:     binary.LittleEndian.Uint16(data[13:15]),
		IY:     binary.LittleEndian.Uint16(data[15:17]),
		IX:     binary.LittleEndian.Uint16(data[17:19]),
		IFF2:   data[19],
		R:      data[20],
		SP:     binary.LittleEndian.Uint16(data[21:23]),
		IM:     (data[23] >> 3) & 0x03,
		Border: data[23] & 0x07,
		A:      data[24],
		F:      data[25],
	}

	// Reconstruct full R register (combine bits 0-6 from byte 20, bit 7 from byte 26)
	header.R = (header.R & 0x7F) | (data[26] & 0x80)

	// Extract RAM (49152 bytes, starting at offset 27)
	ram := make([]byte, 49152)
	copy(ram, data[27:27+49152])

	snap := &Snapshot{
		Header: header,
		RAM:    ram,
		Is128K: len(data) > 49179, // Extended format if larger
	}

	// Parse 128K extended format if present
	if snap.Is128K {
		if len(data) < 49180 {
			return nil, fmt.Errorf("128K SNA file too short: expected at least 49180 bytes, got %d", len(data))
		}

		// Parse 128K-specific fields
		snap.P7FFD = data[49179]     // 0x7FFD register
		snap.TRDOSPage = data[49180] // TR-DOS page

		// Parse additional RAM banks (banks 5, 2, 0, 1, 3, 4, 6, 7 typically)
		// Standard 128K SNA includes banks 5, 2, 0, 1, 3 (5 banks of 16K each = 81920 bytes)
		expected128KSize := 49179 + 2 + 81920 // Header + 2 bytes + 5 banks
		if len(data) >= expected128KSize {
			offset := 49181 // After header + 2 control bytes
			snap.RAMBanks = make([][]byte, 5)
			for i := 0; i < 5; i++ {
				snap.RAMBanks[i] = make([]byte, 16384)
				copy(snap.RAMBanks[i], data[offset+i*16384:offset+(i+1)*16384])
			}
		}
	}

	return snap, nil
}

// SaveSNA writes a ZX Spectrum SNA snapshot file.
//
// For 48K: writes 49179 bytes (27-byte header + 48K RAM)
// For 128K: writes extended format with bank information
func SaveSNA(w io.Writer, snap *Snapshot) error {
	header := &snap.Header

	// Write 27-byte header
	buf := make([]byte, 27)
	buf[0] = header.I
	binary.LittleEndian.PutUint16(buf[1:3], header.HL_)
	binary.LittleEndian.PutUint16(buf[3:5], header.DE_)
	binary.LittleEndian.PutUint16(buf[5:7], header.BC_)
	binary.LittleEndian.PutUint16(buf[7:9], header.AF_)
	binary.LittleEndian.PutUint16(buf[9:11], header.HL)
	binary.LittleEndian.PutUint16(buf[11:13], header.DE)
	binary.LittleEndian.PutUint16(buf[13:15], header.BC)
	binary.LittleEndian.PutUint16(buf[15:17], header.IY)
	binary.LittleEndian.PutUint16(buf[17:19], header.IX)
	buf[19] = header.IFF2
	buf[20] = header.R & 0x7F // Lower 7 bits of R
	binary.LittleEndian.PutUint16(buf[21:23], header.SP)
	buf[23] = (header.IM << 3) | (header.Border & 0x07) // IM (bits 3-4) + border (bits 0-2)
	buf[24] = header.A
	buf[25] = header.F
	buf[26] = (header.R & 0x80) // Bit 7 of R

	if _, err := w.Write(buf); err != nil {
		return fmt.Errorf("write SNA header: %w", err)
	}

	// Write 48K RAM (starting at 0x4000)
	if len(snap.RAM) != 49152 {
		// Pad or truncate RAM to 48K
		paddedRAM := make([]byte, 49152)
		copy(paddedRAM, snap.RAM)
		snap.RAM = paddedRAM
	}
	if _, err := w.Write(snap.RAM); err != nil {
		return fmt.Errorf("write SNA RAM: %w", err)
	}

	// Write 128K extension if present
	if snap.Is128K {
		// Write control bytes
		ext := []byte{snap.P7FFD, snap.TRDOSPage}
		if _, err := w.Write(ext); err != nil {
			return fmt.Errorf("write SNA 128K control bytes: %w", err)
		}

		// Write additional RAM banks
		for _, bank := range snap.RAMBanks {
			if _, err := w.Write(bank); err != nil {
				return fmt.Errorf("write SNA RAM bank: %w", err)
			}
		}
	}

	return nil
}

// GetRegisterValue returns the value of a named register from the snapshot.
//
// Supported register names:
//   - "A", "F", "B", "C", "D", "E", "H", "L" (8-bit registers)
//   - "AF", "BC", "DE", "HL"
//   - "A'", "F'", "B'", "C'", "D'", "E'", "H'", "L'"
//   - "AF'", "BC'", "DE'", "HL'"
//   - "I", "R", "IX", "IY", "SP", "PC"
func (s *Snapshot) GetRegisterValue(name string) (uint16, error) {
	h := &s.Header

	switch name {
	case "A":
		return uint16(h.A), nil
	case "F":
		return uint16(h.F), nil
	case "B":
		return uint16(h.BC >> 8), nil
	case "C":
		return uint16(h.BC & 0xFF), nil
	case "D":
		return uint16(h.DE >> 8), nil
	case "E":
		return uint16(h.DE & 0xFF), nil
	case "H":
		return uint16(h.HL >> 8), nil
	case "L":
		return uint16(h.HL & 0xFF), nil
	case "AF":
		return uint16(h.A)<<8 | uint16(h.F), nil
	case "BC":
		return h.BC, nil
	case "DE":
		return h.DE, nil
	case "HL":
		return h.HL, nil
	case "A'":
		return uint16(h.AF_ >> 8), nil
	case "F'":
		return uint16(h.AF_ & 0xFF), nil
	case "B'":
		return uint16(h.BC_ >> 8), nil
	case "C'":
		return uint16(h.BC_ & 0xFF), nil
	case "D'":
		return uint16(h.DE_ >> 8), nil
	case "E'":
		return uint16(h.DE_ & 0xFF), nil
	case "H'":
		return uint16(h.HL_ >> 8), nil
	case "L'":
		return uint16(h.HL_ & 0xFF), nil
	case "AF'":
		return h.AF_, nil
	case "BC'":
		return h.BC_, nil
	case "DE'":
		return h.DE_, nil
	case "HL'":
		return h.HL_, nil
	case "I":
		return uint16(h.I), nil
	case "R":
		return uint16(h.R), nil
	case "IX":
		return h.IX, nil
	case "IY":
		return h.IY, nil
	case "SP":
		return h.SP, nil
	case "PC":
		// PC is stored at stack top in SNA format
		return s.retrievePC(), nil
	case "IM":
		return uint16(h.IM), nil
	case "Border":
		return uint16(h.Border), nil
	default:
		return 0, fmt.Errorf("unknown register: %s (use A, F, B, C, D, E, H, L, AF, BC, DE, HL, A', F', etc.)", name)
	}
}

// retrievePC reads the PC value from the stack (stored at SP in SNA format)
func (s *Snapshot) retrievePC() uint16 {
	if len(s.RAM) < 49152 {
		return 0 // Invalid RAM size
	}

	// SP points to address on stack where PC is stored
	sp := int(s.Header.SP)
	if sp < 0x4000 || sp >= 0x10000 {
		return 0 // Invalid SP
	}

	offset := sp - 0x4000 // Convert to RAM offset
	if offset < 0 || offset+1 >= len(s.RAM) {
		return 0 // Invalid SP range
	}

	return binary.LittleEndian.Uint16(s.RAM[offset : offset+2])
}

// SetRegisterValue sets the value of a named register in the snapshot.
// Note: SNA format is read-only for some registers (like PC stored on stack).
func (s *Snapshot) SetRegisterValue(name string, value uint16) error {
	h := &s.Header

	switch name {
	case "A":
		if value > 0xFF {
			return fmt.Errorf("value %d too large for 8-bit register A", value)
		}
		h.A = uint8(value)
	case "F":
		if value > 0xFF {
			return fmt.Errorf("value %d too large for 8-bit register F", value)
		}
		h.F = uint8(value)
	case "B":
		if value > 0xFF {
			return fmt.Errorf("value %d too large for 8-bit register B", value)
		}
		h.BC = (h.BC & 0x00FF) | (value << 8)
	case "C":
		if value > 0xFF {
			return fmt.Errorf("value %d too large for 8-bit register C", value)
		}
		h.BC = (h.BC & 0xFF00) | uint16(value)
	case "D":
		if value > 0xFF {
			return fmt.Errorf("value %d too large for 8-bit register D", value)
		}
		h.DE = (h.DE & 0x00FF) | (value << 8)
	case "E":
		if value > 0xFF {
			return fmt.Errorf("value %d too large for 8-bit register E", value)
		}
		h.DE = (h.DE & 0xFF00) | uint16(value)
	case "H":
		if value > 0xFF {
			return fmt.Errorf("value %d too large for 8-bit register H", value)
		}
		h.HL = (h.HL & 0x00FF) | (value << 8)
	case "L":
		if value > 0xFF {
			return fmt.Errorf("value %d too large for 8-bit register L", value)
		}
		h.HL = (h.HL & 0xFF00) | uint16(value)
	case "AF":
		h.A = uint8(value >> 8)
		h.F = uint8(value & 0xFF)
	case "BC":
		h.BC = value
	case "DE":
		h.DE = value
	case "HL":
		h.HL = value
	case "I":
		if value > 0xFF {
			return fmt.Errorf("value %d too large for 8-bit register I", value)
		}
		h.I = uint8(value)
	case "R":
		if value > 0xFF {
			return fmt.Errorf("value %d too large for 8-bit register R", value)
		}
		h.R = uint8(value)
	case "IX":
		h.IX = value
	case "IY":
		h.IY = value
	case "SP":
		h.SP = value
	case "IM":
		if value > 0x02 {
			return fmt.Errorf("invalid IM value %d (must be 0, 1, or 2)", value)
		}
		h.IM = uint8(value)
	case "Border":
		if value > 0x07 {
			return fmt.Errorf("invalid border value %d (must be 0-7)", value)
		}
		h.Border = uint8(value)
	default:
		return fmt.Errorf("cannot set register %s in SNA format (read-only or unsupported)", name)
	}

	return nil
}

// GetRAM returns the RAM contents at a specific address.
//
// Supports 48K addressing (0x4000-0xFFFF) and 128K banked RAM.
// For 128K, only displays currently paged RAM unless bank is specified.
func (s *Snapshot) GetRAM(addr uint16) (byte, error) {
	if addr < 0x4000 || addr > 0xFFFF {
		return 0, fmt.Errorf("invalid RAM address: 0x%04X", addr)
	}

	offset := int(addr) - 0x4000
	if offset >= len(s.RAM) {
		return 0, fmt.Errorf("RAM address out of range: 0x%04X", addr)
	}

	return s.RAM[offset], nil
}

// SetRAM sets the RAM contents at a specific address.
func (s *Snapshot) SetRAM(addr uint16, value byte) error {
	if addr < 0x4000 || addr > 0xFFFF {
		return fmt.Errorf("invalid RAM address: 0x%04X", addr)
	}

	offset := int(addr) - 0x4000
	if offset >= len(s.RAM) {
		return fmt.Errorf("RAM address out of range: 0x%04X", addr)
	}

	s.RAM[offset] = value
	return nil
}

// ValidFor48K returns true if the snapshot is valid for 48K ZX Spectrum.
func (s *Snapshot) ValidFor48K() bool {
	return !s.Is128K && len(s.RAM) == 49152
}

// ValidFor128K returns true if the snapshot is valid for 128K ZX Spectrum.
func (s *Snapshot) ValidFor128K() bool {
	return s.Is128K && len(s.RAM) == 49152 && len(s.RAMBanks) == 5
}
