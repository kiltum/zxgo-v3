// Package emulator is the main ZX Spectrum emulator orchestrator.
// It wires the CPU, ULA, memory, I/O ports, keyboard, and sound together,
// and runs the main emulation loop.
package emulator

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/kiltum/zxgo-v3/pkg/bus"
	"github.com/kiltum/zxgo-v3/pkg/cpu"
	"github.com/kiltum/zxgo-v3/pkg/gs"
	"github.com/kiltum/zxgo-v3/pkg/io_ports"
	"github.com/kiltum/zxgo-v3/pkg/logger"
	"github.com/kiltum/zxgo-v3/pkg/media"
	"github.com/kiltum/zxgo-v3/pkg/mem"
	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/replay"
	"github.com/kiltum/zxgo-v3/pkg/sound"
	"github.com/kiltum/zxgo-v3/pkg/ula"
)

// intLog is an optional logger for interrupt timing diagnostics.
var intLog *slog.Logger

// SetInterruptLogger wires a logger into the emulator for interrupt tracing.
func SetInterruptLogger(l *slog.Logger) { intLog = l }

// Emulator coordinates all ZX Spectrum components.
type Emulator struct {
	cfg      model.Config
	cpu      *cpu.Z80
	ula      *ula.ULA
	mapper   mem.MemoryMapper
	portBus  *io_ports.PortBus
	bus      *bus.DefaultBus
	beeper   *sound.Beeper
	ay       sound.AYChip // AY-3-8912 or a TurboSound pair (see AYChip)
	covox    *sound.Covox // Covox 8-bit DAC (Pentagon/Scorpion add-on)
	gs       *gs.GS       // General Sound card (second Z80 + DAC)
	mixer    *sound.Mixer
	kempston *io_ports.Kempston
	audioOut sound.AudioOutput

	cpuHz       int   // 3.5 MHz (48K), 3.5469 MHz (128K), 3.584 MHz (Pentagon)
	totalTicks  int64 // cumulative T-states since reset; the audio grid's anchor
	frameCount  int64 // frame number for logger tick context
	lastIntTick int64

	// Real-time throttling: moving anchor (end of the previous slice) that keeps
	// emulated time in step with wall-clock time without repaying stall debt.
	lastRealTime time.Time

	// Phase 5: Tape loading support
	tape     *media.Playback // Current tape playback
	tapeTick int64           // Last tick when tape was updated

	// fastTape runs the CPU unthrottled while the tape plays, so a load takes
	// host time instead of tape time (see SetFastTape).
	fastTape bool

	// Phase 7: Disk system support (128K/+2/+3 models)
	betaDisk *media.BetaDiskController // Beta Disk controller (WD1793)
	upd765   *media.UPD765             // uPD765 floppy controller (+2A/+3)

	// diskPath is where the mounted disk came from. A Disk does not carry its own
	// path and its TR-DOS name is empty for other formats, so a front end that wanted
	// to name the file would have nothing to name it by.
	diskPath string

	// trace records a per-instruction delta trace when enabled (Phase 8).
	trace *Trace

	// Input recording and playback (replay.go). The recorder hangs off the
	// emulator's own input API so every source is captured, not just the GUI's
	// keyboard; the player injects at instruction boundaries in step().
	recorder *replay.Recorder
	player   *replay.Player

	// tapeResult is what the last state load did with the tape it referred to
	// (state_media.go). It is a report, not machine state: a save never reads it.
	tapeResult *TapeInfo
}

// CPU returns the Z80 for debugger access.
func (e *Emulator) CPU() *cpu.Z80 { return e.cpu }

// ULA returns the ULA for screen access.
func (e *Emulator) ULA() *ula.ULA { return e.ula }

// Mapper returns the memory mapper for snapshot operations.
func (e *Emulator) Mapper() mem.MemoryMapper { return e.mapper }

// Beeper returns the beeper.
func (e *Emulator) Beeper() *sound.Beeper { return e.beeper }

// AY returns the AY sound source (a single AY-3-8912 or a TurboSound pair).
func (e *Emulator) AY() sound.AYChip { return e.ay }

// Mixer returns the audio mixer (latency and drop diagnostics, AY attachment).
func (e *Emulator) Mixer() *sound.Mixer { return e.mixer }

// AudioOut returns the current audio output.
func (e *Emulator) AudioOut() sound.AudioOutput { return e.audioOut }

