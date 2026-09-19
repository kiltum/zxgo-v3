package snap

import (
	"bytes"
	"testing"
)

// TestLoadSNAHeaderMinimal tests loading a minimal valid SNA header
func TestLoadSNAHeaderMinimal(t *testing.T) {
	// Create a minimal valid 48K SNA file (27 bytes header + some RAM)
	data := make([]byte, 49179)

	// Set some register values
	data[0] = 0x3F                     // I register
	data[1] = 0x01                     // HL' low
	data[2] = 0x00                     // HL' high
	binaryWrite16(data[1:3], 0x0001)   // HL'
	binaryWrite16(data[3:5], 0x0002)   // DE'
	binaryWrite16(data[5:7], 0x0003)   // BC'
	binaryWrite16(data[7:9], 0x0004)   // AF'
	binaryWrite16(data[9:11], 0x0005)  // HL
	binaryWrite16(data[11:13], 0x0006) // DE
	binaryWrite16(data[13:15], 0x0007) // BC
	binaryWrite16(data[15:17], 0x0008) // IY
	binaryWrite16(data[17:19], 0x0009) // IX
	data[19] = 0x04                    // IFF2 (bit 2 = 1)
	data[20] = 0x7F                    // R register
	binaryWrite16(data[21:23], 0xE000) // SP
	data[23] = 0x01                    // IM=0, Border=1
	data[24] = 0x01                    // A register
	data[25] = 0x44                    // F register
	data[26] = 0x00                    // R bit 7

	r := bytes.NewReader(data)
	snap, err := LoadSNA(r)

	if err != nil {
		t.Fatalf("LoadSNA failed: %v", err)
	}

	if snap.Header.I != 0x3F {
		t.Errorf("Expected I=0x3F, got 0x%02X", snap.Header.I)
	}

	if snap.Header.HL_ != 0x0001 {
		t.Errorf("Expected HL'=0x0001, got 0x%04X", snap.Header.HL_)
	}

	if snap.Header.SP != 0xE000 {
		t.Errorf("Expected SP=0xE000, got 0x%04X", snap.Header.SP)
	}

	if snap.Header.A != 0x01 {
		t.Errorf("Expected A=0x01, got 0x%02X", snap.Header.A)
	}

	if snap.Header.Border != 0x01 {
		t.Errorf("Expected Border=0x01, got 0x%02X", snap.Header.Border)
	}

	if snap.Is128K {
		t.Error("Expected 48K snapshot, got 128K flag set")
	}
}

// TestLoadSNATooShort tests error handling for too-short files
func TestLoadSNATooShort(t *testing.T) {
	data := make([]byte, 100) // Too short for valid SNA
	r := bytes.NewReader(data)

	_, err := LoadSNA(r)
	if err == nil {
		t.Error("Expected error for too-short SNA file")
	}

	if !containsSubstring(err.Error(), "too short") {
		t.Errorf("Expected 'too short' error, got: %v", err)
	}
}

// TestSNARoundTrip tests save/load round-trip
func TestSNARoundTrip(t *testing.T) {
	// Create a snapshot
	original := &Snapshot{
		Header: SNAHeader{
			I:      0x3F,
			HL_:    0x1234,
			DE_:    0x5678,
			BC_:    0x9ABC,
			AF_:    0xDEF0,
			HL:     0x1111,
			DE:     0x2222,
			BC:     0x3333,
			IY:     0x4444,
			IX:     0x5555,
			IFF2:   0x04,
			R:      0x7F,
			SP:     0xD000,
			A:      0x01,
			F:      0x44,
			IM:     0x01,
			Border: 0x02,
		},
		RAM:    make([]byte, 49152),
		Is128K: false,
	}

	// Fill RAM with a pattern
	for i := range original.RAM {
		original.RAM[i] = byte(i % 256)
	}

	// Save it
	var buf bytes.Buffer
	err := SaveSNA(&buf, original)
	if err != nil {
		t.Fatalf("SaveSNA failed: %v", err)
	}

	// Load it back
	r := bytes.NewReader(buf.Bytes())
	loaded, err := LoadSNA(r)
	if err != nil {
		t.Fatalf("LoadSNA failed: %v", err)
	}

	// Compare headers
	if loaded.Header.I != original.Header.I {
		t.Errorf("I register mismatch: got 0x%02X, expected 0x%02X",
			loaded.Header.I, original.Header.I)
	}

	if loaded.Header.HL != original.Header.HL {
		t.Errorf("HL register mismatch: got 0x%04X, expected 0x%04X",
			loaded.Header.HL, original.Header.HL)
	}

	if loaded.Header.SP != original.Header.SP {
		t.Errorf("SP register mismatch: got 0x%04X, expected 0x%04X",
			loaded.Header.SP, original.Header.SP)
	}

	if loaded.Header.A != original.Header.A {
		t.Errorf("A register mismatch: got 0x%02X, expected 0x%02X",
			loaded.Header.A, original.Header.A)
	}

	if loaded.Header.Border != original.Header.Border {
		t.Errorf("Border mismatch: got 0x%02X, expected 0x%02X",
			loaded.Header.Border, original.Header.Border)
	}

	// Compare RAM
	if !bytes.Equal(loaded.RAM, original.RAM) {
		t.Error("RAM data mismatch after round-trip")
		for i := 0; i < len(loaded.RAM); i++ {
			if loaded.RAM[i] != original.RAM[i] {
				t.Logf("First mismatch at offset %d: got 0x%02X, expected 0x%02X",
					i, loaded.RAM[i], original.RAM[i])
				break
			}
		}
	}
}

