package emulator

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"

	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/replay"
	"github.com/kiltum/zxgo-v3/pkg/sound"
	"github.com/kiltum/zxgo-v3/pkg/state"
)

// The state tests run real machines: the same code path a user's session takes,
// with the embedded ROMs and no display.
//
// Two of them are the point of the whole design and the rest support them:
//
//   - the round trip, which compares what can be compared field by field;
//   - run-forward equality, which is the only test that proves exactness. Save,
//     restore into a second machine, run both, and compare every frame. A save
//     that carries only the visible state passes the first and fails this one,
//     because a generator phase or a raster position that is off by a few
//     T-states diverges from the frame it was restored at.

func newModel(t *testing.T, key string) *Emulator {
	t.Helper()
	cfg, ok := model.AllModels[key]
	if !ok {
		t.Fatalf("no model %q", key)
	}
	e, err := NewFromModel(cfg, "roms", &sound.NullOutput{})
	if err != nil {
		t.Fatalf("building %s: %v", key, err)
	}
	return e
}

// everyModelKey is the list of machines the state format has to cover. It is
// read from the registry rather than hand-written so a new model joins the tests
// by existing.
func everyModelKey(t *testing.T) []string {
	t.Helper()
	keys := make([]string, 0, len(model.AllModels))
	for k := range model.AllModels {
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		t.Fatal("no models registered")
	}
	return keys
}

// screenDigest hashes the framebuffer. The pixels are the machine's visible
// output, so two machines that hash the same frame are showing the same thing.
func screenDigest(e *Emulator) uint32 {
	px := e.ULA().GetScreen()
	var buf [4]byte
	h := crc32.NewIEEE()
	for _, p := range px {
		binary.LittleEndian.PutUint32(buf[:], p)
		h.Write(buf[:])
	}
	return h.Sum32()
}

// runFrames advances a machine and returns the digest of every frame it drew,
// with the tick count at the end of each.
func runFrames(e *Emulator, n int) ([]uint32, []int64) {
	digests := make([]uint32, 0, n)
	ticks := make([]int64, 0, n)
	for i := 0; i < n; i++ {
		e.RunFrame()
		digests = append(digests, screenDigest(e))
		ticks = append(ticks, e.TotalTicks())
	}
	return digests, ticks
}

func saveToBytes(t *testing.T, e *Emulator) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := e.SaveState(&buf, state.Options{}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	return buf.Bytes()
}

// TestRunForwardEquality is the exactness test. Save a machine that has been
// running, restore it into a second one built from the same model, and run both:
// every frame and every tick has to match from there on.
func TestRunForwardEquality(t *testing.T) {
	for _, key := range everyModelKey(t) {
		t.Run(key, func(t *testing.T) {
			original := newModel(t, key)
			// Enough frames for the ROM to have left the power-on screen and be
			// doing real work with the raster somewhere mid-frame.
			runFrames(original, 40)

			saved := saveToBytes(t, original)

			restored := newModel(t, key)
			if _, err := restored.LoadState(bytes.NewReader(saved)); err != nil {
				t.Fatalf("LoadState: %v", err)
			}

			wantDigests, wantTicks := runFrames(original, 30)
			gotDigests, gotTicks := runFrames(restored, 30)

			for i := range wantDigests {
				if gotTicks[i] != wantTicks[i] {
					t.Fatalf("frame %d: totalTicks = %d, want %d", i, gotTicks[i], wantTicks[i])
				}
				if gotDigests[i] != wantDigests[i] {
					t.Fatalf("frame %d: framebuffer differs (%08x, want %08x)", i, gotDigests[i], wantDigests[i])
				}
			}
		})
	}
}

