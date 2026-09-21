package frontend

import (
	"testing"
	"time"

	"github.com/kiltum/zxgo-v3/pkg/model"
)

// Every action the registry can name has to have a handler. An action declared
// without one is a shortcut that silently does nothing, which is worse than a
// shortcut that is missing: the user cannot tell it from a bug.
func TestEveryActionIsDispatched(t *testing.T) {
	app, _ := testApp()
	// The screenshot action writes a file, and a test that leaves one in the
	// package directory is a test that litters the tree on every run.
	app.ScreenshotDir = t.TempDir()
	for act := range actionNames {
		if act == ActNone {
			continue
		}
		if err := app.Dispatch(act); err != nil {
			t.Errorf("Dispatch(%v): %v", act, err)
		}
	}

	// And an action outside the registry is refused rather than ignored.
	if err := app.Dispatch(Action(9999)); err == nil {
		t.Error("an unknown action was accepted")
	}
}

// Run state transitions, which is the first of the four things M1 is done when it
// has covered: running advances the machine, paused does not, and both are
// reachable from the action the user has.
func TestRunStateTransitions(t *testing.T) {
	app, m := testApp()
	now := time.Now()

	if app.RunState() != StateRun {
		t.Fatalf("a fresh app is %v, want running", app.RunState())
	}
	app.Tick(now)
	if m.slices == 0 {
		t.Error("a running machine did not run")
	}

	app.Dispatch(ActPauseToggle)
	if app.RunState() != StatePaused {
		t.Fatalf("after the pause action the state is %v", app.RunState())
	}
	slices := m.slices
	app.Tick(now)
	if m.slices != slices {
		t.Error("a paused machine ran")
	}
	if m.ticks == 0 {
		t.Error("pausing threw the machine's clock away")
	}

	app.Dispatch(ActPauseToggle)
	if app.RunState() != StateRun {
		t.Fatalf("the second pause action left the state at %v", app.RunState())
	}
	app.Tick(now)
	if m.slices == slices {
		t.Error("the machine did not resume")
	}
}

// A breakpoint and the pause action take the same transition, so there is one
// place that knows how to stop (section 5.2).
func TestPauseIsTheOneWayToStop(t *testing.T) {
	app, _ := testApp()

	app.Pause() // what a breakpoint will call at M8
	if app.RunState() != StatePaused {
		t.Errorf("Pause left the state at %v", app.RunState())
	}
	if err := app.Dispatch(ActPauseToggle); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if app.RunState() != StateRun {
		t.Errorf("the pause action left the state at %v", app.RunState())
	}
}

// A reset clears the machine and the frame-rate measurement together: a rate
// measured across a reset is a rate of nothing.
func TestResetClearsTheMachineAndTheRate(t *testing.T) {
	app, m := testApp()
	app.Pause()
	base := time.Now()
	app.Tick(base)
	m.frames = 10
	app.Tick(base.Add(time.Second))
	if app.Views().Status.FPS == 0 {
		t.Fatal("the frame rate never measured anything")
	}

	app.Dispatch(ActReset)
	app.Tick(base.Add(2 * time.Second)) // the views are as fresh as the last tick

	if m.resets != 1 {
		t.Errorf("resets = %d, want 1", m.resets)
	}
	if got := app.Views().Status.FPS; got != 0 {
		t.Errorf("fps = %v after a reset, want 0", got)
	}
}

// D7: the frame rate counts emulated frames, so a fast load reports the speed it
// is really running at rather than the refresh rate. The meter is fed by the
// tick, so a test drives it with explicit times and never sleeps.
func TestStatusFrameRate(t *testing.T) {
	app, m := testApp()
	app.Pause() // the machine's own frame counter is driven by hand here
	base := time.Now()

	app.Tick(base)
	m.frames = 10
	app.Tick(base.Add(time.Second))
	if got := app.Views().Status.FPS; got != 10 {
		t.Errorf("fps = %v, want 10", got)
	}

	// A tick inside the next second leaves the estimate alone rather than
	// flickering between two numbers that differ in the last digit.
	m.frames = 11
	app.Tick(base.Add(time.Second + 100*time.Millisecond))
	if got := app.Views().Status.FPS; got != 10 {
		t.Errorf("fps = %v after 100 ms, want the previous estimate", got)
	}
}

// The status view is what the menubar shows, so it has to report the machine as
// it is rather than as the front end remembers it.
func TestStatusView(t *testing.T) {
	app, m := testApp()
	m.fastTape = true
	app.Tick(time.Now())

	s := app.Views().Status
	if s.Run != StateRun {
		t.Errorf("run = %v, want running", s.Run)
	}
	if s.Model != model.Spectrum48K.Name {
		t.Errorf("model = %q, want %q", s.Model, model.Spectrum48K.Name)
	}
	if s.CPUHz != m.CPUHz() {
		t.Errorf("cpu = %d, want %d", s.CPUHz, m.CPUHz())
	}
	if s.Ticks != m.TotalTicks() || s.Frames != m.FrameCount() {
		t.Errorf("clock = %d/%d, want %d/%d", s.Ticks, s.Frames, m.TotalTicks(), m.FrameCount())
	}
	if !s.FastTape {
		t.Error("the status view does not say the machine is loading fast")
	}
}

