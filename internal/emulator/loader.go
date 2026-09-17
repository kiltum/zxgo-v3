package emulator

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kiltum/zxgo-v3/pkg/archive"
	"github.com/kiltum/zxgo-v3/pkg/snap"
)

// SnapshotInfo describes the snapshot LoadSnapshotFile applied, so a caller can
// report what it loaded without re-reading the file.
type SnapshotInfo struct {
	Format       string // "SNA" or "Z80"
	Name         string // the file, or the archive entry inside a .zip
	Is128K       bool
	HardwareMode uint8 // Z80 snapshots only; 0 for SNA
	RAMBytes     int
}

// LoadSnapshotFile loads a .sna or .z80 snapshot into the machine, unpacking a
// .zip on the way.
//
// Parsing lives in pkg/snap and applying lives here, so the dispatch between
// them lives here too: the emulator is the only thing that knows which machine
// it is, which is what lets it refuse a snapshot for a machine this is not. The
// format is detected from the name archive.Open reports, so a zipped snapshot is
// recognised by the name it has inside the archive.
func (e *Emulator) LoadSnapshotFile(path string) (SnapshotInfo, error) {
	f, name, err := archive.Open(path)
	if err != nil {
		return SnapshotInfo{}, err
	}
	defer f.Close()

	switch strings.ToLower(filepath.Ext(name)) {
	case ".sna":
		s, err := snap.LoadSNA(f)
		if err != nil {
			return SnapshotInfo{}, fmt.Errorf("%s: %w", name, err)
		}
		if err := e.LoadSNA(s); err != nil {
			return SnapshotInfo{}, err
		}
		// The SNA format carries no machine byte: 128K-ness is implied by the
		// file length, so HardwareMode stays 0 and a banked machine reports as
		// "128k".
		return SnapshotInfo{
			Format:   "SNA",
			Name:     name,
			Is128K:   s.Is128K,
			RAMBytes: len(s.RAM),
		}, nil

	case ".z80":
		z, err := snap.LoadZ80(f)
		if err != nil {
			return SnapshotInfo{}, fmt.Errorf("%s: %w", name, err)
		}
		if err := e.LoadZ80(z); err != nil {
			return SnapshotInfo{}, err
		}
		return SnapshotInfo{
			Format:       "Z80",
			Name:         name,
			Is128K:       z.Is128K,
			HardwareMode: z.HardwareMode,
			RAMBytes:     len(z.RAM),
		}, nil

	default:
		return SnapshotInfo{}, fmt.Errorf("unknown snapshot format %q (supported: .sna .z80)",
			filepath.Ext(name))
	}
}
