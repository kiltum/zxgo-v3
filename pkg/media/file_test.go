package media

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeZip builds an archive in a temp dir holding the given entries and
// returns its path.
func writeZip(t *testing.T, name string, entries map[string][]byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for entry, data := range entries {
		w, err := zw.Create(entry)
		if err != nil {
			t.Fatalf("create entry %s: %v", entry, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("write entry %s: %v", entry, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return path
}

// readFile reads a testdata file, skipping the test if it is absent so the suite
// still runs in a trimmed checkout.
func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("testdata %s not available: %v", path, err)
	}
	return data
}

// TestLoadTapeFileRealZips loads the zipped tapes that ship with releases and
// checks each one against its unpacked twin where there is one.
//
// The entries are as the releases actually ship them: two of these archives
// carry the tape beside a readme, a screenshot and a pokes file, and one carries
// both a 48k and a 128k tape -- the pick takes the tape, and the first tape when
// there are two.
func TestLoadTapeFileRealZips(t *testing.T) {
	zipped := []struct{ zip, format string }{
		{"../../testdata/batty.tap.zip", "TAP"},
		{"../../testdata/Exolon.tzx.zip", "TZX"},
		{"../../testdata/Elite48.tap.zip", "TAP"},
		{"../../testdata/Elite128.tap.zip", "TAP"},
		{"../../testdata/Elite.tzx.zip", "TZX"},
		{"../../testdata/Exolon.tap.zip", "TAP"},
	}

	for _, tc := range zipped {
		tc := tc
		t.Run(filepath.Base(tc.zip), func(t *testing.T) {
			if _, err := os.Stat(tc.zip); err != nil {
				t.Skipf("testdata %s not available", tc.zip)
			}

			tape, err := LoadTapeFile(tc.zip)
			if err != nil {
				t.Fatalf("LoadTapeFile(%s): %v", tc.zip, err)
			}
			if tape.Format != tc.format {
				t.Errorf("format = %s, want %s", tape.Format, tc.format)
			}
			if len(tape.Pulses) == 0 {
				t.Error("no pulses decoded")
			}
			// The tape reports the name inside the archive, not the .zip it
			// arrived in: that is the name the format detection keyed on.
			if ext := filepath.Ext(tape.FileName); !strings.EqualFold(ext, "."+strings.ToLower(tc.format)) {
				t.Errorf("FileName = %q, want a .%s entry", tape.FileName, strings.ToLower(tc.format))
			}
			if filepath.Ext(tape.FileName) == ".zip" {
				t.Errorf("FileName = %q, want the entry name, not the archive", tape.FileName)
			}

			// An unpacked twin, if the repo has one, must decode identically.
			twin := strings.TrimSuffix(tc.zip, ".zip")
			if _, err := os.Stat(twin); err != nil {
				return
			}
			plain, err := LoadTapeFile(twin)
			if err != nil {
				t.Fatalf("LoadTapeFile(%s): %v", twin, err)
			}
			if len(plain.Pulses) != len(tape.Pulses) || len(plain.Blocks) != len(tape.Blocks) {
				t.Errorf("zipped tape differs from plain: %d/%d pulses, %d/%d blocks",
					len(tape.Pulses), len(plain.Pulses), len(tape.Blocks), len(plain.Blocks))
			}
		})
	}
}

// TestLoadTapeFileZipIgnoresReadme: a release that ships a readme beside the
// tape must still load the tape.
func TestLoadTapeFileZipIgnoresReadme(t *testing.T) {
	tap := readFile(t, "../../testdata/batty.tap")

	path := writeZip(t, "batty.zip", map[string][]byte{
		"readme.txt": []byte("Thanks for playing."),
		"batty.tap":  tap,
	})

	tape, err := LoadTapeFile(path)
	if err != nil {
		t.Fatalf("LoadTapeFile: %v", err)
	}
	if tape.Format != "TAP" {
		t.Errorf("format = %s, want TAP", tape.Format)
	}
}

// TestLoadDiskFileZip covers each disk format we can decode, packed. The image
// is built by saving the plain file back out, so this exercises the real loader
// rather than a fixture.
func TestLoadDiskFileZip(t *testing.T) {
	cases := []struct {
		plain string
		inner string
		want  DiskType
	}{
		{"../../testdata/demo.trd", "demo.trd", DiskTypeTRD},
		{"../../testdata/1.scl", "1.scl", DiskTypeTRD},
		{"../../testdata/Alien Storm (1991)(U.S. Gold)(+3).dsk", "game.dsk", DiskTypeDSK},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.inner, func(t *testing.T) {
			data := readFile(t, tc.plain)
			path := writeZip(t, "disk.zip", map[string][]byte{tc.inner: data})

			disk, err := LoadDiskFile(path)
			if err != nil {
				t.Fatalf("LoadDiskFile: %v", err)
			}
			if disk.Type != tc.want {
				t.Errorf("type = %s, want %s", disk.Type, tc.want)
			}
			if len(disk.Tracks) == 0 {
				t.Error("no tracks decoded")
			}

			// The same bytes unpacked must give the same geometry.
			plain, err := LoadDiskFile(tc.plain)
			if err != nil {
				t.Fatalf("LoadDiskFile(%s): %v", tc.plain, err)
			}
			if len(plain.Tracks) != len(disk.Tracks) || plain.SectorSize != disk.SectorSize {
				t.Errorf("zipped disk differs from plain: %d/%d tracks, %d/%d sector size",
					len(disk.Tracks), len(plain.Tracks), disk.SectorSize, plain.SectorSize)
			}
		})
	}
}

// TestLoadDiskFilePlainStillWorks: the unpacked path is the common case and must
// keep loading.
func TestLoadDiskFilePlainStillWorks(t *testing.T) {
	path := "../../testdata/demo.trd"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("testdata %s not available", path)
	}
	disk, err := LoadDiskFile(path)
	if err != nil {
		t.Fatalf("LoadDiskFile: %v", err)
	}
	if disk.Type != DiskTypeTRD {
		t.Errorf("type = %s, want TRD", disk.Type)
	}
}