// ModelConfig returns the model configuration.
func (e *Emulator) ModelConfig() model.Config { return e.cfg }

// GS returns the General Sound card, or nil when the machine has none.
func (e *Emulator) GS() *gs.GS { return e.gs }

// Kempston returns the Kempston joystick.
func (e *Emulator) Kempston() *io_ports.Kempston { return e.kempston }

// LoadDisk mounts a disk image. The +2A/+3 uses the uPD765 floppy controller;
// other models use the Beta Disk controller (WD1793).
func (e *Emulator) LoadDisk(disk *media.Disk) error {
	if e.cfg.PagingModel == "2a3" {
		// The controller is built with the machine (see NewWithLayout), so this
		// only has to put the disk in it.
		if e.upd765 == nil {
			return fmt.Errorf("this machine has no floppy controller")
		}
		e.upd765.MountDisk(disk)
		return nil
	}
	if e.betaDisk == nil {
		e.betaDisk = media.NewBetaDiskController()
		e.betaDisk.SetClockHz(e.cpuHz)
		e.portBus.Register(e.betaDisk)
	}
	e.betaDisk.MountDisk(disk)
	return nil
}

// MountDiskFromPath loads a disk image and mounts it, and is what a front end uses:
// the format dispatch, the mount and the read-out are already single-sourced here, so
// a window and the command line cannot disagree about what a file is.
//
// The path is remembered because the emulator needs it and nothing else does: a disk
// carries its own TR-DOS name, which is empty for other formats, so a window with only
// the disk in hand could not say which file it came from.
func (e *Emulator) MountDiskFromPath(path string) (media.DiskInfo, error) {
	disk, err := media.LoadDiskFile(path)
	if err != nil {
		return media.DiskInfo{}, err
	}
	if err := e.LoadDisk(disk); err != nil {
		return media.DiskInfo{}, err
	}
	e.diskPath = path
	return disk.GetDetailedInfo(), nil
}

// SetNoFDCTiming turns the floppy controller's seek and rotational latency off, or
// back on. It applies to whichever controllers the machine has, and it is a live
// setting: D10 lists it as one, because it changes nothing about the peripheral graph.
//
// The state lives in the machine's config, which is where a *load* reads it from: two
// places holding it would let the window say one thing and the next mount do another.
func (e *Emulator) SetNoFDCTiming(on bool) {
	e.cfg.NoFDCTiming = on
	if e.betaDisk != nil {
		e.betaDisk.SetNoTiming(on)
	}
	if e.upd765 != nil {
		e.upd765.SetNoTiming(on)
	}
}

// NoFDCTiming reports whether the controller is modelling seek and rotational latency.
func (e *Emulator) NoFDCTiming() bool { return e.cfg.NoFDCTiming }

// EjectDisk takes the disk out of the selected drive, leaving the controller in place.
// Both controllers keep their state: what is ejected is the medium, not the chip.
func (e *Emulator) EjectDisk() {
	if e.upd765 != nil {
		e.upd765.MountDisk(nil)
	}
	if e.betaDisk != nil {
		e.betaDisk.UnmountDisk()
	}
	e.diskPath = ""
}

// DiskPath is the file the mounted disk came from, or empty when there is none. It is
// separate from a DiskInfo because a disk does not know where it was loaded from.
func (e *Emulator) DiskPath() string { return e.diskPath }

// DiskInfo is what is in the drive, and whether there is anything there at all.
func (e *Emulator) DiskInfo() (media.DiskInfo, bool) {
	fdc, ok := e.FDC()
	if !ok || !fdc.Ready {
		return media.DiskInfo{}, false
	}
	return fdc.Disk, true
}

// FDC reports the floppy controller's state, and whether this machine has one. A
// machine with no controller answers false rather than making every caller ask which
// model it is.
func (e *Emulator) FDC() (media.FDCState, bool) {
	switch {
	case e.upd765 != nil:
		return e.upd765.StateSnapshot(), true
	case e.betaDisk != nil:
		return e.betaDisk.StateSnapshot(), true
	}
	return media.FDCState{}, false
}

// BetaDisk returns the Beta Disk controller.
func (e *Emulator) BetaDisk() *media.BetaDiskController { return e.betaDisk }

// UPD765 returns the uPD765 floppy controller (+2A/+3).
func (e *Emulator) UPD765() *media.UPD765 { return e.upd765 }

