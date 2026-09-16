package media

import (
	"os"
	"testing"
)

// TestSectorDataWeakRandomization verifies the weak-bit read path: non-weak
// bytes are stable across reads, while a weak byte reads back random (a fresh
// value on each read), matching real hardware's unstable weak sectors.
func TestSectorDataWeakRandomization(t *testing.T) {
	s := &DiskSector{
		Data: []byte("HELLOWORLD"), // 10 bytes
		Weak: make([]bool, 10),
	}
	s.Weak[2] = true // only byte 2 is weak

	// Non-weak bytes must always equal the stored data.
	a := sectorData(s)
	for i := range s.Data {
		if s.Weak[i] {
			continue
		}
		if a[i] != s.Data[i] {
			t.Fatalf("non-weak byte %d changed: got %q, want %q", i, a[i], s.Data[i])
		}
	}

	// A weak byte must take on more than one value across reads. With 256
	// possibilities and 100 reads, all-identical is effectively impossible.
	seen := map[byte]bool{}
	for i := 0; i < 100; i++ {
		seen[sectorData(s)[2]] = true
	}
	if len(seen) < 2 {
		t.Errorf("weak byte returned only %d distinct value(s) across 100 reads", len(seen))
	}

	// A nil Weak mask means no weak bits: the sector is fully stable.
	stable := sectorData(&DiskSector{Data: []byte("ABC")})
	if string(stable) != "ABC" {
		t.Errorf("nil Weak mask corrupted data: got %q", stable)
	}
}

// TestSectorWeak reports weak-ness by ID.
func TestSectorWeak(t *testing.T) {
	d := newPlus3Disk()
	if d.SectorWeak(0, 0, 0, 0, 1) {
		t.Error("normal sector should not be weak")
	}
	d.Tracks[0].Sectors[0].Weak = []bool{true}
	if !d.SectorWeak(0, 0, 0, 0, 1) {
		t.Error("sector with a weak mask should report weak")
	}
}

// TestDSKLoadDetectsWeakSector loads a real Speedlock-protected DSK (Arcticfox)
// and verifies the weak sector (track 0, head 0, sector 2) is detected: it has a
// non-nil weak mask, and re-reading returns unstable data.
func TestDSKLoadDetectsWeakSector(t *testing.T) {
	path := "../../testdata/Games/[DSK]/Arcticfox (1988)(Electronic Arts)(+3).dsk"
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("Arcticfox DSK not present: %v", err)
	}
	defer f.Close()

	d, err := LoadDSK(f)
	if err != nil {
		t.Fatalf("LoadDSK: %v", err)
	}
	if !d.SectorWeak(0, 0, 0, 0, 2) {
		t.Fatal("Arcticfox sector 2 (track 0, head 0) should be weak, got none")
	}

	if _, ok := d.ReadSectorByID(0, 0, 0, 0, 2); !ok {
		t.Fatal("weak sector not found by ID")
	}
	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		b, _ := d.ReadSectorByID(0, 0, 0, 0, 2)
		seen[string(b)] = true
	}
	if len(seen) < 2 {
		t.Errorf("weak sector read back identically across 20 reads (not unstable)")
	}
}
