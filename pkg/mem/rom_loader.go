package mem

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/rom"
)

// LoadROMLayout loads all ROMs described in a ROMLayout.
// Returns a map of ROM bank index -> ROM data.
// romsDir is the base directory for ROM files (typically "roms/").
func LoadROMLayout(layout *model.ROMLayout, romsDir string) (map[int][]byte, error) {
	if layout == nil {
		return nil, fmt.Errorf("ROM layout is nil")
	}

	roms := make(map[int][]byte)

	for _, desc := range layout.ROMs {
		data, err := loadROMDescriptor(&desc, romsDir)
		if err != nil {
			if desc.Optional {
				// Optional ROM missing is not an error
				continue
			}
			return nil, fmt.Errorf("loading ROM %s: %w", desc.Name, err)
		}

		// Validate size
		expectedSize := desc.SizeKB * 1024
		if len(data) != expectedSize {
			return nil, fmt.Errorf("ROM %s: expected %d bytes, got %d", desc.Name, expectedSize, len(data))
		}

		roms[desc.BankIndex] = data
	}

	return roms, nil
}

// loadROMDescriptor loads a single ROM from file or embedded data.
// FileOffset and SizeKB slicing is applied uniformly regardless of source, so a
// multi-bank ROM stored as one file (or one embedded blob) can feed several banks.
func loadROMDescriptor(desc *model.ROMDescriptor, romsDir string) ([]byte, error) {
	var data []byte

	// Try loading from file first.
	if desc.FilePath != "" {
		path := desc.FilePath
		if !filepath.IsAbs(path) {
			path = filepath.Join(romsDir, path)
		}

		fileData, err := os.ReadFile(path)
		if err == nil {
			data = fileData
		} else if !os.IsNotExist(err) {
			return nil, err // Real error, not just missing file
		}
		// File not found -- fall through to embedded.
	}

	// Try embedded ROM.
	if data == nil && desc.EmbeddedName != "" {
		data = rom.Get(desc.EmbeddedName)
	}

	if data == nil {
		return nil, fmt.Errorf("ROM not found: file=%s, embedded=%s", desc.FilePath, desc.EmbeddedName)
	}

	// Slice out the requested bank when the source is larger than one bank or an
	// explicit offset is given.
	expectedSize := desc.SizeKB * 1024
	if desc.FileOffset > 0 || len(data) > expectedSize {
		if desc.FileOffset >= len(data) {
			return nil, fmt.Errorf("file offset %d exceeds file size %d", desc.FileOffset, len(data))
		}
		end := desc.FileOffset + expectedSize
		if end > len(data) {
			return nil, fmt.Errorf("file offset %d + size %d exceeds file size %d", desc.FileOffset, expectedSize, len(data))
		}
		return data[desc.FileOffset:end], nil
	}
	return data, nil
}
