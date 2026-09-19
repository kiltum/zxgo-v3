package emulator

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/kiltum/zxgo-v3/pkg/mem"
	"github.com/kiltum/zxgo-v3/pkg/state"
)

// Machine state: save and restore every piece of state the machine holds, for
// every model (STATE_DESIGN.md).
//
// This file is the coordinator. It owns three things the components cannot own
// themselves:
//
//   - the section table, built for the live machine, so it lists exactly the
//     devices this model has and knows which of them are required;
//   - the pieces that need reconciling across components (the emulator's own
//     clock, the RAM banks under the mapper, the paging state under the
//     mapper's scheme);
//   - the normalisation that makes a save canonical - the audio grid and the
//     chips' deferred work settled before anything is read out.
//
// Everything else lives with the component that owns it: pkg/cpu, pkg/ula,
// pkg/sound, pkg/media and pkg/gs each implement SaveState/LoadState over their
// own private fields, so no component needs a public accessor for state it is
// the only reader of.
//
// The section table is the load order of STATE_DESIGN.md section 9. Save order
// does not matter; load order does, because every deadline in the file is an
// absolute T-state and the machine's clock has to be back before any of them
// means anything.

// BuildID names the binary that wrote a state file. It is informational - it
// goes into refusal messages and bug reports and never decides whether a file
// loads - and a release build may set it with
// -ldflags "-X .../internal/emulator.BuildID=<version>".
var BuildID = "dev"

// ErrRecording refuses a state load while a replay is being recorded. A loaded
// state makes the recorded event times meaningless: the replay would claim
// something happened at a tick the machine never reached.
var ErrRecording = errors.New("cannot load a state while a replay is being recorded")

// ramBankSize is the width of one RAM bank. The whole memory system is built on
// 16K banks (banking offsets are uint16 within a bank), and a file that claims
// any other width for a bank is refused rather than truncated to fit.
const ramBankSize = 16384

// SessionInfo describes what a load did, for the caller to report. The
// components' own result (which chunks were skipped or defaulted) is in Result.
type SessionInfo struct {
	Model  string // the model key the file was saved on
	Build  string // the build that wrote it
	CPUHz  int
	Ticks  int64 // totalTicks at the moment of the save
	Frames int64

	// Saved is the file's modification time. The format carries no timestamp:
	// the machine's own clock at save time is what a session is restored *to*,
	// and a wall-clock stamp would be a second, weaker answer to a question
	// nobody asks.
	Saved time.Time

	Result *state.Result
}

// String reports a session in one line, for a startup message or a log record.
func (s SessionInfo) String() string {
	out := fmt.Sprintf("session: %s saved by %s, %d ticks (%d frames) at %s",
		s.Model, s.Build, s.Ticks, s.Frames, s.Saved.Format(time.RFC3339))
	if s.Result != nil {
		if n := len(s.Result.Skipped); n > 0 {
			out += fmt.Sprintf(", %d unknown chunk(s) skipped", n)
		}
		if n := len(s.Result.Defaulted); n > 0 {
			out += fmt.Sprintf(", %d section(s) defaulted", n)
		}
	}
	return out
}

// SaveState writes the whole machine to w.
//
// The machine is normalised first (see normaliseForSave), so the file describes
// a machine with no unfinished work in flight: nothing is queued in a chip that
// only the audio path would have drained.
func (e *Emulator) SaveState(w io.Writer, opts state.Options) error {
	if e.cfg.Key == "" {
		// Without a key there is nothing to refuse a foreign file with, and the
		// file would load onto any machine that also has no key.
		return fmt.Errorf("model %q has no Key: cannot name the machine a state is saved from", e.cfg.Name)
	}
	e.normaliseForSave()

	hdr := state.Header{CPUHz: e.cpuHz, Model: e.cfg.Key, Build: BuildID}
	return state.Save(w, hdr, e.stateSections(), opts)
}

