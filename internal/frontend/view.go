package frontend

import (
	"strconv"
	"time"

	"github.com/kiltum/zxgo-v3/pkg/media"
)

// StatusView is what the menubar's right-aligned read-outs show (D4): the run
// state, the machine, the clock, and the frame rate the emulation is actually
// achieving.
type StatusView struct {
	Run      RunState
	Model    string
	CPUHz    int
	Ticks    int64
	Frames   int64
	FPS      float64
	FastTape bool
}

// Views bundles the view models the backend draws. It is plain data, refreshed
// from the facade each tick, and holds no pointer that Reset() replaces -
// Emulator.Reset swaps the CPU, so a view model that cached one would be reading
// a dead object (CLAUDE.md).
//
// Only StatusView has landed. The rest of UI_DESIGN.md section 5.6 - DiskView,
// FDCView, TapeView, KeymapView, SettingsView, DebugView - arrives with the
// windows that read them at M4 and later, because each needs an accessor the
// emulator does not have yet (sections 6.2 and 6.3).
type Views struct {
	Status StatusView
	// Tape is the tape window's state, refreshed while that window is open. A tool
	// window that is closed costs nothing, which is what section 5.6 asks for: a tape
	// snapshot copies a block list, and there is no reason to do that sixty times a
	// second for a window nobody is looking at.
	Tape media.TapeState
	// Disks is the disks window's state: what is in the drive and what the controller
	// is doing.
	Disks DiskView
	// Keyboard is the keyboard window's state: which keys of the matrix are down.
	Keyboard KeyboardView
	// Settings is the settings window's state: the choices, and whether any of them is
	// waiting for a relaunch.
	Settings SettingsView
}

// SettingsView is what the settings window draws.
//
// The rows are lists the front end owns rather than fields the backend knows how to build, so
// which models, which switches and which groups exist is one place - the same arrangement
// Tools() and KeyboardRows() already use - and the backend draws a row without knowing what is
// in it. A row's action is what a click asks for; the front end decides what it does (section
// 9).
type SettingsView struct {
	// Radios are the mutually exclusive rows: the machine, the Z80 variant, the MEMPTR
	// behaviour. Each is a group of options with the one in force flagged.
	Radios []RadioGroup
	// Toggles are the switch rows, grouped by what they affect: the sound devices, the two
	// speed switches, the session.
	Toggles []ToggleGroup
	// Restart reports that a restart-required change is waiting for a relaunch.
	Restart bool
}

// RadioGroup is a labelled set of mutually exclusive options, drawn as radio buttons: one
// heading, and the options on one line under it.
type RadioGroup struct {
	Label   string
	Options []Choice
	// Note is the line under the options, for the same reason ToggleGroup has one: whether a
	// row applies at once is the front end's knowledge, not the backend's.
	Note string
}

// KeyboardView is what the on-screen keyboard draws from: the keys the machine is
// holding.
//
// It is read from the machine rather than from the front end's own bookkeeping, so a key
// is drawn held whether the mouse clicked it, the host keyboard pressed it, or a replay
// put it there (M5's *done when* is about the first, and the other two would look wrong
// otherwise).
type KeyboardView struct {
	Down map[Cell]bool
}

// DiskView is what the disks window draws: the disk in the selected drive and the
// floppy controller's own state.
type DiskView struct {
	// Mounted is whether anything is in the drive, and Info is what it is.
	Mounted bool
	Info    media.DiskInfo
	// Path is the file the disk came from. A disk does not know it, and its TR-DOS name
	// is empty for other formats, so the machine remembers it.
	Path string
	// FDC is the controller's state, and HasFDC says whether this machine has one at
	// all: a 48K has no floppy controller until a disk has been mounted in it.
	FDC    media.FDCState
	HasFDC bool
	// NoTiming is the seek and rotational latency switch: off is the faithful
	// setting, on is for a large image that does not need to load accurately.
	NoTiming bool
}

// fpsMeter turns a frame counter into a rate.
//
// D7 is the reason it counts emulated frames (Machine.FrameCount) rather than
// presents: during a fast tape load the loop presents a handful of times a
// second while the machine runs at many times real speed, and a read-out built
// from presents would report the refresh rate and call it emulation speed.
type fpsMeter struct {
	lastFrames int64
	lastAt     time.Time
	fps        float64
}

// observe takes a frame count and the time it was read at, and returns the
// current estimate. The estimate is recomputed at most once a second, so a
// read-out does not flicker between two numbers that differ in the last digit.
func (m *fpsMeter) observe(frames int64, now time.Time) float64 {
	if m.lastAt.IsZero() {
		m.lastAt, m.lastFrames = now, frames
		return m.fps
	}
	elapsed := now.Sub(m.lastAt)
	if elapsed < time.Second {
		return m.fps
	}
	m.fps = float64(frames-m.lastFrames) / elapsed.Seconds()
	m.lastAt, m.lastFrames = now, frames
	return m.fps
}

// reset drops the measurement, so a resumed machine does not average its rate
// across the pause. Reset() replaces the machine, and a rate measured across
// that boundary would be a rate of nothing.
func (m *fpsMeter) reset() {
	m.lastAt = time.Time{}
	m.fps = 0
}

// formatFPS renders the frame rate for the menubar read-out. It is one decimal
// place, which is enough to see a fast load running at 10x rather than at the
// refresh rate, and it does not jitter on the last digit the way an integer does.
func formatFPS(fps float64) string {
	if fps <= 0 {
		return "- fps"
	}
	return strconv.FormatFloat(fps, 'f', 1, 64) + " fps"
}
