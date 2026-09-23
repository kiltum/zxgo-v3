package frontend

import (
	"github.com/kiltum/zxgo-v3/internal/emulator"
	"github.com/kiltum/zxgo-v3/pkg/media"
	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/replay"
	"github.com/kiltum/zxgo-v3/pkg/snap"
	"github.com/kiltum/zxgo-v3/pkg/state"
)

// frameTStates is one 48K frame, which is what the fake machine counts in.
const frameTStates = 69888

// fakeMachine is a Machine with no emulator behind it. It counts what it was
// asked to do, which is what the front end's tests are about, and it models the
// matrix itself because D9's whole point is which calls reach it.
type fakeMachine struct {
	cfg model.Config

	ticks  int64
	frames int64
	// frameBase is the tick count the last counted frame ended at, so RunSlice
	// can report a frame boundary the way the emulator does: from the T-states
	// it actually ran.
	frameBase int64

	resets int
	nmis   int
	steps  int
	slices int

	fastTape bool
	recorder *replay.Recorder
	isNMOS   bool
	memptr   bool

	hasFDC         bool
	loadedSnapshot string
	madeSnapshots  int
	noFDCTiming    bool
	diskMounted    bool
	diskPath       string
	diskInfo       media.DiskInfo

	tapeMounted bool
	tapeName    string
	tapePos     int
	tapePlaying bool
	tapeEnded   bool
	tapeStarts  int
	tapeStops   int
	rewinds     int

	pressed  []Cell
	released []Cell
	down     map[Cell]bool

	// joy is every joystick state the front end wrote, in order, so a test can ask
	// both what the machine ended up holding and how many times it was told.
	joy []JoyState

	screen Screen

	savedTo  string
	loadedOf string
}

func newFakeMachine() *fakeMachine {
	return &fakeMachine{
		cfg:    model.Spectrum48K,
		down:   make(map[Cell]bool),
		screen: Screen{W: 2, H: 2, Pix: make([]uint32, 4)},
		// The two CPU switches start where the real chip starts: NMOS and the measured
		// MEMPTR behaviour (pkg/cpu). A fake that started them false would report a machine
		// the emulator never builds.
		isNMOS: true,
		memptr: true,
	}
}

func (m *fakeMachine) NMI() { m.nmis++ }

func (m *fakeMachine) Reset() {
	m.resets++
	m.ticks, m.frames, m.frameBase = 0, 0, 0
	m.down = make(map[Cell]bool)
}
func (m *fakeMachine) RunFrame()                 { m.ticks += frameTStates; m.frames++ }
func (m *fakeMachine) StepOne()                  { m.steps++; m.ticks += 4 }
func (m *fakeMachine) CPUHz() int                { return 3500000 }
func (m *fakeMachine) TotalTicks() int64         { return m.ticks }
func (m *fakeMachine) FrameCount() int64         { return m.frames }
func (m *fakeMachine) ModelConfig() model.Config { return m.cfg }
func (m *fakeMachine) Screen() Screen            { return m.screen }

// RunSlice advances one 10 ms slice, exactly as the emulator does, and reports a
// frame boundary from the T-states that ran rather than on a schedule.
func (m *fakeMachine) RunSlice() bool {
	m.slices++
	m.ticks += int64(m.CPUHz() / 100)
	if m.ticks-m.frameBase >= frameTStates {
		m.frameBase = m.ticks
		m.frames++
		return true
	}
	return false
}

func (m *fakeMachine) PressKey(row, col int) {
	cell := Cell{Row: row, Col: col}
	m.pressed = append(m.pressed, cell)
	m.down[cell] = true
}

func (m *fakeMachine) MatrixKey(row, col int) bool {
	return m.down[Cell{Row: row, Col: col}]
}

func (m *fakeMachine) ReleaseKey(row, col int) {
	cell := Cell{Row: row, Col: col}
	m.released = append(m.released, cell)
	delete(m.down, cell)
}