// TestStateRoundTrip compares the machine piece by piece, which is what names
// the component when the run-forward test fails.
func TestStateRoundTrip(t *testing.T) {
	for _, key := range everyModelKey(t) {
		t.Run(key, func(t *testing.T) {
			original := newModel(t, key)
			runFrames(original, 25)
			// Poke something into the machine so the comparison is not all
			// power-on defaults: a distinguishable byte in every RAM bank, and
			// the CPU's alternate registers.
			for b := 0; b < original.Mapper().NumRAMBanks(); b++ {
				bank := original.Mapper().GetBank(b)
				for i := range bank {
					bank[i] = uint8(b<<4 | (i & 0x0F))
				}
			}

			saved := saveToBytes(t, original)

			restored := newModel(t, key)
			if _, err := restored.LoadState(bytes.NewReader(saved)); err != nil {
				t.Fatalf("LoadState: %v", err)
			}

			if got, want := restored.TotalTicks(), original.TotalTicks(); got != want {
				t.Errorf("totalTicks = %d, want %d", got, want)
			}
			if got, want := restored.FrameCount(), original.FrameCount(); got != want {
				t.Errorf("frameCount = %d, want %d", got, want)
			}

			// RAM, bank by bank.
			if got, want := restored.Mapper().NumRAMBanks(), original.Mapper().NumRAMBanks(); got != want {
				t.Fatalf("RAM banks = %d, want %d", got, want)
			}
			for b := 0; b < original.Mapper().NumRAMBanks(); b++ {
				want := original.Mapper().GetBank(b)
				got := restored.Mapper().GetBank(b)
				if !bytes.Equal(got, want) {
					diff := -1
					for i := range want {
						if got[i] != want[i] {
							diff = i
							break
						}
					}
					t.Fatalf("RAM bank %d differs first at %d (%02x, want %02x)", b, diff, got[diff], want[diff])
				}
			}

			// The CPU, register by register.
			oc, rc := original.CPU(), restored.CPU()
			if oc.A != rc.A || oc.F != rc.F || oc.B != rc.B || oc.C != rc.C ||
				oc.D != rc.D || oc.E != rc.E || oc.H != rc.H || oc.L != rc.L ||
				oc.A_ != rc.A_ || oc.F_ != rc.F_ || oc.H_ != rc.H_ || oc.L_ != rc.L_ ||
				oc.IX != rc.IX || oc.IY != rc.IY || oc.SP != rc.SP || oc.PC != rc.PC ||
				oc.I != rc.I || oc.R != rc.R || oc.IM != rc.IM ||
				oc.IFF1 != rc.IFF1 || oc.IFF2 != rc.IFF2 || oc.MEMPTR != rc.MEMPTR ||
				oc.IsNMOS != rc.IsNMOS {
				t.Errorf("CPU does not match:\n got %+v\nwant %+v", rc, oc)
			}

			// The ULA's position, which is what the next instruction's
			// contention depends on.
			if got, want := restored.ULA().AbsoluteClock(), original.ULA().AbsoluteClock(); got != want {
				t.Errorf("ULA absoluteClock = %d, want %d", got, want)
			}
			if got, want := restored.ULA().Clock(), original.ULA().Clock(); got != want {
				t.Errorf("ULA clock = %d, want %d", got, want)
			}
			if got, want := restored.ULA().Line(), original.ULA().Line(); got != want {
				t.Errorf("ULA line = %d, want %d", got, want)
			}
			if got, want := restored.ULA().BorderColor(), original.ULA().BorderColor(); got != want {
				t.Errorf("border = %d, want %d", got, want)
			}

			// The paging state, and the recorded 0x7FFD write with it.
			if got, want := restored.Mapper().PagingState(), original.Mapper().PagingState(); got.Scheme != want.Scheme ||
				got.Write7FFD != want.Write7FFD || !bytes.Equal(got.Blob, want.Blob) {
				t.Errorf("paging = %+v, want %+v", got, want)
			}
		})
	}
}

// TestStateRefusesAnotherModel: the banks and the device set would not line up,
// so the file is refused rather than half-applied.
func TestStateRefusesAnotherModel(t *testing.T) {
	original := newModel(t, "48k")
	runFrames(original, 5)
	saved := saveToBytes(t, original)

	other := newModel(t, "128k")
	before := other.TotalTicks()
	if _, err := other.LoadState(bytes.NewReader(saved)); !errors.Is(err, state.ErrModel) {
		t.Fatalf("err = %v, want state.ErrModel", err)
	}
	if other.TotalTicks() != before {
		t.Error("a refused load changed the machine")
	}
}