// D9 and the gates of D5: a matrix key reaches the machine only while the main
// window has focus and nothing in front of it wants the keys.
//
// Which window the key was typed into comes with the key rather than being read from
// App's focus, and this is why: a focus change and the first key of the window that
// gained it can arrive in the same poll, and reading the focus would put that first
// keystroke into the machine.
func TestMatrixKeysRespectFocusAndCapture(t *testing.T) {
	app, m := testApp()

	app.HandleKey(ToolMain, KeyA, 0, true)
	if len(m.pressed) != 1 {
		t.Fatalf("the main window's keys did not reach the machine: %v", m.pressed)
	}

	// The same key, from a tool window: the machine does not hear it (rule 1).
	app.HandleKey(ToolDebugger, KeyS, 0, true)
	if len(m.pressed) != 1 {
		t.Error("the machine heard a key typed into a tool window")
	}

	app.SetCapturing(true)
	app.HandleKey(ToolMain, KeyD, 0, true)
	if len(m.pressed) != 1 {
		t.Error("the machine heard a key while a widget wanted the keyboard")
	}
	app.SetCapturing(false)

	app.SetDialogOpen(true)
	app.HandleKey(ToolMain, KeyF, 0, true)
	if len(m.pressed) != 1 {
		t.Error("the machine heard a key while a file dialog was up")
	}
	app.SetDialogOpen(false)

	app.HandleKey(ToolMain, KeyG, 0, true)
	if len(m.pressed) != 2 {
		t.Errorf("the machine did not hear a key once the gates lifted: %v", m.pressed)
	}
}

// The other half of rule 1: the key that a tool window swallowed is *not* left
// behind. A press the machine never saw must not be followed by a release it does,
// or the recording gets a key-up for a key that was never down.
func TestAToolWindowDoesNotLeaveAStuckKey(t *testing.T) {
	app, m := testApp()

	// Typed into the debugger, press and release.
	app.HandleKey(ToolDebugger, KeyA, 0, true)
	app.HandleKey(ToolDebugger, KeyA, 0, false)

	if len(m.pressed) != 0 || len(m.released) != 0 {
		t.Errorf("a tool window's keystrokes reached the machine: %v / %v", m.pressed, m.released)
	}

	// And the machine is not holding anything after a press it did see, when focus
	// goes away mid-press (D5 rule 4).
	app.HandleKey(ToolMain, KeyB, 0, true)
	app.Apply([]Event{{Kind: EvFocusChange, Tool: ToolDebugger}})
	if len(m.released) != 1 {
		t.Errorf("focus loss did not release the held key: %v", m.released)
	}
	if len(m.down) != 0 {
		t.Errorf("the machine is still holding %v", m.down)
	}
}

// The three actions D5 rule 3 keeps live while something else has the keyboard.
func TestQuitPauseAndScreenshotSurviveCapture(t *testing.T) {
	app, _ := testApp()
	app.SetCapturing(true)

	if act, ok := app.HandleKey(ToolMain, KeyF5, 0, true); !ok || act != ActPauseToggle {
		t.Fatalf("pause under capture = %v (%v), want it to fire", act, ok)
	}
	if app.RunState() != StatePaused {
		t.Error("pause did not pause")
	}

	// A tool toggle does not fire: it would rearrange the screen the user is
	// typing into.
	if _, ok := app.HandleKey(ToolMain, KeyT, ModSuper, true); openIs(app, ToolTape) {
		t.Error("a tool toggle opened a window while a widget had the keyboard")
	} else if !ok {
		t.Error("the toggle's key was not eaten, so it would reach the machine")
	}
}

// Both edges of a host chord belong to the front end, so the machine never sees a
// key-up for a key it never saw go down (D5 rule 5).
func TestAChordEatsBothEdges(t *testing.T) {
	app, m := testApp()

	if act, ok := app.HandleKey(ToolMain, KeyP, ModSuper, true); !ok || act != ActTapePlayPause {
		t.Fatalf("Cmd+P = %v (%v)", act, ok)
	}
	// The release arrives with the modifier already up, which is exactly the
	// order that makes the swallow a memory rather than a recomputation.
	if act, ok := app.HandleKey(ToolMain, KeyP, 0, false); act != ActNone || !ok {
		t.Fatalf("the release of Cmd+P was not eaten: %v (%v)", act, ok)
	}
	if len(m.pressed) != 0 || len(m.released) != 0 {
		t.Errorf("the machine saw %v / %v from a host chord", m.pressed, m.released)
	}

	// P on its own is the machine's key, and both its edges get through.
	app.HandleKey(ToolMain, KeyP, 0, true)
	app.HandleKey(ToolMain, KeyP, 0, false)
	if len(m.pressed) != 1 || len(m.released) != 1 {
		t.Errorf("plain P = %v / %v, want one press and one release", m.pressed, m.released)
	}
}