// LoadTape loads a TAP/TZX file for playback.
// Note: Tape must be started manually via StartTapePlayback() after LOAD "" command
func (e *Emulator) LoadTape(tape *media.Tape) error {
	e.tape = media.NewPlayback(tape)
	// Tape does NOT auto-start - user must press CMD-P to start playback
	// This gives manual control like a real tape recorder
	return nil
}

// TapePlayback returns the current tape playback controller.
func (e *Emulator) TapePlayback() *media.Playback { return e.tape }

// Tape reports the tape's state as plain data, and whether one is loaded. A front end
// asks every frame it draws a tape window, so a machine with no tape answers rather
// than being a case the caller has to check for first.
func (e *Emulator) Tape() (media.TapeState, bool) {
	if e.tape == nil {
		return media.TapeState{Current: -1}, false
	}
	return e.tape.StateSnapshot(), true
}

// LoadTapeFromPath loads a tape image, zipped or not: the format is whatever the file
// (or the archive entry) turns out to be, so no caller has to know.
func (e *Emulator) LoadTapeFromPath(path string) error {
	tape, err := media.LoadTapeFile(path)
	if err != nil {
		return err
	}
	return e.LoadTape(tape)
}

// SetFastTape enables fast tape loading: while a tape is playing the emulator
// skips the speed throttle and runs as fast as the host allows. The tape bit
// stream is driven by emulated T-states, so a load completes in host time
// rather than in tape time; the throttle returns as soon as playback stops
// (paused, or run off the end, which clears Playback.Active by itself).
func (e *Emulator) SetFastTape(on bool) { e.fastTape = on }

// FastTapeActive reports whether the emulator is currently running unthrottled
// for a fast tape load. The hosting loop should stop presenting frames while it
// is true: on macOS SDL_RenderPresent blocks once its drawable pool is
// exhausted, which caps the loop at the display refresh rate and puts the tape
// back on something close to real time.
func (e *Emulator) FastTapeActive() bool { return e.fastTapeActive() }

// FastTape is the fast-tape switch itself, which is not the same as whether a tape is
// currently running fast: a recording writes down how the machine was configured, and
// FastTapeActive is only true while a tape is actually playing.
func (e *Emulator) FastTape() bool { return e.fastTape }

// fastTapeActive reports whether the emulator should currently run unthrottled.
func (e *Emulator) fastTapeActive() bool {
	if !e.fastTape || e.tape == nil {
		return false
	}
	return e.tape.IsPlaying() && !e.tape.Ended()
}

// StartTapePlayback starts tape playback active.
func (e *Emulator) StartTapePlayback() {
	if e.tape != nil {
		e.recordTape(replay.TapePlay)
		e.tape.Play()
	}
}

// StopTapePlayback pauses tape playback.
func (e *Emulator) StopTapePlayback() {
	if e.tape != nil {
		e.recordTape(replay.TapePause)
		e.tape.Pause()
	}
}

// ResetTapePlayback rewinds tape to beginning. A rewind is deliberately not a
// recorded event: the player has no "rewind" to dispatch, and a session that
// rewinds is better served by the recording ending there than by a tape the
// replay cannot put back.
func (e *Emulator) ResetTapePlayback() {
	if e.tape != nil {
		e.tape.Reset()
	}
}

// PressKey presses a key in the ZX Spectrum keyboard matrix (row 0-7, col 0-4).
//
// The tick stamped here is the same tick playback injects at, so a recorded
// press lands exactly where it was made. Recording hangs off this method rather
// than off the SDL event loop so that every source is captured -- the GUI, the
// MCP worker's press_key, anything else that drives the machine.
func (e *Emulator) PressKey(row, col int) {
	if e.recorder != nil {
		e.recorder.Key(e.totalTicks, row, col, true)
	}
	e.ula.SetKeyDown(row, col)
}

// ReleaseKey releases a key in the ZX Spectrum keyboard matrix.
func (e *Emulator) ReleaseKey(row, col int) {
	if e.recorder != nil {
		e.recorder.Key(e.totalTicks, row, col, false)
	}
	e.ula.SetKeyUp(row, col)
}

// LoadROM loads a ROM bank.
func (e *Emulator) LoadROM(bank int, data []byte) error {
	e.mapper.SetROMWritable(true)
	err := e.mapper.LoadROM(bank, data)
	e.mapper.SetROMWritable(false)
	return err
}