// LoadState reads a state from r and applies it.
//
// A file that does not belong to this machine's model, or that fails any of the
// container's checks, is refused before anything is applied. A refusal while a
// replay is being recorded is checked first, because no part of it is
// recoverable afterwards: the recording's timeline is already wrong.
func (e *Emulator) LoadState(r io.Reader) (*state.Result, error) {
	if e.recorder != nil {
		return nil, ErrRecording
	}
	if e.cfg.Key == "" {
		return nil, fmt.Errorf("model %q has no Key: cannot tell which machine this state belongs to", e.cfg.Name)
	}

	res, err := state.Load(r, e.cfg.Key, e.stateSections())
	if err != nil {
		return res, err
	}

	// The one thing derived rather than stored: the /INT level the CPU samples
	// at an instruction boundary. Recomputing it here is what keeps a stale
	// level from the saving machine out of the first instruction after a load.
	lastTick := e.totalTicks - 1
	e.cpu.SetInterruptLevel(e.cpu.RequiresInterrupt() && lastTick < e.ula.IntAssertedUntil())

	// A restored machine's audio grid is anchored at its restored tick, so
	// anything still staged belongs to a machine that no longer exists. The
	// wall-clock anchor is dropped for the same reason: it is measured against
	// the run that was interrupted.
	e.mixer.Discard()
	e.lastRealTime = time.Time{}
	return res, nil
}

// SaveSession writes the machine to path, atomically: the state goes to a
// temporary file in the same directory, is flushed to the device, and is then
// renamed over path. A crash during the write leaves the previous session
// intact rather than a truncated file where it used to be.
func (e *Emulator) SaveSession(path string, opts state.Options) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, filepath.Base(path)+".tmp*")
	if err != nil {
		return fmt.Errorf("creating a temporary session file in %s: %w", dir, err)
	}
	tmp := f.Name()
	defer func() {
		// If the rename below did not happen the temporary file is debris.
		if f != nil {
			f.Close()
			os.Remove(tmp)
		}
	}()

	if err := e.SaveState(f, opts); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("flushing %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", tmp, err)
	}
	f = nil

	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	return nil
}

// LoadSession reads a session file and applies it.
//
// An unreadable file is reported as an error and the machine is left alone, so
// the caller can start clean; nothing here is fatal to a process that is only
// trying to restore what it had.
func (e *Emulator) LoadSession(path string) (SessionInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return SessionInfo{}, err
	}
	defer f.Close()

	var info SessionInfo
	if st, err := f.Stat(); err == nil {
		info.Saved = st.ModTime()
	}
	res, err := e.LoadState(f)
	info.Result = res
	if res != nil {
		info.Model = res.Header.Model
		info.Build = res.Header.Build
		info.CPUHz = res.Header.CPUHz
	}
	if err != nil {
		return info, err
	}
	info.Ticks = e.totalTicks
	info.Frames = e.frameCount
	return info, nil
}

// normaliseForSave settles every piece of deferred work the machine holds, so
// what is read out is the state itself rather than the state plus a queue.
//
// Three things happen, and each is needed for a different device:
//
//   - the mixer's grid is advanced to now, so the sources' event lists are
//     complete up to the tick being saved;
//   - each event-based source drops the events the mixer has already consumed
//     (the beeper's and the DAC's cursors), because those are history the
//     restored machine would otherwise replay;
//   - the General Sound compacts its DAC event ring, so what is stored is the
//     ring the mixer will read next rather than the whole run's log.
//
// The AY is deliberately not flushed: its queue of register writes is state on
// the same tick clock, not history, and is stored as it stands (pkg/sound).
func (e *Emulator) normaliseForSave() {
	e.mixer.Resolve(e.totalTicks)
	e.beeper.ResetEvents()
	e.covox.ResetEvents()
	if e.gs != nil {
		e.gs.ResetEvents()
	}
}

