package frontend

import (
	"github.com/kiltum/zxgo-v3/internal/emulator"
	"github.com/kiltum/zxgo-v3/pkg/media"
	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/replay"
	"github.com/kiltum/zxgo-v3/pkg/snap"
	"github.com/kiltum/zxgo-v3/pkg/state"
)

// Screen is one frame of the machine's picture: an ARGB8888 framebuffer and the
// size that gives it meaning. It is the front end's type rather than the
// backend's because App produces it and the backend consumes it (UI_DESIGN.md
// section 4), and the size travels with the pixels so a caller cannot pair one
// model's geometry with another's buffer.
type Screen struct {
	W, H int
	Pix  []uint32
}

// Machine is the emulator as the front end uses it.
//
// It is an interface so that M1's tests can drive a fake with no ROMs, no audio
// device and no window; UI_DESIGN.md section 5.5 is the other reason - a
// `Machine` implemented over the worker protocol would let the GUI drive a
// headless emulator in another process, and nothing in M0-M8 depends on that.
//
// This is not yet the whole of UI_DESIGN.md section 5.1. That contract is the
// target; the methods appear here as the stage that needs them lands, so that
// nothing is declared before something calls it and tests it. Still to come:
// `ReadPort`/`WritePort`/`ReadMemory`/`ScreenText` (M8).
type Machine interface {
	// Execution.
	Reset()
	// NMI is the non-maskable interrupt button: one call is one NMI.
	NMI()
	// SetCPUType selects the Z80 variant, and IsNMOS reports the one in force. It is live
	// (D10): the variant changes the undocumented flags, not the instruction set. The
	// variant is set on the running chip, so a caller changing it under a running machine
	// resets afterwards (section 6.4) - the window does - while one applying it to a machine
	// that has just been restored does not.
	SetCPUType(isNMOS bool)
	IsNMOS() bool
	// SetMEMPTRReal selects the MEMPTR behaviour of the repeating block I/O instructions,
	// and IsMEMPTRReal reports it. It is live like the variant, and needs no reset: it
	// decides a value a later instruction writes, not one already computed (KNOWN_BUGS.md).
	SetMEMPTRReal(real bool)
	IsMEMPTRReal() bool
	RunSlice() bool
	RunFrame()
	StepOne()
	TotalTicks() int64
	FrameCount() int64
	CPUHz() int
	ModelConfig() model.Config

	// The screen, for PresentMain.
	Screen() Screen

	// Input. D9: these are the methods the replay recorder hangs off, so every
	// route to the matrix goes through them and nothing reaches the ULA directly.
	PressKey(row, col int)
	ReleaseKey(row, col int)
	// MatrixKey reports whether a key of the matrix is held, which is what the
	// on-screen keyboard draws from: a key can be down because the machine's own code
	// or a replay put it there, not only because the front end pressed it.
	MatrixKey(row, col int) bool

	// Tape transport. D9 again: the recorder hooks the transport methods, not the
	// Playback object they wrap.
	StartTapePlayback()
	StopTapePlayback()
	RewindTape()
	FastTapeActive() bool
	SetFastTape(on bool)
	// FastTape is the switch rather than the live state: a recording writes down what the
	// machine was configured for, and FastTapeActive is only true while a tape is
	// actually running fast.
	FastTape() bool
	// SetRecorder attaches a recorder, which then sees every key and transport action
	// through the methods above (D9). Nil detaches it.
	SetRecorder(rec *replay.Recorder)

	// Tape state, for the transport actions and the status read-out. The three
	// questions are what the actions need; Tape is the state a window draws, and
	// answers false for a machine with no tape rather than making the caller check
	// first.
	TapeMounted() bool
	TapePlaying() bool
	TapeEnded() bool
	Tape() (media.TapeState, bool)
	LoadTapeFromPath(path string) error

	// Disk state and media. FDC answers false for a machine with no floppy controller,
	// and the two media calls are what the disks window and its dialog use.
	FDC() (media.FDCState, bool)
	MountDiskFromPath(path string) (media.DiskInfo, error)
	EjectDisk()
	DiskPath() string
	SetNoFDCTiming(on bool)
	NoFDCTiming() bool

	// Snapshots. SNA is the interchange format; the session format is the machine's
	// own (STATE_DESIGN.md).
	LoadSnapshotFile(path string) (emulator.SnapshotInfo, error)
	CreateSNA() (*snap.Snapshot, error)

	// Session (STATE_DESIGN.md).
	SaveSession(path string, opts state.Options) error
	LoadSession(path string) (emulator.SessionInfo, error)
}

// Adapter makes *emulator.Emulator satisfy Machine. Most of the interface is
// already there under the same name, so it is embedded; the methods below are
// the ones that need a name of their own or a little translation.
type Adapter struct {
	*emulator.Emulator
}

// The adapter must actually satisfy the interface: most of it is satisfied by
// embedding, which the compiler only checks if something asks.
var _ Machine = (*Adapter)(nil)

// NewMachine returns the Machine the CLI drives: the emulator itself, adapted.
func NewMachine(emu *emulator.Emulator) *Adapter {
	return &Adapter{Emulator: emu}
}

// StepOne executes a single instruction, which is what the front end's step
// action means and what `RunInstructions(1)` does.
func (a *Adapter) StepOne() { a.Emulator.RunInstructions(1) }

// RewindTape returns the tape to its start. It is the transport call rather than
// a direct rewind on the Playback object for the D9 reason: a rewind is a
// recorded event, and reaching past the emulator would make it invisible.
func (a *Adapter) RewindTape() { a.Emulator.ResetTapePlayback() }

// TapeMounted reports whether a tape is loaded.
func (a *Adapter) TapeMounted() bool { return a.Emulator.TapePlayback() != nil }

// TapePlaying reports whether the tape is running.
func (a *Adapter) TapePlaying() bool {
	t := a.Emulator.TapePlayback()
	return t != nil && t.IsPlaying()
}

// TapeEnded reports whether the tape has run off its end, which is the state the
// loop announces once.
func (a *Adapter) TapeEnded() bool {
	t := a.Emulator.TapePlayback()
	return t != nil && t.Ended()
}

// MatrixKey reports whether a key of the ZX matrix is held.
func (a *Adapter) MatrixKey(row, col int) bool {
	return a.Emulator.ULA().IsKeyDown(row, col)
}

// Screen reports the ULA's framebuffer and size. The ULA owns both, so they
// cannot disagree; the adaption is only that the front end wants them as one
// value.
func (a *Adapter) Screen() Screen {
	ula := a.Emulator.ULA()
	return Screen{W: ula.Width(), H: ula.Height(), Pix: ula.GetScreen()}
}