// Reset resets all components.
//
// totalTicks must be zeroed with the mixer: the sample grid is anchored to
// tick 0, and leaving the clock running across a reset made the first chunk of
// audio look like a multi-second gap and vanish (AUDIT S3).
func (e *Emulator) Reset() {
	e.mapper.Reset()
	e.ula.Reset()
	e.beeper.Reset()
	e.ay.Reset()    // Reset AY sound chip (Phase 4)
	e.covox.Reset() // Reset Covox DAC
	if e.gs != nil {
		e.gs.Reset() // Reset the General Sound card
	}
	e.kempston.Reset()
	e.mixer.Reset()
	e.totalTicks = 0
	e.lastIntTick = 0
	e.lastRealTime = time.Time{} // Reset real-time tracking (ISSUE 4)
	e.cpu = cpu.New(e.bus)

	// Phase 5: Reset tape playback if loaded
	if e.tape != nil {
		e.tape.Reset()
	}

	// Phase 7: Reset Beta Disk controller if present
	if e.betaDisk != nil {
		e.betaDisk.Reset()
	}
	if e.upd765 != nil {
		e.upd765.Reset()
	}
}

// SetCPUType selects the Z80 variant. UI_DESIGN.md section 6.4 asks for `SetCPUType`
// followed by `Reset()` when the user changes it, and the reset is the *caller's* step
// rather than this one's, because the two callers want different things: the settings
// window resets, since the registers would otherwise hold flags from the other chip, while a
// startup applies the variant to a machine that has just been restored from a session and
// must not throw that state away.
//
// NMOS is the hardware and the default; CMOS is what the +2A/+3 and later machines carried,
// and it differs in the undocumented flags, not in the instruction set.
func (e *Emulator) SetCPUType(isNMOS bool) { e.cpu.SetCPUType(isNMOS) }

// IsNMOS reports which Z80 variant the machine is running. It is read from the CPU rather
// than remembered, so a session that was resumed with the other variant reports that one.
func (e *Emulator) IsNMOS() bool { return e.cpu.IsNMOS }

// SetMEMPTRReal selects the MEMPTR behaviour of the repeating block I/O instructions: true is
// the measured hardware behaviour, false the documented BC-derived one. See KNOWN_BUGS.md -
// the two are visible only on that repeat path, and the default is the measured one.
//
// Unlike the CPU variant this needs no reset: it decides what a block instruction leaves in
// MEMPTR, and the next one that runs gets the new answer. There is no state that was computed
// under the old switch.
func (e *Emulator) SetMEMPTRReal(on bool) { e.cpu.SetMEMPTRReal(on) }

// IsMEMPTRReal reports which MEMPTR behaviour the machine is running. Like IsNMOS it is read
// from the CPU, because a session carries it too.
func (e *Emulator) IsMEMPTRReal() bool { return e.cpu.MEMPTRReal() }

// NMI asks the CPU for a non-maskable interrupt, which it takes at the next
// instruction boundary. It is the button on a real Spectrum, and it exists because
// a front end and a debugger both want one (UI_DESIGN.md section 6.1).
//
// The latch clears when the interrupt is serviced, so one call is one NMI: holding
// the button down does not storm the CPU.
func (e *Emulator) NMI() { e.cpu.RequestNMI() }