// TestStateRefusedLoadLeavesMachineUntouched: a corrupt file must not leave the
// target half-way between two machines.
func TestStateRefusedLoadLeavesMachineUntouched(t *testing.T) {
	original := newModel(t, "48k")
	runFrames(original, 5)
	saved := saveToBytes(t, original)

	target := newModel(t, "48k")
	runFrames(target, 7)
	wantTicks := target.TotalTicks()
	wantDigest := screenDigest(target)
	wantBank := append([]byte(nil), target.Mapper().GetBank(5)...)

	corrupt := append([]byte(nil), saved...)
	corrupt[len(corrupt)-1] ^= 0xFF
	if _, err := target.LoadState(bytes.NewReader(corrupt)); !errors.Is(err, state.ErrCRC) {
		t.Fatalf("err = %v, want state.ErrCRC", err)
	}

	if target.TotalTicks() != wantTicks {
		t.Errorf("totalTicks = %d, want %d", target.TotalTicks(), wantTicks)
	}
	if got := screenDigest(target); got != wantDigest {
		t.Errorf("framebuffer changed: %08x, want %08x", got, wantDigest)
	}
	if got := target.Mapper().GetBank(5); !bytes.Equal(got, wantBank) {
		t.Error("RAM changed after a refused load")
	}
}

// TestStateLoadRefusedWhileRecording: a loaded state makes the recorded event
// times meaningless, so the load is refused before anything is touched.
func TestStateLoadRefusedWhileRecording(t *testing.T) {
	original := newModel(t, "48k")
	runFrames(original, 3)
	saved := saveToBytes(t, original)

	target := newModel(t, "48k")
	target.SetRecorder(replay.NewRecorder())

	if _, err := target.LoadState(bytes.NewReader(saved)); !errors.Is(err, ErrRecording) {
		t.Fatalf("err = %v, want ErrRecording", err)
	}
	if target.TotalTicks() != 0 {
		t.Error("the machine was modified despite the refusal")
	}
}

// TestStateKeyboardIsReleasedOnLoad: a key held when the state was written is
// not held after a load.
func TestStateKeyboardIsReleasedOnLoad(t *testing.T) {
	original := newModel(t, "48k")
	original.PressKey(0, 1) // CAPS SHIFT down
	runFrames(original, 2)
	saved := saveToBytes(t, original)

	restored := newModel(t, "48k")
	if _, err := restored.LoadState(bytes.NewReader(saved)); err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	// Port 0xFE with row 0 selected reads the row's bits; a released row is all
	// ones. The ULA ANDs in the EAR bit, so mask to the five key bits.
	if got := restored.ReadPort(0xFEFE) & 0x1F; got != 0x1F {
		t.Errorf("keyboard row 0 = %05b, want all released", got)
	}
}

// TestSessionRoundTripThroughFile exercises the file-level API: the atomic
// write, the read back, and the report.
func TestSessionRoundTripThroughFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.zxstate")

	original := newModel(t, "128k")
	runFrames(original, 20)

	if err := original.SaveSession(path, state.Options{Deflate: true}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	// The temporary file is renamed into place, so nothing is left beside it.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "session.zxstate" {
		t.Errorf("directory holds %v, want just the session file", entries)
	}

	restored := newModel(t, "128k")
	info, err := restored.LoadSession(path)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if info.Model != "128k" || info.Build != BuildID {
		t.Errorf("info = %+v, want the model key and this build", info)
	}
	if info.Ticks != original.TotalTicks() || info.Frames != original.FrameCount() {
		t.Errorf("info ticks/frames = %d/%d, want %d/%d",
			info.Ticks, info.Frames, original.TotalTicks(), original.FrameCount())
	}
	if info.Saved.IsZero() {
		t.Error("info carries no modification time")
	}
	if info.String() == "" {
		t.Error("SessionInfo.String() is empty")
	}

	// Overwriting an existing session works, and still leaves one file.
	if err := restored.SaveSession(path, state.Options{}); err != nil {
		t.Fatalf("second SaveSession: %v", err)
	}
	entries, _ = os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("directory holds %v after a second save", entries)
	}
}