// SetJoystick records every write, including the ones that repeat a state: what
// the front end's own tests are about is whether it wrote at all, so the fake
// must not collapse the written states the way the real port would.
func (m *fakeMachine) SetJoystick(j JoyState) { m.joy = append(m.joy, j) }

func (m *fakeMachine) StartTapePlayback()   { m.tapeStarts++; m.tapePlaying = true }
func (m *fakeMachine) StopTapePlayback()    { m.tapeStops++; m.tapePlaying = false }
func (m *fakeMachine) RewindTape()          { m.rewinds++; m.tapeEnded = false }
func (m *fakeMachine) FastTapeActive() bool { return m.fastTape }
func (m *fakeMachine) SetFastTape(on bool)  { m.fastTape = on }

func (m *fakeMachine) Tape() (media.TapeState, bool) {
	if !m.tapeMounted {
		return media.TapeState{Current: -1}, false
	}
	return media.TapeState{
		FileName: m.tapeName, Format: "TAP", Playing: m.tapePlaying,
		Ended: m.tapeEnded, Pos: m.tapePos, Pulses: 1000, Percent: 0.1,
	}, true
}

func (m *fakeMachine) LoadTapeFromPath(path string) error {
	m.tapeMounted, m.tapeName = true, path
	return nil
}

func (m *fakeMachine) FDC() (media.FDCState, bool) {
	if !m.diskMounted {
		return media.FDCState{IndexPhase: -1}, m.hasFDC
	}
	return media.FDCState{
		Controller: "WD1793", Command: "READ SECTOR", Ready: true, Drive: 0,
		Disk: m.diskInfo, Track: 2, Sector: 3, Busy: true, DRQ: true,
		IndexPhase: 0.25,
	}, true
}

func (m *fakeMachine) MountDiskFromPath(path string) (media.DiskInfo, error) {
	m.diskMounted, m.diskPath = true, path
	return m.diskInfo, nil
}

func (m *fakeMachine) EjectDisk() { m.diskMounted, m.diskPath = false, "" }

func (m *fakeMachine) DiskPath() string { return m.diskPath }

func (m *fakeMachine) LoadSnapshotFile(path string) (emulator.SnapshotInfo, error) {
	m.loadedSnapshot = path
	return emulator.SnapshotInfo{Format: "SNA", Name: path}, nil
}

func (m *fakeMachine) CreateSNA() (*snap.Snapshot, error) {
	m.madeSnapshots++
	return &snap.Snapshot{RAM: make([]byte, 49152)}, nil
}

func (m *fakeMachine) SetNoFDCTiming(on bool) { m.noFDCTiming = on }

func (m *fakeMachine) NoFDCTiming() bool { return m.noFDCTiming }

func (m *fakeMachine) FastTape() bool { return m.fastTape }

func (m *fakeMachine) SetRecorder(rec *replay.Recorder) { m.recorder = rec }

func (m *fakeMachine) SetCPUType(isNMOS bool) { m.isNMOS = isNMOS }

func (m *fakeMachine) IsNMOS() bool { return m.isNMOS }

func (m *fakeMachine) SetMEMPTRReal(real bool) { m.memptr = real }

func (m *fakeMachine) IsMEMPTRReal() bool { return m.memptr }

func (m *fakeMachine) TapeMounted() bool { return m.tapeMounted }
func (m *fakeMachine) TapePlaying() bool { return m.tapePlaying }
func (m *fakeMachine) TapeEnded() bool   { return m.tapeEnded }

func (m *fakeMachine) SaveSession(path string, _ state.Options) error {
	m.savedTo = path
	return nil
}

func (m *fakeMachine) LoadSession(path string) (emulator.SessionInfo, error) {
	m.loadedOf = path
	return emulator.SessionInfo{Model: m.cfg.Key}, nil
}

var _ Machine = (*fakeMachine)(nil)

// testApp is an App over a fake machine with its output collected instead of
// printed, which is what every test below wants.
func testApp() (*App, *fakeMachine) {
	m := newFakeMachine()
	app := New(m)
	app.Notice = func(string) {}
	app.Warn = func(string) {}
	return app, m
}