// step executes one instruction and advances every subsystem by its T-states.
// Returns the T-states consumed and whether the frame completed.
//
// The ordering here is load-bearing:
//  1. the bus needs the frame position *before* the instruction, for contention;
//  2. the beeper needs the tick *before* the instruction, so an OUT during it is
//     timestamped at the instruction's start (sub-instruction placement needs
//     the per-M-cycle hook -- AUDIT D1/T6);
//  3. contention accumulated during the instruction's accesses is added after;
//  4. the ULA is ticked once per T-state, interrupt checked each tick.
//
// The AY is NOT ticked here: it is driven from the mixer's sample windows
// (MeanLevel), the same contract as the Beeper. Its register writes are
// timestamped via SetTick before each instruction and applied as the
// generator reaches them.
func (e *Emulator) step() (int, bool) {
	// Recorded input is settled first: an event due at this tick is a press that
	// arrived between the previous instruction and this one, so it must be in
	// the keyboard matrix before this instruction can read it.
	e.applyReplay()

	e.bus.UpdateFrameClock(e.ula.Clock())
	// The absolute tick of this instruction's start: per-machine-cycle timing
	// (contention positions and audio event stamps) is measured from here.
	e.bus.SetInstructionTick(e.totalTicks)
	e.beeper.SetTick(e.totalTicks)
	e.ay.SetTick(e.totalTicks)
	e.covox.SetTick(e.totalTicks)
	if e.betaDisk != nil {
		// The FDC needs the tick clock for one thing only: drive rotation.
		// TR-DOS decides a disk is present by watching the index-pulse bit
		// change, so a stalled clock reads as "no disk".
		e.betaDisk.SetTick(e.totalTicks)
	}
	if e.upd765 != nil {
		// The uPD765 needs the tick clock to time out a read-data phase whose
		// bytes the CPU has stopped draining (overrun), which the Erbe "Long 8K
		// tracks" copy-protection loaders depend on to end a partial read.
		e.upd765.SetTick(e.totalTicks)
	}

	// Update shared tick clock for logger (adds tick= and frame= to all log records)
	logger.SetTick(e.totalTicks, e.frameCount)

	var regBefore [7]uint16
	if e.trace != nil && e.trace.Enabled() {
		regBefore = snapshotRegs(e.cpu)
		e.trace.beginInstruction(e.cpu.PC)
	}

	ts := e.cpu.ExecuteOneInstruction()
	ts += e.bus.FlushContention()

	// Advance the General Sound's own Z80 in lockstep with the Spectrum CPU.
	if e.gs != nil {
		e.gs.Tick(int64(ts))
	}

	if e.trace != nil && e.trace.Enabled() {
		e.trace.endInstruction(regBefore, e.cpu)
	}
	e.totalTicks += int64(ts)

	frameDone := false
	prevClock := e.ula.Clock() // Track ULA clock to detect wraparound

	for i := 0; i < ts; i++ {
		currentTick := e.totalTicks - int64(ts) + int64(i)
		e.ula.OneTick()
		currentClock := e.ula.Clock()

		// Phase 5: Update tape EAR bit (drives ULA port 0xFE bit 6)
		if e.tape != nil && e.tape.IsPlaying() {
			earLevel := e.tape.CurrentLevel(currentTick)
			e.ula.SetAudioState(earLevel)
		}

		// Detect frame completion: the frame clock wraps from end-of-frame to 0.
		if currentClock < prevClock {
			frameDone = true
			e.frameCount++ // Increment frame counter for logger tick context
			// Finish this instruction's remaining ULA ticks.
			for j := i + 1; j < ts; j++ {
				e.ula.OneTick()
			}
			break
		}

		prevClock = currentClock
	}

	// AUDIT T4: sample the /INT level at the instruction boundary (the last
	// tick), not at every tick. /INT is level-triggered and sampled at the end of
	// the instruction; if the pulse has ended before the instruction completes, a
	// request must not survive to the next instruction.
	lastTick := e.totalTicks - 1
	intActive := e.cpu.RequiresInterrupt() && lastTick < e.ula.IntAssertedUntil()
	e.cpu.SetInterruptLevel(intActive)
	if intActive {
		e.lastIntTick = lastTick
		if intLog != nil {
			intLog.Debug("int_sample", "clock", e.ula.Clock(), "pc", e.cpu.PC)
		}
	}

	return ts, frameDone
}

// pumpAudio resolves recorded events onto the sample grid and pushes them to
// the device. Called once per frame or slice -- never ahead of emulation, so the
// sources' event lists are always complete for the window being resolved.
//
// The AY needs no ResetEvents here: it generates no events. The mixer advances
// it window-by-window through MeanLevel, and register writes are applied as
// their timestamps are reached.
func (e *Emulator) pumpAudio() {
	e.mixer.Resolve(e.totalTicks)
	e.beeper.ResetEvents()
	e.covox.ResetEvents()
	if e.fastTapeActive() {
		// Unthrottled tape load: resolve so the grid stays anchored to emulated
		// time, then throw the samples away. Pushing them would hand the device
		// a backlog the length of the whole tape, which it would then play out
		// long after the load finished.
		e.mixer.Discard()
		return
	}
	e.mixer.Drain()
}