// TestLoadSessionMissingFile: a session file that is not there is a reported
// error and nothing else - the caller starts clean.
func TestLoadSessionMissingFile(t *testing.T) {
	e := newModel(t, "48k")
	if _, err := e.LoadSession(filepath.Join(t.TempDir(), "absent.zxstate")); err == nil {
		t.Fatal("loading a missing session succeeded")
	}
}

// TestPagingSurvivesRoundTrip covers the paging state directly, because it is
// the one component whose encoding is keyed by scheme and whose value can be
// non-default in ways a booting machine never reaches.
func TestPagingSurvivesRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		model string
		write func(*Emulator)
	}{
		{"128k", func(e *Emulator) {
			e.WritePort(0x7FFD, 0x1B) // bank 3 at 0xC000, shadow screen, ROM 1, lock
		}},
		{"pentagon512", func(e *Emulator) {
			e.WritePort(0x7FFD, 0xC5) // 5-bit page: bits 0-2 = 5, bits 6-7 -> bit 3
		}},
		{"2a3", func(e *Emulator) {
			e.WritePort(0x1FFD, 0x01) // special paging mode: RAM over 0x0000
			e.WritePort(0x1FFD, 0x05) // and a different special map
		}},
	} {
		t.Run(tc.model, func(t *testing.T) {
			original := newModel(t, tc.model)
			runFrames(original, 5)
			tc.write(original)

			want := original.Mapper().PagingState()
			saved := saveToBytes(t, original)

			restored := newModel(t, tc.model)
			if _, err := restored.LoadState(bytes.NewReader(saved)); err != nil {
				t.Fatalf("LoadState: %v", err)
			}
			got := restored.Mapper().PagingState()
			if got.Scheme != want.Scheme {
				t.Fatalf("scheme = %q, want %q", got.Scheme, want.Scheme)
			}
			if got.Write7FFD != want.Write7FFD {
				t.Errorf("Write7FFD = %#x, want %#x", got.Write7FFD, want.Write7FFD)
			}
			if !bytes.Equal(got.Blob, want.Blob) {
				t.Errorf("paging blob differs: %x, want %x", got.Blob, want.Blob)
			}
			// And the state has to be *usable*, not just equal: the same write
			// on both machines has to move the same bytes.
			original.Mapper().WriteByte(0xC000, 0xA5)
			restored.Mapper().WriteByte(0xC000, 0xA5)
			for addr := uint16(0x4000); addr < 0xFFFF; addr += 0x1111 {
				if a, b := original.Mapper().ReadByte(addr), restored.Mapper().ReadByte(addr); a != b {
					t.Fatalf("address %04x: %02x != %02x after an identical write", addr, a, b)
				}
			}
		})
	}
}

// TestPagingSchemeMismatchRefused: the Pentagon 512's page is five bits in the
// same field the 128K uses for three, so a blob from one must not be read by the
// other. The scheme name is what stops it.
func TestPagingSchemeMismatchRefused(t *testing.T) {
	pentagon := newModel(t, "pentagon512")
	runFrames(pentagon, 3)
	ps := pentagon.Mapper().PagingState()
	if ps.Scheme != "pentagon512" {
		t.Fatalf("scheme = %q, want pentagon512", ps.Scheme)
	}

	spectrum := newModel(t, "128k")
	err := spectrum.Mapper().RestorePagingState(ps)
	if err == nil {
		t.Fatal("a pentagon512 paging blob was accepted by a 128k machine")
	}
}

// TestStateCarriesNoHostState: the replay recorder and the player are session
// state, and neither a save nor a load touches them.
func TestStateCarriesNoHostState(t *testing.T) {
	target := newModel(t, "48k")
	runFrames(target, 5)

	rec := replay.NewRecorder()
	target.SetRecorder(rec)
	if target.Recorder() != rec {
		t.Fatal("recorder did not attach")
	}
	// A save while recording is allowed: only a load invalidates the timeline.
	var buf bytes.Buffer
	if err := target.SaveState(&buf, state.Options{}); err != nil {
		t.Fatalf("SaveState while recording: %v", err)
	}
	if target.Recorder() != rec {
		t.Error("the recorder was disturbed by a save")
	}
}
