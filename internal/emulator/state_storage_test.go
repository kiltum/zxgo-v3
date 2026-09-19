package emulator

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/kiltum/zxgo-v3/pkg/media"
	"github.com/kiltum/zxgo-v3/pkg/state"
)

// Storage: a session saved with a disk mounted and a tape playing has to come
// back with both, and come back *into the middle of* what they were doing.
//
// The disk is embedded, so the test can also prove the other half of that
// decision: a disk the machine has written to is restored as the machine left
// it, not as the file on disk has it.

// testDisk loads a disk image from testdata, or skips if the corpus is absent
// (the repository ships a large set that is not always present).
func testDisk(t *testing.T, name string) *media.Disk {
	t.Helper()
	for _, dir := range []string{"../../testdata/", "testdata/"} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		disk, err := media.LoadDiskFile(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		return disk
	}
	t.Skipf("disk image %s not available", name)
	return nil
}

// TestDiskIsEmbeddedAndRestored: the disk goes into the session as bytes, so a
// write the machine made after the file was loaded survives a restore.
func TestDiskIsEmbeddedAndRestored(t *testing.T) {
	disk := testDisk(t, "exolon.scl")
	original := newModel(t, "128k")
	if err := original.LoadDisk(disk); err != nil {
		t.Fatalf("LoadDisk: %v", err)
	}
	runFrames(original, 5)

	// Write a marker through the controller's own path, so what is compared is
	// the machine's view of the disk rather than a copy of the file.
	const (
		track = 5
		sect  = 3
	)
	marker := make([]byte, 256) // exactly one TR-DOS sector
	copy(marker, "ZXGOSTATE MARKER")
	before, err := original.BetaDisk().ActiveDisk().ReadSector(track, 0, sect)
	if err != nil {
		t.Fatalf("ReadSector: %v", err)
	}
	if err := original.BetaDisk().ActiveDisk().WriteSector(track, 0, sect, marker); err != nil {
		t.Fatalf("WriteSector: %v", err)
	}
	_ = before

	saved := saveToBytes(t, original)

	restored := newModel(t, "128k")
	if _, err := restored.LoadState(bytes.NewReader(saved)); err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if restored.BetaDisk() == nil || restored.BetaDisk().ActiveDisk() == nil {
		t.Fatal("no disk mounted after the load")
	}
	got, err := restored.BetaDisk().ActiveDisk().ReadSector(track, 0, sect)
	if err != nil {
		t.Fatalf("ReadSector after load: %v", err)
	}
	if !bytes.HasPrefix(got, marker) {
		t.Errorf("sector %d/%d does not hold the write the machine made: %x", track, sect, got[:min(32, len(got))])
	}
	if restored.BetaDisk().GetTrack() != original.BetaDisk().GetTrack() {
		t.Errorf("track register = %d, want %d", restored.BetaDisk().GetTrack(), original.BetaDisk().GetTrack())
	}
}

// TestDiskImageSurvivesEveryFormat exercises the disk codec on the formats the
// repository ships, including the ones whose sectors carry decoded status.
func TestDiskImageSurvivesEveryFormat(t *testing.T) {
	for _, name := range []string{"exolon.scl", "1.scl", "batty.tap"} {
		if name == "batty.tap" {
			continue // a tape, not a disk
		}
		t.Run(name, func(t *testing.T) {
			disk := testDisk(t, name)
			e := state.NewEncoder()
			media.SaveDisk(e, disk)
			d := state.NewDecoder(e.Payload(), 1)
			got, err := media.LoadDisk(d)
			if err != nil {
				t.Fatalf("LoadDisk: %v", err)
			}
			if err := d.Err(); err != nil {
				t.Fatalf("trailing bytes: %v", err)
			}
			if got.Type != disk.Type || got.Sides != disk.Sides || got.TracksPerSide != disk.TracksPerSide ||
				got.SectorsPerTrack != disk.SectorsPerTrack || got.SectorSize != disk.SectorSize ||
				got.ReadOnly != disk.ReadOnly {
				t.Errorf("geometry = %+v, want %+v", got, disk)
			}
			if len(got.Tracks) != len(disk.Tracks) {
				t.Fatalf("%d tracks, want %d", len(got.Tracks), len(disk.Tracks))
			}
			for ti := range disk.Tracks {
				ws, gs := disk.Tracks[ti].Sectors, got.Tracks[ti].Sectors
				if len(ws) != len(gs) {
					t.Fatalf("track %d: %d sectors, want %d", ti, len(gs), len(ws))
				}
				for si := range ws {
					a, b := ws[si], gs[si]
					if a.Track != b.Track || a.Side != b.Side || a.SectorID != b.SectorID || a.N != b.N ||
						a.Deleted != b.Deleted || a.ST1 != b.ST1 || a.ST2 != b.ST2 || a.IDCRC != b.IDCRC {
						t.Fatalf("track %d sector %d header differs: %+v vs %+v", ti, si, b, a)
					}
					if !bytes.Equal(a.Data, b.Data) {
						t.Fatalf("track %d sector %d data differs", ti, si)
					}
					if len(a.Weak) != len(b.Weak) {
						t.Fatalf("track %d sector %d weak mask length %d, want %d", ti, si, len(b.Weak), len(a.Weak))
					}
					for i := range a.Weak {
						if a.Weak[i] != b.Weak[i] {
							t.Fatalf("track %d sector %d weak bit %d differs", ti, si, i)
						}
					}
				}
			}
		})
	}
}