// stateSections builds the section table for this machine: exactly the devices
// it has, in load order.
//
// Everything here is required except the media, which a machine legitimately
// may not have: a file that is missing the CPU chunk is not a session from
// which this machine can be resumed, while a file with no disk is simply a
// machine with no disk.
func (e *Emulator) stateSections() []state.Spec {
	sections := []state.Spec{
		{
			ID:       state.IDEmulator,
			Version:  1,
			Required: true,
			Save: func(enc *state.Encoder) error {
				enc.I64(e.totalTicks)
				enc.I64(e.frameCount)
				enc.I64(e.lastIntTick)
				enc.Bool(e.fastTape)
				return nil
			},
			Load: func(dec *state.Decoder) error {
				ticks, frames, lastInt := dec.I64(), dec.I64(), dec.I64()
				fast := dec.Bool()
				if err := dec.Err(); err != nil {
					return err
				}
				e.totalTicks = ticks
				e.frameCount = frames
				e.lastIntTick = lastInt
				e.fastTape = fast
				return nil
			},
		},
		{
			ID:       state.IDRAM,
			Version:  1,
			Required: true,
			Save: func(enc *state.Encoder) error {
				banks := e.mapper.Snapshot()
				enc.Count(len(banks))
				for _, b := range banks {
					enc.Bytes(b)
				}
				return nil
			},
			Load: func(dec *state.Decoder) error {
				// Every length is checked against the machine before it is used
				// for anything: the count against this mapper's bank count, and
				// each bank against the width the memory system uses.
				n := dec.Count(4)
				if err := dec.Err(); err != nil {
					return err
				}
				if want := e.mapper.NumRAMBanks(); n != want {
					return fmt.Errorf("file has %d RAM banks, this machine has %d", n, want)
				}
				banks := make([][]byte, n)
				for i := range banks {
					b := dec.Bytes()
					if err := dec.Err(); err != nil {
						return err
					}
					if len(b) != ramBankSize {
						return fmt.Errorf("RAM bank %d is %d bytes, want %d", i, len(b), ramBankSize)
					}
					banks[i] = b
				}
				// RestoreSnapshot copies, so the banks may alias the file's body.
				e.mapper.RestoreSnapshot(banks)
				return nil
			},
		},
		{
			ID:       state.IDPaging,
			Version:  1,
			Required: true,
			Save: func(enc *state.Encoder) error {
				ps := e.mapper.PagingState()
				enc.String(ps.Scheme)
				enc.Bytes(ps.Blob)
				enc.U8(ps.Write7FFD)
				return nil
			},
			Load: func(dec *state.Decoder) error {
				scheme, blob, write7FFD := dec.String(), dec.Bytes(), dec.U8()
				if err := dec.Err(); err != nil {
					return err
				}
				return e.mapper.RestorePagingState(mem.PagingState{
					Scheme:    scheme,
					Blob:      blob,
					Write7FFD: write7FFD,
				})
			},
		},
		state.Section(state.IDCPU, 1, true, e.cpu),
		state.Section(state.IDULA, 1, true, e.ula),
		// The mixer rides with the emulator chunk rather than with the sound
		// devices: its grid position is meaningless without totalTicks, and
		// saving one without the other is the "state that depends on a queue"
		// failure the format is meant to avoid. The chips' own phase comes with
		// their chunks.
		state.Section(state.IDMixer, 1, true, e.mixer),
	}
	return append(sections, e.deviceSections()...)
}

// deviceSections is the tail of the table: the devices beyond the core, in load
// order, exactly those the machine has.
//
// The AY chip reports its own chunk id, because which one it is decides the
// payload: a single 8912, a TurboSound pair or a TurboSound FM board. All three
// are mutually exclusive, so a file carries one of the three ids and a reader
// with a different chip finds the section absent rather than misreading it.
func (e *Emulator) deviceSections() []state.Spec {
	sections := []state.Spec{
		state.SectionOf(1, true, e.ay),
		state.Section(state.IDBeeper, 1, true, e.beeper),
		state.Section(state.IDCovox, 1, true, e.covox),
	}
	if e.gs != nil {
		sections = append(sections, state.Section(state.IDGeneralSound, 1, true, e.gs))
	}
	return append(sections, e.storageSections()...)
}
