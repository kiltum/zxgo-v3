package mem

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kiltum/zxgo-v3/pkg/model"
)

func TestLoadROMLayout_SingleFile(t *testing.T) {
	// Create temporary ROM files
	tmpDir := t.TempDir()
	romPath := filepath.Join(tmpDir, "test.rom")
	data := make([]byte, 16384)
	for i := range data {
		data[i] = byte(i & 0xFF)
	}
	if err := os.WriteFile(romPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	layout := &model.ROMLayout{
		ROMs: []model.ROMDescriptor{
			{
				Name:      "test-rom",
				BankIndex: 0,
				SizeKB:    16,
				FilePath:  "test.rom",
			},
		},
		DefaultROM: 0,
	}

	roms, err := LoadROMLayout(layout, tmpDir)
	if err != nil {
		t.Fatalf("LoadROMLayout() error = %v", err)
	}

	if len(roms) != 1 {
		t.Errorf("Expected 1 ROM, got %d", len(roms))
	}

	if rom, ok := roms[0]; !ok {
		t.Error("ROM bank 0 not found")
	} else if len(rom) != 16384 {
		t.Errorf("ROM bank 0 size = %d, want 16384", len(rom))
	}
}

func TestLoadROMLayout_MultiFileWithOffset(t *testing.T) {
	// Create a large ROM file with multiple banks
	tmpDir := t.TempDir()
	romPath := filepath.Join(tmpDir, "large.rom")
	data := make([]byte, 65536) // 64KB = 4 banks
	for i := range data {
		data[i] = byte(i >> 8) // Each 256-byte block has same value
	}
	if err := os.WriteFile(romPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	layout := &model.ROMLayout{
		ROMs: []model.ROMDescriptor{
			{
				Name:       "bank0",
				BankIndex:  0,
				SizeKB:     16,
				FilePath:   "large.rom",
				FileOffset: 0,
			},
			{
				Name:       "bank1",
				BankIndex:  1,
				SizeKB:     16,
				FilePath:   "large.rom",
				FileOffset: 16384,
			},
			{
				Name:       "bank2",
				BankIndex:  2,
				SizeKB:     16,
				FilePath:   "large.rom",
				FileOffset: 32768,
			},
		},
		DefaultROM: 0,
	}

	roms, err := LoadROMLayout(layout, tmpDir)
	if err != nil {
		t.Fatalf("LoadROMLayout() error = %v", err)
	}

	if len(roms) != 3 {
		t.Errorf("Expected 3 ROMs, got %d", len(roms))
	}

	// Verify each bank has correct data
	for i := 0; i < 3; i++ {
		rom, ok := roms[i]
		if !ok {
			t.Errorf("ROM bank %d not found", i)
			continue
		}
		if len(rom) != 16384 {
			t.Errorf("ROM bank %d size = %d, want 16384", i, len(rom))
		}
		// Check first byte (should be from different offsets)
		expectedFirst := byte(i * 16384 >> 8)
		if rom[0] != expectedFirst {
			t.Errorf("ROM bank %d first byte = 0x%02X, want 0x%02X", i, rom[0], expectedFirst)
		}
	}
}

func TestLoadROMLayout_EmbeddedFallback(t *testing.T) {
	layout := &model.ROMLayout{
		ROMs: []model.ROMDescriptor{
			{
				Name:         "48k",
				BankIndex:    0,
				SizeKB:       16,
				FilePath:     "nonexistent.rom",
				EmbeddedName: "48k/48.rom",
			},
		},
		DefaultROM: 0,
	}

	roms, err := LoadROMLayout(layout, "/nonexistent")
	if err != nil {
		t.Fatalf("LoadROMLayout() error = %v (should use embedded fallback)", err)
	}

	if len(roms) != 1 {
		t.Errorf("Expected 1 ROM, got %d", len(roms))
	}

	if rom, ok := roms[0]; !ok {
		t.Error("ROM bank 0 not found")
	} else if len(rom) != 16384 {
		t.Errorf("ROM bank 0 size = %d, want 16384", len(rom))
	}
}

func TestLoadROMLayout_OptionalROM(t *testing.T) {
	layout := &model.ROMLayout{
		ROMs: []model.ROMDescriptor{
			{
				Name:      "missing",
				BankIndex: 0,
				SizeKB:    16,
				FilePath:  "nonexistent.rom",
				Optional:  true,
			},
		},
		DefaultROM: 0,
	}

	roms, err := LoadROMLayout(layout, "/nonexistent")
	if err != nil {
		t.Fatalf("LoadROMLayout() error = %v (optional ROM should not error)", err)
	}

	if len(roms) != 0 {
		t.Errorf("Expected 0 ROMs (optional missing), got %d", len(roms))
	}
}

func TestLoadROMLayout_SizeValidation(t *testing.T) {
	tmpDir := t.TempDir()
	romPath := filepath.Join(tmpDir, "wrong-size.rom")
	data := make([]byte, 8192) // Wrong size: 8KB instead of 16KB
	if err := os.WriteFile(romPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	layout := &model.ROMLayout{
		ROMs: []model.ROMDescriptor{
			{
				Name:      "test",
				BankIndex: 0,
				SizeKB:    16,
				FilePath:  "wrong-size.rom",
			},
		},
		DefaultROM: 0,
	}

	_, err := LoadROMLayout(layout, tmpDir)
	if err == nil {
		t.Error("LoadROMLayout() should error on wrong ROM size")
	}
}
