package emulator

import (
	"testing"

	"github.com/kiltum/zxgo-v3/pkg/media"
	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/sound"
)

// countingAudio is a fake AudioOutput that records how the emulator treated it:
// how many frames were pushed, and how often the throttle asked how much
// playback backlog remains. Queued always reports an empty queue, so the
// throttle never sleeps and the test stays deterministic.
type countingAudio struct {
	rate       int
	pushed     int
	queuedCall int
}

func (a *countingAudio) Init() error                    { return nil }
func (a *countingAudio) PushSamples(l, r []int16) error { a.pushed += len(l); return nil }
func (a *countingAudio) SampleRate() int                { return a.rate }
func (a *countingAudio) Close() error                   { return nil }
func (a *countingAudio) Queued() int                    { a.queuedCall++; return 0 }
func (a *countingAudio) HasClock() bool                 { return true }

// fastTapeEmu builds a 48K emulator (embedded ROMs) wired to a counting output.
func fastTapeEmu(t *testing.T) (*Emulator, *countingAudio) {
	t.Helper()
	out := &countingAudio{rate: 44100}
	return New(model.Spectrum48K, out), out
}

// A tape that will not run out during the test: one long pulse pair repeated.
func longTestTape() *media.Tape {
	pulses := make([]media.Pulse, 0, 2048)
	for i := 0; i < 1024; i++ {
		pulses = append(pulses, media.Pulse{Level: true, Duration: 1000})
		pulses = append(pulses, media.Pulse{Level: false, Duration: 1000})
	}
	return &media.Tape{FileName: "test", Format: "TEST", Pulses: pulses}
}

// While fast tape is on and the tape is playing, RunSlice must neither consult
// the audio clock (the throttle is off) nor push samples to the device
// (everything produced is ahead of real time by design). Both come back the
// moment playback stops.
func TestFastTapeSkipsThrottleAndDropsAudio(t *testing.T) {
	e, out := fastTapeEmu(t)

	if err := e.LoadTape(longTestTape()); err != nil {
		t.Fatalf("load tape: %v", err)
	}
	e.SetFastTape(true)
	e.StartTapePlayback()

	for i := 0; i < 6; i++ {
		e.RunSlice()
	}
	if out.queuedCall != 0 {
		t.Errorf("throttle consulted %d times while fast-loading, want 0", out.queuedCall)
	}
	if out.pushed != 0 {
		t.Errorf("%d frames pushed to the device while fast-loading, want 0", out.pushed)
	}

	// Pausing the tape ends the fast path: pacing and audio resume.
	e.StopTapePlayback()
	for i := 0; i < 6; i++ {
		e.RunSlice()
	}
	if out.queuedCall == 0 {
		t.Error("throttle not consulted after playback stopped")
	}
	if out.pushed == 0 {
		t.Error("no audio pushed after playback stopped")
	}
}

// The fast path keys on playback state, not on the flag alone: a loaded but
// idle tape, and a tape that has run to its end, both leave the emulator
// throttled.
func TestFastTapeOnlyWhilePlaying(t *testing.T) {
	e, _ := fastTapeEmu(t)

	if e.fastTapeActive() {
		t.Error("active with no tape loaded")
	}

	if err := e.LoadTape(longTestTape()); err != nil {
		t.Fatalf("load tape: %v", err)
	}
	e.SetFastTape(true)
	if e.fastTapeActive() {
		t.Error("active while the tape is loaded but not playing")
	}

	e.StartTapePlayback()
	if !e.fastTapeActive() {
		t.Error("not active while the tape is playing")
	}

	e.StopTapePlayback()
	if e.fastTapeActive() {
		t.Error("still active after the tape was paused")
	}

	// With the feature off, a playing tape must not disable the throttle.
	e.SetFastTape(false)
	e.StartTapePlayback()
	if e.fastTapeActive() {
		t.Error("active with fast tape disabled")
	}
}

// NullOutput has no clock, so the wall-clock fallback paces headless runs. Fast
// tape must skip that too -- otherwise a headless load still takes tape time.
func TestFastTapeSkipsWallClockFallback(t *testing.T) {
	e := New(model.Spectrum48K, &sound.NullOutput{})
	if err := e.LoadTape(longTestTape()); err != nil {
		t.Fatalf("load tape: %v", err)
	}

	e.SetFastTape(true)
	e.StartTapePlayback()

	// Six unthrottled slices are microseconds of host time; the wall-clock
	// fallback would sleep them out to 60 ms of emulated time.
	for i := 0; i < 6; i++ {
		e.RunSlice()
	}
	if !e.lastRealTime.IsZero() {
		t.Error("wall-clock throttle ran during a fast tape load")
	}
}