// TestGetRegisterValue tests register value retrieval
func TestGetRegisterValue(t *testing.T) {
	snap := &Snapshot{
		Header: SNAHeader{
			A:      0x12,
			F:      0x34,
			HL:     0x1234,
			BC:     0x5678,
			DE:     0x9ABC,
			HL_:    0x1111,
			DE_:    0x2222,
			BC_:    0x3333,
			AF_:    0x4444,
			I:      0x3F,
			R:      0x7F,
			IX:     0x5555,
			IY:     0x6666,
			SP:     0xE000,
			IM:     0x01,
			Border: 0x02,
		},
		RAM: make([]byte, 49152),
	}

	// Set up PC on stack
	ramOffset := int(snap.Header.SP) - 0x4000
	binaryWrite16(snap.RAM[ramOffset:ramOffset+2], 0x8000)

	tests := []struct {
		name      string
		register  string
		expected  uint16
		shouldErr bool
	}{
		{"A", "A", 0x12, false},
		{"F", "F", 0x34, false},
		{"B", "B", 0x56, false},
		{"C", "C", 0x78, false},
		{"D", "D", 0x9A, false},
		{"E", "E", 0xBC, false},
		{"H", "H", 0x12, false},
		{"L", "L", 0x34, false},
		{"AF", "AF", 0x1234, false},
		{"BC", "BC", 0x5678, false},
		{"DE", "DE", 0x9ABC, false},
		{"HL", "HL", 0x1234, false},
		{"A'", "A'", 0x44, false},
		{"AF'", "AF'", 0x4444, false},
		{"BC'", "BC'", 0x3333, false},
		{"I", "I", 0x3F, false},
		{"R", "R", 0x7F, false},
		{"IX", "IX", 0x5555, false},
		{"IY", "IY", 0x6666, false},
		{"SP", "SP", 0xE000, false},
		{"PC", "PC", 0x8000, false},
		{"Invalid", "Z", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := snap.GetRegisterValue(tt.register)
			if tt.shouldErr {
				if err == nil {
					t.Errorf("Expected error for register %s, got value %d", tt.register, got)
				}
			} else {
				if err != nil {
					t.Errorf("GetRegisterValue(%s) failed: %v", tt.register, err)
				}
				if got != tt.expected {
					t.Errorf("GetRegisterValue(%s) = 0x%04X, expected 0x%04X",
						tt.register, got, tt.expected)
				}
			}
		})
	}
}