// TestLoadFileUnknownFormatInZip: the error names the format inside the archive,
// not the ".zip" that carried it, which is what makes it actionable.
func TestLoadFileUnknownFormatInZip(t *testing.T) {
	path := writeZip(t, "odd.zip", map[string][]byte{"game.xyz": []byte("???")})

	_, err := LoadDiskFile(path)
	if err == nil {
		t.Fatal("expected an error for an unknown disk format")
	}
	if !strings.Contains(err.Error(), ".xyz") {
		t.Errorf("error %q should name the inner format", err)
	}

	_, err = LoadTapeFile(path)
	if err == nil {
		t.Fatal("expected an error for an unknown tape format")
	}
	if !strings.Contains(err.Error(), ".xyz") {
		t.Errorf("error %q should name the inner format", err)
	}
}

// TestLoadFileMissing: a missing path reports the open failure rather than an
// empty image.
func TestLoadFileMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.tap")
	if _, err := LoadTapeFile(missing); err == nil {
		t.Fatal("expected an error for a missing tape")
	}
	if _, err := LoadDiskFile(missing); err == nil {
		t.Fatal("expected an error for a missing disk")
	}
}

// TestLoadDiskFileZipWithDirectoryEntry: archives written by desktop tools carry
// a directory entry first; the pick must skip it.
func TestLoadDiskFileZipWithDirectoryEntry(t *testing.T) {
	data := readFile(t, "../../testdata/demo.trd")

	dir := t.TempDir()
	path := filepath.Join(dir, "game.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	zw := zip.NewWriter(f)
	if _, err := zw.Create("game/"); err != nil {
		t.Fatalf("create dir: %v", err)
	}
	w, err := zw.Create("game/demo.trd")
	if err != nil {
		t.Fatalf("create entry: %v", err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}

	disk, err := LoadDiskFile(path)
	if err != nil {
		t.Fatalf("LoadDiskFile: %v", err)
	}
	if len(disk.Tracks) == 0 {
		t.Error("no tracks decoded")
	}
}
