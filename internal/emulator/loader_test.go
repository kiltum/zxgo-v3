package emulator

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/snap"
	"github.com/kiltum/zxgo-v3/pkg/sound"
)

// zipBytes wraps data in a one-entry .zip under entry and returns its path.
func zipBytes(t *testing.T, entry string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "snap.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	w, err := zw.Create(entry)
	if err != nil {
		t.Fatalf("create entry: %v", err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return path
}

// TestLoadSnapshotFileZippedSNA: a snapshot delivered inside a .zip loads
// through the same call as the bare file, and reports the entry it came from.
func TestLoadSnapshotFileZippedSNA(t *testing.T) {
	src := New(model.Spectrum48K, &sound.NullOutput{})
	banked(t, src)
	first := saveBytes(t, src)

	path := zipBytes(t, "game.sna", first)

	dst := New(model.Spectrum48K, &sound.NullOutput{})
	info, err := dst.LoadSnapshotFile(path)
	if err != nil {
		t.Fatalf("LoadSnapshotFile: %v", err)
	}
	if info.Format != "SNA" {
		t.Errorf("format = %s, want SNA", info.Format)
	}
	if info.Name != "game.sna" {
		t.Errorf("name = %q, want the entry name game.sna", info.Name)
	}
	if info.Is128K {
		t.Error("a 48K snapshot reported as 128K")
	}
	if info.RAMBytes != 49152 {
		t.Errorf("RAM bytes = %d, want 49152", info.RAMBytes)
	}

	if second := saveBytes(t, dst); !bytes.Equal(first, second) {
		t.Error("zipped snapshot did not reproduce the machine byte for byte")
	}
}

// TestLoadSnapshotFilePlainSNA: the unpacked path is the common case and must
// keep loading, reporting the path as the name.
func TestLoadSnapshotFilePlainSNA(t *testing.T) {
	src := New(model.Spectrum48K, &sound.NullOutput{})
	banked(t, src)

	path := filepath.Join(t.TempDir(), "game.sna")
	if err := os.WriteFile(path, saveBytes(t, src), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	dst := New(model.Spectrum48K, &sound.NullOutput{})
	info, err := dst.LoadSnapshotFile(path)
	if err != nil {
		t.Fatalf("LoadSnapshotFile: %v", err)
	}
	if info.Name != path {
		t.Errorf("name = %q, want the path %q", info.Name, path)
	}
	if info.Format != "SNA" {
		t.Errorf("format = %s, want SNA", info.Format)
	}
}

// TestLoadSnapshotFileRealZ80Zip loads a release snapshot that ships zipped
// beside a screenshot, which the pick has to see past.
func TestLoadSnapshotFileRealZ80Zip(t *testing.T) {
	path := "../../testdata/Tai-Pan128.z80.zip"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("testdata %s not available", path)
	}

	e := New(model.Spectrum128K, &sound.NullOutput{})
	info, err := e.LoadSnapshotFile(path)
	if err != nil {
		t.Fatalf("LoadSnapshotFile: %v", err)
	}
	if info.Format != "Z80" {
		t.Errorf("format = %s, want Z80", info.Format)
	}
	if !strings.EqualFold(filepath.Ext(info.Name), ".z80") {
		t.Errorf("name = %q, want a .z80 entry", info.Name)
	}
	if !info.Is128K {
		t.Error("a 128K snapshot reported as 48K")
	}
}

// TestLoadSnapshotFileZ80InterruptState loads a .z80 whose interrupt state is
// deliberately unambiguous and checks what the machine ends up with.
//
// The parser was pinned years before the apply path was: `pkg/snap`'s
// `TestZ80InterruptFields` checks that IFF1/IFF2/IM come from bytes 27/28/29, while
// the emulator read them from byte 12 (the R/compression byte), so every loaded
// .z80 ran with interrupts disabled and IM 0 - and no test noticed, because no test
// loaded a Z80 file into a machine. Byte 12 below holds a value that would look
// plausible if it were misread (0x23: bits 0-1 = 3, bit 2 = 0), so a regression
// cannot pass by accident.
func TestLoadSnapshotFileZ80InterruptState(t *testing.T) {
	data := make([]byte, 30+49152)
	data[12] = 0x23 // R bit 7 clear, not compressed, and the byte that must NOT be read
	data[27] = 0x01 // IFF1: nonzero means EI
	data[28] = 0x01 // IFF2: the same
	data[29] = 0x02 // IM 2

	path := filepath.Join(t.TempDir(), "int.z80")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	e := New(model.Spectrum48K, &sound.NullOutput{})
	if _, err := e.LoadSnapshotFile(path); err != nil {
		t.Fatalf("LoadSnapshotFile: %v", err)
	}

	cpu := e.CPU()
	if !cpu.IFF1 || !cpu.IFF2 {
		t.Errorf("interrupts = IFF1 %v, IFF2 %v; want both enabled (byte 27/28, not 12)",
			cpu.IFF1, cpu.IFF2)
	}
	if cpu.IM != 2 {
		t.Errorf("IM = %d, want 2 (byte 29, not the low bits of byte 12)", cpu.IM)
	}
}

// TestLoadSnapshotFileZ80RoundTrip packs a 128K Z80 snapshot and checks the
// machine that comes back is the one that was saved, paging included.
func TestLoadSnapshotFileZ80RoundTrip(t *testing.T) {
	src := New(model.Spectrum128K, &sound.NullOutput{})
	banked(t, src)
	paging(t, src, 0x07)

	var buf bytes.Buffer
	s, err := src.CreateSNA()
	if err != nil {
		t.Fatalf("CreateSNA: %v", err)
	}
	if err := snap.SaveSNA(&buf, s); err != nil {
		t.Fatalf("SaveSNA: %v", err)
	}

	path := zipBytes(t, "game.sna", buf.Bytes())

	dst := New(model.Spectrum128K, &sound.NullOutput{})
	info, err := dst.LoadSnapshotFile(path)
	if err != nil {
		t.Fatalf("LoadSnapshotFile: %v", err)
	}
	if !info.Is128K {
		t.Error("128K snapshot loaded as 48K")
	}
	if got := dst.Mapper().(interface{ PagingValue() uint8 }).PagingValue(); got != 0x07 {
		t.Errorf("paging = 0x%02X after load, want 0x07", got)
	}
}

// TestLoadSnapshotFileUnknownFormat: the error names the entry inside the
// archive, not the ".zip" that carried it.
func TestLoadSnapshotFileUnknownFormat(t *testing.T) {
	path := zipBytes(t, "game.xyz", []byte("not a snapshot"))

	e := New(model.Spectrum48K, &sound.NullOutput{})
	_, err := e.LoadSnapshotFile(path)
	if err == nil {
		t.Fatal("expected an error for an unknown snapshot format")
	}
	if !strings.Contains(err.Error(), ".xyz") {
		t.Errorf("error %q should name the inner format", err)
	}
}

// TestLoadSnapshotFileMismatchedMachine: a 128K snapshot into a 48K machine is
// still refused, zipped or not -- the class check must not be sidestepped by the
// archive layer.
func TestLoadSnapshotFileMismatchedMachine(t *testing.T) {
	src := New(model.Spectrum128K, &sound.NullOutput{})
	banked(t, src)

	var buf bytes.Buffer
	s, err := src.CreateSNA()
	if err != nil {
		t.Fatalf("CreateSNA: %v", err)
	}
	if err := snap.SaveSNA(&buf, s); err != nil {
		t.Fatalf("SaveSNA: %v", err)
	}

	path := zipBytes(t, "game.sna", buf.Bytes())

	e := New(model.Spectrum48K, &sound.NullOutput{})
	if _, err := e.LoadSnapshotFile(path); err == nil {
		t.Fatal("expected a 128K snapshot to be refused by a 48K machine")
	}
}

// TestLoadSnapshotFileMissing reports the open failure.
func TestLoadSnapshotFileMissing(t *testing.T) {
	e := New(model.Spectrum48K, &sound.NullOutput{})
	if _, err := e.LoadSnapshotFile(filepath.Join(t.TempDir(), "nope.zip")); err == nil {
		t.Fatal("expected an error for a missing snapshot")
	}
}