// TestSetRegisterValue tests register value setting
func TestSetRegisterValue(t *testing.T) {
	snap := &Snapshot{
		Header: SNAHeader{
			A:      0x12,
			F:      0x34,
			HL:     0x1234,
			BC:     0x5678,
			DE:     0x9ABC,
			I:      0x3F,
			R:      0x7F,
			IX:     0x5555,
			IY:     0x6666,
			SP:     0xE000,
			IM:     0x01,
			Border: 0x02,
		},
		RAM: make([]byte, 49152),
	}

	tests := []struct {
		name      string
		register  string
		value     uint16
		expected  uint16
		shouldErr bool
	}{
		{"Set A", "A", 0xFF, 0xFF, false},
		{"Set F", "F", 0x55, 0x55, false},
		{"Set B", "B", 0xAA, 0xAA, false}, // B becomes high byte of BC, but we read B back
		{"Set C", "C", 0xBB, 0xBB, false}, // C becomes low byte of BC, but we read C back
		{"Set H", "H", 0xCC, 0xCC, false}, // H becomes high byte of HL, but we read H back
		{"Set L", "L", 0xDD, 0xDD, false}, // L becomes low byte of HL, but we read L back
		{"Set AF", "AF", 0x1234, 0x1234, false},
		{"Set BC", "BC", 0x9ABC, 0x9ABC, false},
		{"Set DE", "DE", 0x1234, 0x1234, false},
		{"Set HL", "HL", 0x5678, 0x5678, false},
		{"Set I", "I", 0x11, 0x11, false},
		{"Set R", "R", 0x22, 0x22, false},
		{"Set IX", "IX", 0x7777, 0x7777, false},
		{"Set IY", "IY", 0x8888, 0x8888, false},
		{"Set SP", "SP", 0xF000, 0xF000, false},
		{"Set IM", "IM", 0x02, 0x02, false},
		{"Set Border", "Border", 0x07, 0x07, false},
		{"Invalid value", "A", 0x100, 0, true},
		{"Invalid register", "PC", 0x8000, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := snap.SetRegisterValue(tt.register, tt.value)

			if tt.shouldErr {
				if err == nil {
					t.Errorf("Expected error for SetRegisterValue(%s, %d)", tt.register, tt.value)
				}
			} else {
				if err != nil {
					t.Errorf("SetRegisterValue(%s, %d) failed: %v", tt.register, tt.value, err)
				}
			}

			// Check expected value by reading it back
			got, err := snap.GetRegisterValue(tt.register)
			if !tt.shouldErr && err != nil {
				t.Errorf("GetRegisterValue(%s) failed after SetRegisterValue: %v", tt.register, err)
			}
			if !tt.shouldErr && got != tt.expected {
				t.Errorf("After SetRegisterValue(%s, 0x%04X): got 0x%04X, expected 0x%04X",
					tt.register, tt.value, got, tt.expected)
			}
		})
	}
}

// TestGetRAMSetRAM tests RAM read/write operations
func TestGetRAMSetRAM(t *testing.T) {
	snap := &Snapshot{
		Header: SNAHeader{},
		RAM:    make([]byte, 49152),
		Is128K: false,
	}

	// Test valid addresses
	tests := []struct {
		addr  uint16
		value byte
	}{
		{0x4000, 0xAA},
		{0x8000, 0xBB},
		{0xFFFF, 0xCC},
		{0x5A5A, 0xDD},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			err := snap.SetRAM(tt.addr, tt.value)
			if err != nil {
				t.Errorf("SetRAM(0x%04X) failed: %v", tt.addr, err)
			}

			got, err := snap.GetRAM(tt.addr)
			if err != nil {
				t.Errorf("GetRAM(0x%04X) failed: %v", tt.addr, err)
			}
			if got != tt.value {
				t.Errorf("GetRAM(0x%04X) = 0x%02X, expected 0x%02X", tt.addr, got, tt.value)
			}
		})
	}

	// Test invalid addresses
	invalidAddrs := []uint16{0x0000, 0x3FFF}
	for _, addr := range invalidAddrs {
		_, err := snap.GetRAM(addr)
		if err == nil {
			t.Errorf("Expected error for GetRAM(0x%04X)", addr)
		}

		err = snap.SetRAM(addr, 0x00)
		if err == nil {
			t.Errorf("Expected error for SetRAM(0x%04X)", addr)
		}
	}
}

// TestValidFor48K tests 48K snapshot validation
func TestValidFor48K(t *testing.T) {
	valid48K := &Snapshot{
		RAM:    make([]byte, 49152),
		Is128K: false,
	}

	if !valid48K.ValidFor48K() {
		t.Error("Expected ValidFor48K() to return true")
	}

	invalid48K := &Snapshot{
		RAM:    make([]byte, 1000),
		Is128K: false,
	}

	if invalid48K.ValidFor48K() {
		t.Error("Expected ValidFor48K() to return false for wrong RAM size")
	}

	invalid128K := &Snapshot{
		RAM:    make([]byte, 49152),
		Is128K: true,
	}

	if invalid128K.ValidFor48K() {
		t.Error("Expected ValidFor48K() to return false for 128K snapshot")
	}
}

// Helper functions

func binaryWrite16(data []byte, value uint16) {
	data[0] = byte(value)
	data[1] = byte(value >> 8)
}

func containsSubstring(s, substring string) bool {
	return len(s) >= len(substring) && (s == substring || len(s) > len(substring) && containsSubstringHelper(s, substring))
}

func containsSubstringHelper(s, substring string) bool {
	for i := 0; i <= len(s)-len(substring); i++ {
		if s[i:i+len(substring)] == substring {
			return true
		}
	}
	return false
}
