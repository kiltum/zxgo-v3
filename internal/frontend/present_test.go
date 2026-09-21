package frontend

import (
	"testing"
	"time"
)

// UI_DESIGN.md D7: a machine that is not loading a tape presents whenever a frame
// boundary arrives, and a fast load presents at most once per interval so the
// display cannot become the thing pacing it.
func TestPresentPolicyRunning(t *testing.T) {
	app, _ := testApp()
	now := time.Now()

	// The fake machine runs 10 ms slices of a 69888 T-state frame, so the second
	// slice completes one and the fourth completes the next: the policy has to
	// follow the machine rather than present on a schedule of its own.
	app.Tick(now)
	if app.ShouldPresent(now) {
		t.Error("presented before any frame boundary")
	}
	app.Tick(now)
	if !app.ShouldPresent(now) {
		t.Error("a frame boundary was not presented")
	}
	app.Tick(now)
	if app.ShouldPresent(now) {
		t.Error("presented between frame boundaries")
	}
	app.Tick(now)
	if !app.ShouldPresent(now) {
		t.Error("the second frame boundary was not presented")
	}
}

// A paused machine has no frame boundary and no new picture, and it still
// presents: the backend's VSync'd present is what stops the loop spinning, which
// is cheaper than a sleep and keeps the window live (D7).
func TestPresentPolicyPaused(t *testing.T) {
	app, m := testApp()
	now := time.Now()

	app.Pause()
	app.Tick(now)
	if !app.ShouldPresent(now) {
		t.Error("a paused machine did not present, so nothing would pace the loop")
	}
	if m.slices != 0 {
		t.Errorf("a paused machine ran %d slices, want none", m.slices)
	}
}

// A step executes one instruction and falls back to paused, so a user can walk
// the machine a instruction at a time without holding a key down.
func TestStepRunsOneInstructionAndPauses(t *testing.T) {
	app, m := testApp()
	now := time.Now()

	app.Dispatch(ActStepOne)
	if app.RunState() != StateStep {
		t.Fatalf("run state = %v, want step", app.RunState())
	}
	app.Tick(now)

	if m.steps != 1 {
		t.Errorf("ran %d instructions, want 1", m.steps)
	}
	if app.RunState() != StatePaused {
		t.Errorf("run state after a step = %v, want paused", app.RunState())
	}
	if !app.ShouldPresent(now) {
		t.Error("a stepped machine did not present")
	}
}

// A fast tape load is presented once and then throttled, because the whole point
// of the interval is that the display does not cap a load running at host speed.
func TestPresentPolicyFastTape(t *testing.T) {
	app, m := testApp()
	m.fastTape = true
	now := time.Now()

	if !app.ShouldPresent(now) {
		t.Error("the first frame of a fast load was not presented")
	}
	if app.ShouldPresent(now.Add(50 * time.Millisecond)) {
		t.Error("a second frame was presented inside the interval")
	}
	if !app.ShouldPresent(now.Add(DefaultPresentInterval)) {
		t.Error("no frame was presented when the interval had elapsed")
	}

	// The interval is a setting, not a constant.
	short, sm := testApp()
	sm.fastTape = true
	short.PresentInterval = 10 * time.Millisecond
	if !short.ShouldPresent(now) {
		t.Error("the first frame of a fast load was not presented")
	}
	if short.ShouldPresent(now.Add(9 * time.Millisecond)) {
		t.Error("presented inside the custom interval")
	}
	if !short.ShouldPresent(now.Add(10 * time.Millisecond)) {
		t.Error("the custom interval was not used")
	}

	// Leaving fast mode clears the clock, so the next fast load presents at once
	// rather than waiting out an interval left over from this one.
	m.fastTape = false
	app.ShouldPresent(now)
	m.fastTape = true
	if !app.ShouldPresent(now) {
		t.Error("the clock was not cleared on leaving fast mode")
	}
}

// A tape that runs off the end is announced once, and a tape that is started
// again re-arms the notice, so a second load still reports its own end.
func TestTapeNoticeAnnouncesOnce(t *testing.T) {
	app, m := testApp()

	if _, ok := app.TapeNotice(); ok {
		t.Error("a machine with no tape announced an end")
	}

	m.tapeMounted = true
	if _, ok := app.TapeNotice(); ok {
		t.Error("a running tape announced its end")
	}

	m.tapeEnded = true
	notice, ok := app.TapeNotice()
	if !ok {
		t.Fatal("the end of the tape was not announced")
	}
	if want := "Tape ended - playback stopped"; notice != want {
		t.Errorf("notice = %q, want %q", notice, want)
	}
	for i := 0; i < 3; i++ {
		if _, ok := app.TapeNotice(); ok {
			t.Errorf("the end of the tape was announced %d extra time(s)", i+1)
		}
	}

	// Rewound and played again: the next end is a new event.
	m.tapeEnded = false
	if _, ok := app.TapeNotice(); ok {
		t.Error("a restarted tape announced an end")
	}
	m.tapeEnded = true
	if _, ok := app.TapeNotice(); !ok {
		t.Error("the second end of the tape was not announced")
	}
}