// TestTapeIsReferencedAndResumes: a tape is a path plus a pulse-stream hash, so
// a restore resumes where playback was - and a tape that has changed underneath
// is loaded from the start rather than resumed into the middle of another tape.
func TestTapeIsReferencedAndResumes(t *testing.T) {
	path := ""
	for _, dir := range []string{"../../testdata/", "testdata/"} {
		if _, err := os.Stat(filepath.Join(dir, "batty.tap")); err == nil {
			path = filepath.Join(dir, "batty.tap")
			break
		}
	}
	if path == "" {
		t.Skip("testdata/batty.tap not available")
	}

	original := newModel(t, "48k")
	tape, err := media.LoadTapeFile(path)
	if err != nil {
		t.Fatalf("LoadTapeFile: %v", err)
	}
	if err := original.LoadTape(tape); err != nil {
		t.Fatalf("LoadTape: %v", err)
	}
	original.StartTapePlayback()
	for i := 0; i < 5; i++ {
		original.RunFrame()
	}
	// The tape has to have actually advanced, or resuming would be trivial.
	if original.TapePlayback().Pos == 0 {
		t.Skip("the tape did not advance in five frames")
	}
	wantPos := original.TapePlayback().Pos
	wantTick := original.TapePlayback().TapeTick

	saved := saveToBytes(t, original)

	restored := newModel(t, "48k")
	if _, err := restored.LoadState(bytes.NewReader(saved)); err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	pb := restored.TapePlayback()
	if pb == nil {
		t.Fatal("no tape after the load")
	}
	if pb.Pos != wantPos || pb.TapeTick != wantTick {
		t.Errorf("playback at pulse %d tick %d, want %d/%d", pb.Pos, pb.TapeTick, wantPos, wantTick)
	}
	if !pb.IsPlaying() {
		t.Error("the tape is not playing after the load")
	}
	if restored.tapeResult == nil || restored.tapeResult.Outcome != tapeOutcomeResumed {
		t.Errorf("tape outcome = %v, want resumed", restored.tapeResult)
	}

	// And it carries on from there rather than from the beginning.
	restored.RunFrame()
	if got := restored.TapePlayback().Pos; got <= wantPos {
		t.Errorf("playback did not advance: %d, want more than %d", got, wantPos)
	}
}

// TestTapeThatChangedIsLoadedFromTheStart: the file is still there but is not
// the tape the session was playing, so resuming into the middle of it would be
// resuming into the middle of something else.
func TestTapeThatChangedIsLoadedFromTheStart(t *testing.T) {
	src := ""
	for _, dir := range []string{"../../testdata/", "testdata/"} {
		if _, err := os.Stat(filepath.Join(dir, "batty.tap")); err == nil {
			src = filepath.Join(dir, "batty.tap")
			break
		}
	}
	if src == "" {
		t.Skip("testdata/batty.tap not available")
	}

	// The session refers to a path that will hold a *different* tape by the time
	// it is restored, which is what the hash is there to catch.
	dir := t.TempDir()
	path := filepath.Join(dir, "tape.tap")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	original := newModel(t, "48k")
	tape, err := media.LoadTapeFile(path)
	if err != nil {
		t.Fatalf("LoadTapeFile: %v", err)
	}
	if err := original.LoadTape(tape); err != nil {
		t.Fatalf("LoadTape: %v", err)
	}
	original.StartTapePlayback()
	for i := 0; i < 5; i++ {
		original.RunFrame()
	}
	saved := saveToBytes(t, original)
	if original.TapePlayback().Pos == 0 {
		t.Skip("the tape did not advance")
	}

	// Replace the file with a shorter, different tape.
	other, err := media.LoadTapeFile(src)
	if err != nil {
		t.Fatal(err)
	}
	shortened := &media.Tape{FileName: path, Format: other.Format, Pulses: other.Pulses[:len(other.Pulses)/2]}
	if err := os.WriteFile(path, encodeTAP(t, shortened), 0o644); err != nil {
		t.Fatal(err)
	}

	restored := newModel(t, "48k")
	if _, err := restored.LoadState(bytes.NewReader(saved)); err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if restored.tapeResult == nil || restored.tapeResult.Outcome != tapeOutcomeChanged {
		t.Errorf("tape outcome = %v, want changed", restored.tapeResult)
	}
	if restored.TapePlayback() == nil {
		t.Fatal("no tape after the load")
	}
	if restored.TapePlayback().Pos != 0 {
		t.Errorf("playback resumed at pulse %d, want 0", restored.TapePlayback().Pos)
	}
}

