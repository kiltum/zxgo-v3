package mem

import (
	"testing"

	"github.com/kiltum/zxgo-v3/pkg/mem/banking"
	"github.com/kiltum/zxgo-v3/pkg/model"
)

func TestGenericMapper_128K(t *testing.T) {
	// Create a simple ROM layout for testing
	layout := &model.ROMLayout{
		ROMs: []model.ROMDescriptor{
			{
				Name:         "rom0",
				BankIndex:    0,
				SizeKB:       16,
				EmbeddedName: "128k/128-0.rom",
				Activation: model.ROMActivation{
					PortSelect: &model.PortSelect{
						Port:  0x7FFD,
						Mask:  0x10,
						Value: 0x00,
					},
				},
			},
			{
				Name:         "rom1",
				BankIndex:    1,
				SizeKB:       16,
				EmbeddedName: "128k/128-1.rom",
				Activation: model.ROMActivation{
					PortSelect: &model.PortSelect{
						Port:  0x7FFD,
						Mask:  0x10,
						Value: 0x10,
					},
				},
			},
		},
		DefaultROM: 0,
	}

	cfg := model.Spectrum128K
	scheme := banking.NewStandard128(layout)
	mapper := NewGenericMapper(cfg, scheme)

	// Load test ROM data
	romData, err := LoadROMLayout(layout, "")
	if err != nil {
		t.Fatalf("LoadROMLayout() error = %v", err)
	}

	for bankIdx, data := range romData {
		if err := mapper.LoadROM(bankIdx, data); err != nil {
			t.Fatalf("LoadROM(%d) error = %v", bankIdx, err)
		}
	}

	// Test ROM read (slot 0: ROM)
	val := mapper.ReadByte(0x0000)
	if val == 0 {
		t.Error("ROM bank 0 should contain data, got 0x00")
	}

	// Test RAM read (slot 1: bank 5)
	mapper.WriteByte(0x4000, 0x42)
	val = mapper.ReadByte(0x4000)
	if val != 0x42 {
		t.Errorf("RAM bank 5 at 0x4000 = 0x%02X, want 0x42", val)
	}

	// Test RAM read (slot 2: bank 2)
	mapper.WriteByte(0x8000, 0x84)
	val = mapper.ReadByte(0x8000)
	if val != 0x84 {
		t.Errorf("RAM bank 2 at 0x8000 = 0x%02X, want 0x84", val)
	}

	// Test paging: select bank 3 for slot 3
	mapper.WritePort(0x7FFD, 0x03) // D0-D2 = 3
	mapper.WriteByte(0xC000, 0xC3)
	val = mapper.ReadByte(0xC000)
	if val != 0xC3 {
		t.Errorf("RAM bank 3 at 0xC000 = 0x%02X, want 0xC3", val)
	}
}

func TestGenericMapper_ROMSwitch(t *testing.T) {
	layout := &model.ROMLayout{
		ROMs: []model.ROMDescriptor{
			{
				Name:         "rom0",
				BankIndex:    0,
				SizeKB:       16,
				EmbeddedName: "128k/128-0.rom",
				Activation: model.ROMActivation{
					PortSelect: &model.PortSelect{
						Port:  0x7FFD,
						Mask:  0x10,
						Value: 0x00,
					},
				},
			},
			{
				Name:         "rom1",
				BankIndex:    1,
				SizeKB:       16,
				EmbeddedName: "128k/128-1.rom",
				Activation: model.ROMActivation{
					PortSelect: &model.PortSelect{
						Port:  0x7FFD,
						Mask:  0x10,
						Value: 0x10,
					},
				},
			},
		},
		DefaultROM: 0,
	}

	cfg := model.Spectrum128K
	scheme := banking.NewStandard128(layout)
	mapper := NewGenericMapper(cfg, scheme)

	// Load ROMs
	romData, err := LoadROMLayout(layout, "")
	if err != nil {
		t.Fatalf("LoadROMLayout() error = %v", err)
	}
	for bankIdx, data := range romData {
		mapper.LoadROM(bankIdx, data)
	}

	// Write distinct values to each ROM for testing
	mapper.SetROMWritable(true)
	mapper.WriteByte(0x0000, 0xAA) // ROM 0
	mapper.WritePort(0x7FFD, 0x10) // Switch to ROM 1
	mapper.WriteByte(0x0000, 0xBB) // ROM 1
	mapper.SetROMWritable(false)

	// Switch back to ROM 0
	mapper.WritePort(0x7FFD, 0x00)
	val := mapper.ReadByte(0x0000)
	if val != 0xAA {
		t.Errorf("ROM 0 at 0x0000 = 0x%02X, want 0xAA", val)
	}

	// Switch to ROM 1
	mapper.WritePort(0x7FFD, 0x10)
	val = mapper.ReadByte(0x0000)
	if val != 0xBB {
		t.Errorf("ROM 1 at 0x0000 = 0x%02X, want 0xBB", val)
	}
}

