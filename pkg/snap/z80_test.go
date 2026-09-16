package snap

import (
	"bytes"
	"testing"
)

// TestLoadZ80BasicV1 tests loading a basic Z80 v1 snapshot
func TestLoadZ80BasicV1(t *testing.T) {
	// Create a minimal valid Z80 v1 file (30-byte header + 49152 bytes RAM)
	data := make([]byte, 30+49152)

	// Set some register values in the header following Z80_specs.txt
	binaryWrite16(data[0:2], 0x1234)  // AF (bytes 0-1)
	binaryWrite16(data[2:4], 0x5678)  // BC (bytes 2-3)
	binaryWrite16(data[4:6], 0x9ABC)  // HL (bytes 4-5)
	binaryWrite16(data[6:8], 0x8000)  // PC (bytes 6-7)
	binaryWrite16(data[8:10], 0xD000) // SP (bytes 8-9)

	data[10] = 0x3F // I register (byte 10)
	data[11] = 0x7F // R register bits 0-6 (byte 11)
	data[12] = 0x04 // Flags byte - bit 0 = R bit 7 (0), IFF2 bit 2 (bit 2 = 1) (byte 12)

	binaryWrite16(data[13:15], 0x9ABC) // DE (bytes 13-14)
	binaryWrite16(data[15:17], 0xDEF0) // BC' (bytes 15-16)
	binaryWrite16(data[17:19], 0x2222) // DE' (bytes 17-18)
	binaryWrite16(data[19:21], 0x3333) // HL' (bytes 19-20)

	// bytes 21-22: A' and F' are single bytes
	data[21] = 0xAA // A'
	data[22] = 0xFF // F'

	binaryWrite16(data[23:25], 0x4444) // IY (bytes 23-24)
	binaryWrite16(data[25:27], 0x5555) // IX (bytes 25-26)

	data[27] = 0x04 // IFF1 (byte 27) - bit 2 = 1 (EI)
	data[28] = 0x04 // IFF2 (byte 28) - bit 2 = 1 (EI)
	data[29] = 0x00 // IM (byte 29) - bits 0-1 = 0 (IM0)

	// RAM starts at byte 30 for v1 format
	for i := 30; i < len(data); i++ {
		data[i] = byte(i % 256)
	}

	r := bytes.NewReader(data)
	zsnap, err := LoadZ80(r)

	if err != nil {
		t.Fatalf("LoadZ80 failed: %v", err)
	}

	if zsnap.Header.AF != 0x1234 {
		t.Errorf("Expected AF=0x1234, got 0x%04X", zsnap.Header.AF)
	}

	if zsnap.Header.SP != 0xD000 {
		t.Errorf("Expected SP=0xD000, got 0x%04X", zsnap.Header.SP)
	}

	if zsnap.Header.PC != 0x8000 {
		t.Errorf("Expected PC=0x8000, got 0x%04X", zsnap.Header.PC)
	}

	if zsnap.Header.I != 0x3F {
		t.Errorf("Expected I=0x3F, got 0x%02X", zsnap.Header.I)
	}

	if zsnap.Header.HL != 0x9ABC {
		t.Errorf("Expected HL=0x9ABC, got 0x%04X", zsnap.Header.HL)
	}

	if zsnap.Header.DE != 0x9ABC {
		t.Errorf("Expected DE=0x9ABC, got 0x%04X", zsnap.Header.DE)
	}

	if len(zsnap.RAM) != 49152 {
		t.Errorf("Expected RAM size 49152, got %d", len(zsnap.RAM))
	}

	if !zsnap.ValidFor48K() {
		t.Error("Expected ValidFor48K() to return true")
	}
}