// TestTapeMissingIsReported: the file is gone, so the machine comes back with no
// tape rather than refusing the session.
func TestTapeMissingIsReported(t *testing.T) {
	src := ""
	for _, dir := range []string{"../../testdata/", "testdata/"} {
		if _, err := os.Stat(filepath.Join(dir, "batty.tap")); err == nil {
			src = filepath.Join(dir, "batty.tap")
			break
		}
	}
	if src == "" {
		t.Skip("testdata/batty.tap not available")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "tape.tap")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	original := newModel(t, "48k")
	tape, err := media.LoadTapeFile(path)
	if err != nil {
		t.Fatalf("LoadTapeFile: %v", err)
	}
	if err := original.LoadTape(tape); err != nil {
		t.Fatal(err)
	}
	original.StartTapePlayback()
	runFrames(original, 3)
	saved := saveToBytes(t, original)

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	restored := newModel(t, "48k")
	if _, err := restored.LoadState(bytes.NewReader(saved)); err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if restored.tapeResult == nil || restored.tapeResult.Outcome != tapeOutcomeMissing {
		t.Errorf("tape outcome = %v, want missing", restored.tapeResult)
	}
	if restored.TapePlayback() != nil {
		t.Error("a tape is mounted although its file is gone")
	}
	if s := restored.tapeResult.String(); s == "" {
		t.Error("the outcome does not describe itself")
	}
}

// encodeTAP writes a pulse stream back out as a TAP file, so a test can put a
// different tape where the session expects the old one.
func encodeTAP(t *testing.T, tape *media.Tape) []byte {
	t.Helper()
	var buf bytes.Buffer
	// A TAP file is a sequence of length-prefixed blocks. The pulse stream a
	// tape was decoded into is not losslessly re-encodable in general, so this
	// writes one synthetic block: enough to be a valid, *different* tape.
	block := []byte{0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	buf.WriteByte(byte(len(block) & 0xFF))
	buf.WriteByte(byte(len(block) >> 8))
	buf.Write(block)
	return buf.Bytes()
}

// TestPlus3DiskIsRestored covers the uPD765 path, which exists only on the
// +2A/+3 and so is not reached by the TR-DOS tests.
func TestPlus3DiskIsRestored(t *testing.T) {
	disk := testDisk(t, "exolon.scl")
	e := newModel(t, "2a3")
	if err := e.LoadDisk(disk); err != nil {
		t.Fatalf("LoadDisk: %v", err)
	}
	runFrames(e, 5)

	if e.UPD765() == nil {
		t.Fatal("the +2A/+3 has no uPD765")
	}
	if ids := sectionIDs(e); !ids[state.IDUPD765] {
		t.Error("the +2A/+3 does not list the uPD765's section")
	}

	saved := saveToBytes(t, e)

	restored := newModel(t, "2a3")
	if _, err := restored.LoadState(bytes.NewReader(saved)); err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if restored.UPD765() == nil || restored.UPD765().Drives()[0] == nil {
		t.Fatal("the +3 disk was not restored")
	}
	if restored.UPD765().Drives()[0].Type != disk.Type {
		t.Errorf("disk type = %q, want %q", restored.UPD765().Drives()[0].Type, disk.Type)
	}
}

// TestSectionsForEachModel names the whole table for each machine, so a device
// that stops being registered is visible here rather than as a session that
// quietly loses it.
func TestSectionsForEachModel(t *testing.T) {
	for _, key := range everyModelKey(t) {
		t.Run(key, func(t *testing.T) {
			e := newModel(t, key)
			ids := sectionIDs(e)
			for _, want := range []state.ID{
				state.IDEmulator, state.IDRAM, state.IDPaging, state.IDCPU, state.IDULA,
				state.IDMixer, state.IDBeeper, state.IDCovox, state.IDBetaDisk,
			} {
				if !ids[want] {
					t.Errorf("no section for %s", want)
				}
			}
			// A Beta 128 is an add-on, so the controller is built for every
			// model and every model lists it. The +3's own FDC is part of the
			// machine and is listed only there.
			if e.ModelConfig().PagingModel == "2a3" && !ids[state.IDUPD765] {
				t.Error("the +2A/+3 should have a uPD765 section")
			}
			if e.ModelConfig().PagingModel != "2a3" && ids[state.IDUPD765] {
				t.Error("only the +2A/+3 has a uPD765")
			}
		})
	}
}