func TestGenericMapper_Snapshot(t *testing.T) {
	layout := &model.ROMLayout{
		ROMs: []model.ROMDescriptor{
			{
				Name:         "rom0",
				BankIndex:    0,
				SizeKB:       16,
				EmbeddedName: "128k/128-0.rom",
			},
		},
		DefaultROM: 0,
	}

	cfg := model.Spectrum128K
	scheme := banking.NewStandard128(layout)
	mapper := NewGenericMapper(cfg, scheme)

	// Write test data to RAM
	mapper.WriteByte(0x4000, 0x12)
	mapper.WriteByte(0x8000, 0x34)
	mapper.WriteByte(0xC000, 0x56)

	// Take snapshot
	snapshot := mapper.Snapshot()
	if len(snapshot) != 8 {
		t.Errorf("Snapshot should have 8 banks, got %d", len(snapshot))
	}

	// Modify RAM
	mapper.WriteByte(0x4000, 0xFF)
	mapper.WriteByte(0x8000, 0xFF)
	mapper.WriteByte(0xC000, 0xFF)

	// Restore snapshot
	mapper.RestoreSnapshot(snapshot)

	// Verify restored data
	if val := mapper.ReadByte(0x4000); val != 0x12 {
		t.Errorf("After restore, RAM at 0x4000 = 0x%02X, want 0x12", val)
	}
	if val := mapper.ReadByte(0x8000); val != 0x34 {
		t.Errorf("After restore, RAM at 0x8000 = 0x%02X, want 0x34", val)
	}
	if val := mapper.ReadByte(0xC000); val != 0x56 {
		t.Errorf("After restore, RAM at 0xC000 = 0x%02X, want 0x56", val)
	}
}

func TestNewMapperWithLayout(t *testing.T) {
	// Test that NewMapperWithLayout creates a working mapper
	mapper, err := NewMapperWithLayout(model.Spectrum128K, model.Spectrum128K_ROMLayout, "")
	if err != nil {
		t.Fatalf("NewMapperWithLayout() error = %v", err)
	}

	// Verify it's a MemoryMapper
	if mapper == nil {
		t.Fatal("NewMapperWithLayout() returned nil mapper")
	}

	// Test basic read/write
	mapper.WriteByte(0x4000, 0x99)
	val := mapper.ReadByte(0x4000)
	if val != 0x99 {
		t.Errorf("RAM write/read = 0x%02X, want 0x99", val)
	}

	// Test ROM is loaded
	val = mapper.ReadByte(0x0000)
	if val == 0 {
		t.Error("ROM should be loaded with data")
	}
}

// TestPlus3SpecialModeWrite verifies that in the +3 special paging mode
// (0x1FFD bit 0), the RAM mapped over 0x0000-0x3FFF is writable. WriteByte must
// not treat that region as ROM.
func TestPlus3SpecialModeWrite(t *testing.T) {
	mapper := NewGenericMapper(model.Spectrum2A3, banking.NewPlus3(nil))

	// Enter special paging mode: 0x1FFD bit 0 = 1.
	mapper.WritePort(0x1FFD, 0x01)

	// In special mode 0x0000-0x3FFF is RAM (bank 0). A write must stick.
	mapper.WriteByte(0x0000, 0xAB)
	if got := mapper.ReadByte(0x0000); got != 0xAB {
		t.Errorf("special-mode write to 0x0000 = %02X, want AB", got)
	}

	// 0xC000 maps to RAM bank 3 in special mode (which=0 -> {0,1,2,3}).
	mapper.WriteByte(0xC000, 0xCD)
	if got := mapper.ReadByte(0xC000); got != 0xCD {
		t.Errorf("special-mode write to 0xC000 = %02X, want CD", got)
	}
}

