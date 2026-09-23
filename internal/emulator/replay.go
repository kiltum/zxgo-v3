package emulator

import (
	"github.com/kiltum/zxgo-v3/pkg/replay"
)

// Input recording and playback.
//
// A session is captured by hanging a recorder off the emulator's own input API
// (PressKey, ReleaseKey, the tape transport) and played back by draining a
// player at the top of step(). Both halves are here rather than in the hosting
// loop because the loop is not the only way into a machine: the MCP worker
// presses keys through exactly the same two methods, and anything else that
// grows an input path should be recordable without touching it.
//
// The property that makes a replay worth anything is that it reproduces the
// session rather than approximating it, and that comes from the unit: an event
// is stamped with Emulator.totalTicks, and step() applies every event due at or
// before the tick of the instruction it is about to execute. So the injection
// point is the instruction boundary the press arrived at, not the nearest frame
// or millisecond.

// SetRecorder starts capturing input into r. Passing nil stops capture.
//
// Recording is off by default: a machine that is not being recorded should not
// pay an append per key press, and a library user should not have to opt out.
func (e *Emulator) SetRecorder(r *replay.Recorder) { e.recorder = r }

// Recorder returns the attached recorder, or nil.
func (e *Emulator) Recorder() *replay.Recorder { return e.recorder }

// SetPlayer attaches a replay to play back, or nil to detach.
//
// The player is drained from step(), so attaching it after the machine has been
// running leaves the events before the current tick unplayed -- a replay is
// meant to be attached before the run loop starts, at tick 0.
func (e *Emulator) SetPlayer(p *replay.Player) { e.player = p }

// Player returns the attached player, or nil.
func (e *Emulator) Player() *replay.Player { return e.player }

// CPUHz returns the machine's clock in T-states per second. A replay needs it to
// rescale its timestamps when it is played on a model other than the one it was
// recorded on (the three models differ by up to 2.4%).
func (e *Emulator) CPUHz() int { return e.cpuHz }

// recordTape captures a transport change, if a recorder is attached.
func (e *Emulator) recordTape(action string) {
	if e.recorder != nil {
		e.recorder.Tape(e.totalTicks, action)
	}
}

// applyReplayEvent feeds one recorded event into the machine.
//
// Keys go through PressKey/ReleaseKey and the joystick through SetJoystick
// rather than straight to the ULA and the port so that a session played back
// while a recorder is attached re-records itself; the two are not combined by
// the CLI, but nothing here depends on that.
func (e *Emulator) applyReplayEvent(ev replay.Event) {
	switch {
	case ev.Key != nil:
		if ev.Key.Down {
			e.PressKey(ev.Key.Row, ev.Key.Col)
		} else {
			e.ReleaseKey(ev.Key.Row, ev.Key.Col)
		}
	case ev.Joy != nil:
		j := ev.Joy
		e.SetJoystick(j.Right, j.Left, j.Down, j.Up, j.Fire)
	case ev.Tape == replay.TapePlay:
		e.StartTapePlayback()
	case ev.Tape == replay.TapePause:
		e.StopTapePlayback()
	}
}

// applyReplay drains the player up to the current tick. Called at the top of
// step(), before the bus is ticked and before the instruction executes, so an
// injected key is settled for the whole instruction -- exactly as a host press
// arriving between instructions would be.
func (e *Emulator) applyReplay() {
	if e.player != nil {
		e.player.Apply(e.totalTicks, e.applyReplayEvent)
	}
}