// runLoop provides unified loop logic for both RunFrame and RunSlice (AUDIT T7).
// The condition function should return false to terminate the loop (for slice termination).
// The onComplete callback is called when a frame completes.
// Returns the total T-states executed and whether a frame completed.
func (e *Emulator) runLoop(condition func(int) bool, onComplete func()) (int, bool) {
	executed := 0

	for {
		ts, done := e.step()
		executed += ts

		if done {
			e.pumpAudio()
			onComplete()
			return executed, true
		}

		if !condition(executed) {
			e.pumpAudio()
			return executed, false
		}
	}
}

// RunFrame runs until the next frame interrupt.
func (e *Emulator) RunFrame() {
	e.runLoop(func(_ int) bool { return true }, func() {})
}

// RunSlice executes instructions for a time slice (~10ms), throttling based on
// audio generation to maintain real-time synchronization. Returns true if a frame
// completed during this slice.
func (e *Emulator) RunSlice() bool {
	const timeSlice = 10 * time.Millisecond
	tStatesPerSlice := e.cpuHz / int(time.Second/timeSlice)

	frameReady := false
	executed, _ := e.runLoop(
		func(executed int) bool {
			return executed < tStatesPerSlice
		},
		func() {
			frameReady = true
		},
	)

	// Fast tape load: no master clock at all while the tape plays. Everything
	// the emulator produces now is ahead of real time by design, so there is
	// nothing to pace against -- pumpAudio drops the samples.
	if e.fastTapeActive() {
		return frameReady
	}

	// Pace the emulator against a master clock. With a real audio device the
	// device's playback backlog is the master (see throttleToAudioClock); a
	// NullOutput falls back to a wall-clock deadline so headless runs still
	// pace to real time instead of spinning.
	if e.audioOut != nil && e.audioOut.HasClock() {
		e.throttleToAudioClock()
	} else {
		e.throttleToWallClock(executed)
	}

	return frameReady
}

// audioHighWaterMS is the target playback backlog held in the audio device. The
// emulator sleeps while the device holds more than this, so the device's clock
// -- not wall-clock -- is the master. This self-corrects for host sound-card
// clock drift, which a wall-clock deadline cannot (the two clocks drift apart
// and click). Keeping it at the previous ring-target depth holds total latency
// steady.
const audioHighWaterMS = 50

// throttleToAudioClock paces emulation against the audio device's playback
// backlog. It is a closed-loop master clock: production runs only as fast as
// the device drains, so overrun (drop) and underrun clicks are impossible
// regardless of host-clock drift. It also naturally recovers from a stall --
// a paused device drains its queue to zero, so no debt is ever repaid.
func (e *Emulator) throttleToAudioClock() {
	highWater := int64(e.audioOut.SampleRate()) * audioHighWaterMS / 1000
	for {
		d := audioClockSleep(int64(e.audioOut.Queued()), highWater, e.audioOut.SampleRate())
		if d == 0 {
			return
		}
		time.Sleep(d)
	}
}

// audioClockSleep returns how long to sleep to drain the queued backlog down to
// the high-water mark (both in frames at sampleRate). 0 means no sleep. Clamped
// to [1ms, 20ms] so a deep queue is drained in responsive slices rather than one
// long block. Pure, so it can be pinned by tests.
func audioClockSleep(queued, highWater int64, sampleRate int) time.Duration {
	if queued <= highWater {
		return 0
	}
	d := time.Duration(queued-highWater) * time.Second / time.Duration(sampleRate)
	if d < time.Millisecond {
		d = time.Millisecond
	}
	if d > 20*time.Millisecond {
		d = 20 * time.Millisecond
	}
	return d
}

// throttleToWallClock is the fallback pacing for a backend with no real device
// (NullOutput). A virtual deadline advances by exactly the emulated duration
// each slice; anchoring on the deadline (not time.Now) means a sleep overshoot
// is repaid by the next slice sleeping less.
func (e *Emulator) throttleToWallClock(executed int) {
	emulated := time.Duration(float64(executed) * float64(time.Second) / float64(e.cpuHz))
	const spinMargin = 2 * time.Millisecond
	if e.lastRealTime.IsZero() {
		e.lastRealTime = time.Now()
	}
	deadline := e.lastRealTime.Add(emulated)
	for {
		d := time.Until(deadline)
		if d <= 0 {
			break
		}
		if d > 2*spinMargin {
			time.Sleep(d - spinMargin)
		} else {
			for time.Now().Before(deadline) {
			}
			break
		}
	}
	e.lastRealTime = deadline
}