// TestPlus3ULAScreenBankInSpecialMode verifies the ULA keeps reading the screen
// from bank 5 (the hardware screen bank) even when the +3 special paging mode
// remaps 0x4000-0x7FFF to a different bank for the CPU.
func TestPlus3ULAScreenBankInSpecialMode(t *testing.T) {
	mapper := NewGenericMapper(model.Spectrum2A3, banking.NewPlus3(nil))

	bank5 := make([]byte, 16384)
	bank5[0] = 0x5A
	mapper.SetBank(5, bank5)
	bank1 := make([]byte, 16384)
	bank1[0] = 0x1B
	mapper.SetBank(1, bank1)

	// Special mode, which=0 -> {0,1,2,3}: the CPU sees bank 1 at 0x4000.
	mapper.WritePort(0x1FFD, 0x01)

	if got := mapper.ReadByte(0x4000); got != 0x1B {
		t.Errorf("CPU read 0x4000 in special mode = %02X, want 1B (bank 1)", got)
	}
	if got := mapper.ULAReadByte(0x4000); got != 0x5A {
		t.Errorf("ULA read 0x4000 in special mode = %02X, want 5A (bank 5)", got)
	}
}

// TestContendedBanks verifies contention is a property of the *bank*, not the
// address: on the 128K odd banks are contended wherever they are paged (so an
// odd bank in 0xC000 is contended), and on the +3 banks 4-7 are contended.
func TestContendedBanks(t *testing.T) {
	// 128K: odd banks contended (Fuse spec128.c).
	m := NewGenericMapper(model.Spectrum128K, banking.NewStandard128(nil))
	if !m.IsContended(0x4000) { // bank 5, screen
		t.Error("128K: 0x4000 (bank 5) should be contended")
	}
	if m.IsContended(0x8000) { // bank 2
		t.Error("128K: 0x8000 (bank 2) should not be contended")
	}
	if m.IsContended(0xC000) { // bank 0 default
		t.Error("128K: 0xC000 (bank 0) should not be contended")
	}
	m.WritePort(0x7FFD, 0x03) // page odd bank 3 into 0xC000
	if !m.IsContended(0xC000) {
		t.Error("128K: 0xC000 with bank 3 (odd) should be contended")
	}
	m.WritePort(0x7FFD, 0x04) // page even bank 4 into 0xC000
	if m.IsContended(0xC000) {
		t.Error("128K: 0xC000 with bank 4 (even) should not be contended")
	}

	// +3: banks 4-7 contended (Fuse specplus3.c).
	p := NewGenericMapper(model.Spectrum2A3, banking.NewPlus3(nil))
	if !p.IsContended(0x4000) { // bank 5
		t.Error("+3: 0x4000 (bank 5) should be contended")
	}
	if p.IsContended(0x8000) { // bank 2
		t.Error("+3: 0x8000 (bank 2) should not be contended")
	}
	if p.IsContended(0xC000) { // bank 0 default
		t.Error("+3: 0xC000 (bank 0) should not be contended")
	}
	p.WritePort(0x7FFD, 0x06) // page bank 6 into 0xC000
	if !p.IsContended(0xC000) {
		t.Error("+3: 0xC000 with bank 6 should be contended")
	}
}

// TestPentagon512Banks verifies the Pentagon 512 has 32 RAM banks and that the
// 5-bit page decode (bits 0-2 plus bits 6-7 shifted to bits 3-4) pages any of
// them into 0xC000 via the standard 0x7FFD port.
func TestPentagon512Banks(t *testing.T) {
	cfg := model.Pentagon512
	scheme := banking.NewPentagon512(nil)
	m := NewGenericMapper(cfg, scheme)

	if got := scheme.NumRAMBanks(); got != 32 {
		t.Fatalf("NumRAMBanks = %d, want 32", got)
	}

	// 0xC3 = 0b11000011: bits 0-2 = 3, bits 6-7 = 11 -> page 3 | (0xC0>>3) = 27.
	m.WritePort(0x7FFD, 0xC3)
	m.WriteByte(0xC000, 0x42)
	if got := m.ReadByte(0xC000); got != 0x42 {
		t.Errorf("bank 27 at 0xC000 = 0x%02X, want 0x42", got)
	}

	// A page under 8 (bits 6-7 clear) behaves like the standard 128K page.
	m.WritePort(0x7FFD, 0x05)
	m.WriteByte(0xC000, 0x24)
	if got := m.ReadByte(0xC000); got != 0x24 {
		t.Errorf("bank 5 at 0xC000 = 0x%02X, want 0x24", got)
	}
}