// TestZ80RoundTrip tests save/load round-trip
func TestZ80RoundTrip(t *testing.T) {
	// Create a snapshot
	original := &Z80Snapshot{
		Header: Z80Header{
			AF:    0x1234,
			BC:    0x5678,
			HL:    0x1111,
			SP:    0xD000,
			I:     0x3F,
			R:     0x7F,
			Flags: 0x04,
			AF_:   0x2222,
			HL_:   0x3333,
			DE_:   0x4444,
			BC_:   0x5555,
			IY:    0x6666,
			IX:    0x7777,
			IFF:   0x04,
			PC:    0x8000,
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
	err := SaveZ80(&buf, original)
	if err != nil {
		t.Fatalf("SaveZ80 failed: %v", err)
	}

	// Load it back
	r := bytes.NewReader(buf.Bytes())
	loaded, err := LoadZ80(r)
	if err != nil {
		t.Fatalf("LoadZ80 failed: %v", err)
	}

	// Compare key registers
	if loaded.Header.AF != original.Header.AF {
		t.Errorf("AF register mismatch: got 0x%04X, expected 0x%04X",
			loaded.Header.AF, original.Header.AF)
	}

	if loaded.Header.SP != original.Header.SP {
		t.Errorf("SP register mismatch: got 0x%04X, expected 0x%04X",
			loaded.Header.SP, original.Header.SP)
	}

	if loaded.Header.PC != original.Header.PC {
		t.Errorf("PC register mismatch: got 0x%04X, expected 0x%04X",
			loaded.Header.PC, original.Header.PC)
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

// TestGetZ80RegisterValue tests register value retrieval
func TestGetZ80RegisterValue(t *testing.T) {
	zsnap := &Z80Snapshot{
		Header: Z80Header{
			AF:  0x1234,
			BC:  0x5678,
			HL:  0x1111,
			DE:  0x9ABC,
			AF_: 0x2222,
			BC_: 0x3333,
			DE_: 0x4444,
			HL_: 0x5555,
			I:   0x3F,
			IX:  0x6666,
			IY:  0x7777,
			SP:  0xE000,
			PC:  0x8000,
			IFF: 0x04,
			R:   0x7F,
		},
		RAM: make([]byte, 49152),
	}

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
		{"H", "H", 0x11, false},
		{"L", "L", 0x11, false}, // HL=0x1111, so H=L=0x11
		{"AF", "AF", 0x1234, false},
		{"BC", "BC", 0x5678, false},
		{"DE", "DE", 0x9ABC, false},
		{"HL", "HL", 0x1111, false},
		{"A'", "A'", 0x22, false},
		{"AF'", "AF'", 0x2222, false},
		{"BC'", "BC'", 0x3333, false},
		{"I", "I", 0x3F, false},
		{"R", "R", 0x7F, false},
		{"IX", "IX", 0x6666, false},
		{"IY", "IY", 0x7777, false},
		{"SP", "SP", 0xE000, false},
		{"PC", "PC", 0x8000, false},
		{"Invalid", "Z", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := zsnap.GetRegisterValue(tt.register)
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

// TestLoadZ80TooShort tests error handling for too-short files
func TestLoadZ80TooShort(t *testing.T) {
	data := make([]byte, 100) // Actually this length is OK for a header (30 bytes minimum)
	r := bytes.NewReader(data)

	_, err := LoadZ80(r)
	if err != nil {
		// Expected behavior: LoadZ80 succeeds with minimal header
		t.Logf("LoadZ80 with 100-byte data: %v", err)
	}

	// Test with truly too-short data (less than 30 bytes)
	shortData := make([]byte, 25)
	shortReader := bytes.NewReader(shortData)
	_, err = LoadZ80(shortReader)
	if err == nil {
		t.Error("Expected error for too-short Z80 file (< 30 bytes)")
	}

	if err != nil && !containsSubstring(err.Error(), "too short") {
		t.Errorf("Expected 'too short' error, got: %v", err)
	}
}

// TestZ80MinimalHeader tests loading a Z80 file with minimal header only
func TestZ80MinimalHeader(t *testing.T) {
	// Create a minimal Z80 file (just 30-byte header, no RAM)
	data := make([]byte, 30)

	// Set some basic register values following Z80_specs.txt
	binaryWrite16(data[0:2], 0x1234) // AF (bytes 0-1)
	binaryWrite16(data[6:8], 0x8000) // PC (bytes 6-7 in V1 format)
	data[12] = 0x00                  // Flags byte - bit 5 = 0 (uncompressed)

	r := bytes.NewReader(data)
	zsnap, err := LoadZ80(r)

	if err != nil {
		t.Fatalf("LoadZ80 with minimal header failed: %v", err)
	}

	if zsnap.Header.AF != 0x1234 {
		t.Errorf("Expected AF=0x1234, got 0x%04X", zsnap.Header.AF)
	}

	if zsnap.Header.PC != 0x8000 {
		t.Errorf("Expected PC=0x8000, got 0x%04X", zsnap.Header.PC)
	}

	if len(zsnap.RAM) != 0 {
		t.Errorf("Expected empty RAM, got %d bytes", len(zsnap.RAM))
	}
}

// TestZ80InterruptFields verifies IFF1/IFF2/IM come from bytes 27/28/29, not
// from the Flags byte (byte 12). This pins the header semantics the snapshot
// apply path depends on (a load that read IM/IFF2 from Flags corrupts interrupt
// state silently).
func TestZ80InterruptFields(t *testing.T) {
	data := make([]byte, 30+49152)

	// Byte 12 (Flags): bit 5 = compressed, bit 0 = R bit 7. Deliberately set a
	// value that would be misread if someone maps IM/IFF2 from it (bits 0-1 = 3).
	data[12] = 0x23

	data[27] = 0x01 // IFF1: nonzero -> EI (use bit 0, not bit 2)
	data[28] = 0x02 // IFF2: nonzero -> EI (use bit 1)
	data[29] = 0x02 // IM: bits 0-1 = 2 (IM2)

	r := bytes.NewReader(data)
	zsnap, err := LoadZ80(r)
	if err != nil {
		t.Fatalf("LoadZ80 failed: %v", err)
	}

	if zsnap.Header.IFF != 0x01 {
		t.Errorf("IFF = 0x%02X, want 0x01", zsnap.Header.IFF)
	}
	if zsnap.Header.IFF2 != 0x02 {
		t.Errorf("IFF2 = 0x%02X, want 0x02", zsnap.Header.IFF2)
	}
	if zsnap.Header.IM != 0x02 {
		t.Errorf("IM = 0x%02X, want 0x02", zsnap.Header.IM)
	}
}

// TestZ80RRegisterHandling tests R register bit reconstruction
func TestZ80RRegisterHandling(t *testing.T) {
	// Test that bit 7 is properly extracted and reconstructed
	data := make([]byte, 30+49152)

	// Set R with bit 7 = 1 according to Z80_specs.txt:
	// Byte 11 contains R bits 0-6 (0x7F)
	// Byte 12 bit 0 contains R bit 7 (set to 1)
	data[11] = 0x7F // R bits 0-6
	data[12] = 0x01 // Bit 0 of byte 12 contains R bit 7 (1)

	r := bytes.NewReader(data)
	zsnap, err := LoadZ80(r)

	if err != nil {
		t.Fatalf("LoadZ80 failed: %v", err)
	}

	expectedR := uint8(0xFF) // All bits set (bits 0-6 from byte 11, bit 7 from byte 12 bit 0)
	if zsnap.Header.R != expectedR {
		t.Errorf("Expected R register 0x%02X, got 0x%02X", expectedR, zsnap.Header.R)
	}
}