// A cell that two sources hold is one ZX key: the release of either must not let
// go of a cell the other still holds (D5 rule 4).
func TestACellTwoSourcesHold(t *testing.T) {
	app, m := testApp()

	app.MatrixCell(6, 0, true) // a physical key
	app.MatrixCell(6, 0, true) // and an on-screen click on the same key
	if len(m.pressed) != 1 {
		t.Fatalf("pressed %d times, want 1: two sources are one key", len(m.pressed))
	}

	app.MatrixCell(6, 0, false)
	if len(m.released) != 0 {
		t.Error("the first release let go of a cell the other source still holds")
	}
	app.MatrixCell(6, 0, false)
	if len(m.released) != 1 {
		t.Error("the second release did not let go")
	}
}

// A press a gate suppressed leaves no trace, so its release is not delivered
// either: through D9 the recorder would otherwise write down a key-up for a key
// that was never pressed.
func TestASuppressedPressDoesNotLeakARelease(t *testing.T) {
	app, m := testApp()
	app.SetCapturing(true)

	app.HandleKey(ToolMain, KeyA, 0, true) // suppressed by the gate
	app.SetCapturing(false)

	if _, ok := app.HandleKey(ToolMain, KeyA, 0, false); ok {
		t.Error("the release of a suppressed press was claimed by the front end")
	}
	if len(m.released) != 0 {
		t.Errorf("the machine was released (%v) for a key it never saw pressed", m.released)
	}
}

// Losing focus releases what the front end is holding: SDL does not deliver the
// releases of keys that were held when focus went (D5 rule 4).
func TestLosingFocusReleasesHeldKeys(t *testing.T) {
	app, m := testApp()
	app.HandleKey(ToolMain, KeyA, 0, true)
	app.MatrixCell(6, 0, true)

	app.ReleaseHeld()

	if len(m.released) != 2 {
		t.Fatalf("released %v, want both held keys", m.released)
	}
	// The physical release still arrives afterwards and must not be delivered
	// twice, or the replay records a key-up that is already accounted for.
	app.HandleKey(ToolMain, KeyA, 0, false)
	if len(m.released) != 2 {
		t.Errorf("released %v after ReleaseHeld, want no more", m.released)
	}
}

// Turbo is the fast tape switch, and it is a machine property rather than a loop
// mode (D7).
func TestTurboTogglesFastTape(t *testing.T) {
	app, m := testApp()

	app.Dispatch(ActTurboToggle)
	if !m.fastTape {
		t.Error("turbo did not turn fast tape on")
	}
	app.Dispatch(ActTurboToggle)
	if m.fastTape {
		t.Error("turbo did not turn fast tape off")
	}
}

// Rewind goes through the transport, so a recording sees it (D9).
func TestTapeActionsGoThroughTheTransport(t *testing.T) {
	app, m := testApp()

	app.Dispatch(ActTapeRewind)
	if m.rewinds != 1 {
		t.Errorf("rewinds = %d, want 1", m.rewinds)
	}

	m.tapeMounted, m.tapePlaying = true, false
	app.Dispatch(ActTapePlayPause)
	if m.tapeStarts != 1 || !m.tapePlaying {
		t.Errorf("play: starts = %d, playing = %v", m.tapeStarts, m.tapePlaying)
	}
	app.Dispatch(ActTapePlayPause)
	if m.tapeStops != 1 || m.tapePlaying {
		t.Errorf("pause: stops = %d, playing = %v", m.tapeStops, m.tapePlaying)
	}
}

// The NMI action asks the machine for one non-maskable interrupt (M4's button, and
// the one thing in M4 that touches the CPU).
func TestNMIActionReachesTheMachine(t *testing.T) {
	app, m := testApp()

	app.Dispatch(ActNMI)
	if m.nmis != 1 {
		t.Errorf("nmis = %d, want 1", m.nmis)
	}

	// One press is one NMI: it is the machine's latch that clears, and the front end
	// does not hold a count of its own to get wrong.
	app.Dispatch(ActNMI)
	if m.nmis != 2 {
		t.Errorf("nmis = %d after a second press, want 2", m.nmis)
	}

	// And the key it is bound to fires it.
	if act, ok := app.HandleKey(ToolMain, KeyF10, 0, true); !ok || act != ActNMI {
		t.Errorf("F10 = %v (%v), want the NMI action", act, ok)
	}
	if m.nmis != 3 {
		t.Errorf("nmis = %d after F10, want 3", m.nmis)
	}
}
