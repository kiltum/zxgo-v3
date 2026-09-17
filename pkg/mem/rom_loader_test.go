package mem

import (
	"archive/zip"
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

// romBytes returns a deterministic 16K ROM image.
func romBytes() []byte {
	data := make([]byte, 16384)
	for i := range data {
		data[i] = byte(i & 0xFF)
	}
	return data
}

// singleROMLayout is a layout with one 16K ROM at bank 0, named "test.rom".
func singleROMLayout() *model.ROMLayout {
	return &model.ROMLayout{
		ROMs: []model.ROMDescriptor{
			{Name: "test-rom", BankIndex: 0, SizeKB: 16, FilePath: "test.rom"},
		},
		DefaultROM: 0,
	}
}

// TestLoadROMLayout_ZippedOverride: a ROM override that ships packed overrides
// the embedded image exactly as the bare file does.
func TestLoadROMLayout_ZippedOverride(t *testing.T) {
	tmpDir := t.TempDir()
	writeROMZip(t, filepath.Join(tmpDir, "test.rom.zip"), "test.rom", romBytes())

	roms, err := LoadROMLayout(singleROMLayout(), tmpDir)
	if err != nil {
		t.Fatalf("LoadROMLayout() error = %v", err)
	}
	if got := len(roms[0]); got != 16384 {
		t.Fatalf("ROM bank 0 size = %d, want 16384", got)
	}
	if roms[0][0] != 0 || roms[0][255] != 0xFF {
		t.Error("zipped ROM contents do not match the override")
	}
}

// TestLoadROMLayout_ZipNamedAsTheROM: "roms/48.rom" spelled as a .zip path
// directly ("roms/48.rom.zip" is found for FilePath "48.rom.zip" too).
func TestLoadROMLayout_ZipNamedAsTheROM(t *testing.T) {
	tmpDir := t.TempDir()
	writeROMZip(t, filepath.Join(tmpDir, "test.rom.zip"), "test.rom", romBytes())

	layout := singleROMLayout()
	layout.ROMs[0].FilePath = "test.rom.zip"

	roms, err := LoadROMLayout(layout, tmpDir)
	if err != nil {
		t.Fatalf("LoadROMLayout() error = %v", err)
	}
	if got := len(roms[0]); got != 16384 {
		t.Fatalf("ROM bank 0 size = %d, want 16384", got)
	}
}

// TestLoadROMLayout_ZippedOverrideBeatenByPlainFile: the bare file still wins
// when both are present, so a stale zip cannot shadow a deliberate override.
func TestLoadROMLayout_ZippedOverrideBeatenByPlainFile(t *testing.T) {
	tmpDir := t.TempDir()
	plain := romBytes()
	plain[0] = 0xAB
	if err := os.WriteFile(filepath.Join(tmpDir, "test.rom"), plain, 0644); err != nil {
		t.Fatal(err)
	}
	zipped := romBytes()
	zipped[0] = 0xCD
	writeROMZip(t, filepath.Join(tmpDir, "test.rom.zip"), "test.rom", zipped)

	roms, err := LoadROMLayout(singleROMLayout(), tmpDir)
	if err != nil {
		t.Fatalf("LoadROMLayout() error = %v", err)
	}
	if roms[0][0] != 0xAB {
		t.Errorf("ROM byte 0 = 0x%02X, want 0xAB from the bare file", roms[0][0])
	}
}

// TestLoadROMLayout_MissingZipFallsBackToEmbedded: no ROM file and no zip is
// still the embedded fallback, not an error.
func TestLoadROMLayout_MissingZipFallsBackToEmbedded(t *testing.T) {
	layout := &model.ROMLayout{
		ROMs: []model.ROMDescriptor{
			{Name: "48k", BankIndex: 0, SizeKB: 16, FilePath: "missing.rom", EmbeddedName: "48k/48.rom"},
		},
		DefaultROM: 0,
	}
	roms, err := LoadROMLayout(layout, t.TempDir())
	if err != nil {
		t.Fatalf("LoadROMLayout() error = %v, want the embedded fallback", err)
	}
	if len(roms) != 1 {
		t.Fatalf("got %d ROMs, want 1", len(roms))
	}
}

// writeROMZip packs data as entry inside a zip at path.
func writeROMZip(t *testing.T, path, entry string, data []byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	w, err := zw.Create(entry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}
