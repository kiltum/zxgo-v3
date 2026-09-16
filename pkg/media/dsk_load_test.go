package media

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadAllDSKFiles loads every .dsk image in the repo testdata directory and
// verifies the parser produces a sane geometry and a readable boot sector. The
// +3 disk set exercises both sector-ID conventions (01-09 and C1-C9) plus
// non-standard track counts (42, 44) and sector counts (10).
func TestLoadAllDSKFiles(t *testing.T) {
	files, err := filepath.Glob("../../testdata/*.dsk")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(files) == 0 {
		t.Skip("no .dsk images in testdata")
	}

	for _, path := range files {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			f, err := os.Open(path)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer f.Close()
			disk, err := LoadDSK(f)
			if err != nil {
				t.Fatalf("LoadDSK: %v", err)
			}

			if disk.TracksPerSide <= 0 {
				t.Errorf("TracksPerSide=%d, want > 0", disk.TracksPerSide)
			}
			if disk.Sides < 1 {
				t.Errorf("Sides=%d, want >= 1", disk.Sides)
			}
			if disk.SectorsPerTrack <= 0 {
				t.Errorf("SectorsPerTrack=%d, want > 0", disk.SectorsPerTrack)
			}
			if disk.SectorSize != 512 {
				t.Errorf("SectorSize=%d, want 512 (+3 DOS)", disk.SectorSize)
			}
			if len(disk.Tracks) != disk.TracksPerSide*disk.Sides {
				t.Errorf("len(Tracks)=%d, want %d", len(disk.Tracks), disk.TracksPerSide*disk.Sides)
			}

			// The boot sector must be readable using the disk's actual first
			// sector ID (01 for some images, C1 for others).
			if len(disk.Tracks) == 0 || len(disk.Tracks[0].Sectors) == 0 {
				t.Fatal("track 0 has no sectors")
			}
			firstID := disk.Tracks[0].Sectors[0].SectorID
			if _, err := disk.ReadSector(0, 0, firstID); err != nil {
				t.Errorf("boot sector (ID %02X) not readable: %v", firstID, err)
			}
		})
	}
}
