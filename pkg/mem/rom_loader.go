package mem

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kiltum/zxgo-v3/pkg/archive"
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
//
// The file may also be packed: a ROM set that ships as "48.rom.zip" (or any
// .zip holding a .rom) overrides the embedded image the same way the bare file
// does, so an override does not have to be unpacked by hand first.
func loadROMDescriptor(desc *model.ROMDescriptor, romsDir string) ([]byte, error) {
	var data []byte

	// Try loading from file first.
	if desc.FilePath != "" {
		path := desc.FilePath
		if !filepath.IsAbs(path) {
			path = filepath.Join(romsDir, path)
		}

		fileData, err := ReadROMFile(path)
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

// ReadROMFile reads a ROM override from disk, unpacking a .zip when the file is
// one. A missing ".zip" beside a missing name is still a missing file, so a
// caller's fall-through to the embedded ROM is unchanged.
//
// Exported because the General Sound ROM override is loaded outside this
// package and wants the same "packed or not" behaviour as a bank ROM.
func ReadROMFile(path string) ([]byte, error) {
	if strings.EqualFold(filepath.Ext(path), ".zip") {
		return readZippedROM(path)
	}
	data, err := os.ReadFile(path)
	if err == nil {
		return data, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	// A zipped override may sit beside the bare name it stands in for:
	// roms/48.rom missing, roms/48.rom.zip present.
	zipped, zerr := readZippedROM(path + ".zip")
	if zerr != nil {
		return nil, err // report the bare name: that is what the layout asked for
	}
	return zipped, nil
}

// readZippedROM opens path as a .zip and reads the ROM image inside it (the
// pick is archive.Open's: a ".rom" entry wins, else the first regular file).
func readZippedROM(path string) ([]byte, error) {
	r, _, err := archive.Open(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}
