package emulator

import (
	"os"
	"testing"

	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/sound"
)

// These tests run the real 48K ROM through the whole chain -- CPU, ULA, memory,
// ports, beeper, mixer -- with no display. They replace the smoke-test role the
// old headless PNG/WAV dump played (AUDIT D6), but assert on invariants rather
// than on a frame that needed a human to judge.

func newTestEmu(t *testing.T) *Emulator {
	t.Helper()
	rom, err := os.ReadFile("../../roms/48.rom")
	if err != nil {
		t.Skipf("48K ROM not available: %v", err)
	}
	e := New(model.Spectrum48K, &sound.NullOutput{})
	if err := e.LoadROM(0, rom); err != nil {
		t.Fatalf("load ROM: %v", err)
	}
	e.Reset()
	return e
}

// The anti-drift invariant, end to end: after real execution the number of
// samples generated must be fixed by elapsed T-states alone.
func TestBootGeneratesExactSampleCount(t *testing.T) {
	e := newTestEmu(t)

	const frames = 200
	for i := 0; i < frames; i++ {
		e.RunFrame()
	}

	rate := int64(e.Mixer().SampleRate())
	exact := float64(e.totalTicks) * float64(rate) / float64(e.cpuHz)
	got := e.Mixer().Generated()

	t.Logf("%d frames: %d T-states, %d samples generated (expected %.1f), %d dropped",
		frames, e.totalTicks, got, exact, e.Mixer().Dropped())

	if d := float64(got) - exact; d < -1 || d > 1 {
		t.Errorf("sample count drifted %+.1f from %.1f", d, exact)
	}
	if e.Mixer().Dropped() != 0 {
		t.Errorf("dropped %d frames during per-frame pumping", e.Mixer().Dropped())
	}
	if e.Mixer().Resyncs() != 0 {
		t.Errorf("unexpected grid resync during normal execution: %d", e.Mixer().Resyncs())
	}
}

// Proves the ROM actually executes and the ULA draws: a booted 48K shows the
// copyright line, so the framebuffer must contain more than one colour.
func TestBootReachesBasicPrompt(t *testing.T) {
	e := newTestEmu(t)
	for i := 0; i < 200; i++ {
		e.RunFrame()
	}

	seen := map[uint32]int{}
	for _, px := range e.ULA().GetScreen() {
		seen[px]++
	}
	if len(seen) < 2 {
		t.Errorf("framebuffer has %d distinct colour(s); ROM did not draw", len(seen))
	}
	if e.CPU().PC == 0 {
		t.Error("PC is 0; CPU did not execute")
	}
	t.Logf("%d distinct colours, PC=%04x, SP=%04x", len(seen), e.CPU().PC, e.CPU().SP)
}

// Reset must re-anchor the audio grid. Leaving the tick clock running across a
// reset made the first chunk of audio look like a multi-second gap (AUDIT S3).
func TestResetReanchorsAudioGrid(t *testing.T) {
	e := newTestEmu(t)
	for i := 0; i < 20; i++ {
		e.RunFrame()
	}
	if e.totalTicks == 0 {
		t.Fatal("no ticks elapsed before reset")
	}

	e.Reset()
	if e.totalTicks != 0 {
		t.Errorf("totalTicks = %d after Reset, want 0", e.totalTicks)
	}
	if g := e.Mixer().Generated(); g != 0 {
		t.Errorf("mixer generated = %d after Reset, want 0", g)
	}

	const frames = 20
	for i := 0; i < frames; i++ {
		e.RunFrame()
	}
	if e.Mixer().Resyncs() != 0 {
		t.Errorf("grid resynced %d time(s) after reset; clock was not re-anchored",
			e.Mixer().Resyncs())
	}
	exact := float64(e.totalTicks) * float64(e.Mixer().SampleRate()) / float64(e.cpuHz)
	if d := float64(e.Mixer().Generated()) - exact; d < -1 || d > 1 {
		t.Errorf("post-reset drift %+.1f from %.1f", d, exact)
	}
}

// SetCPUType selects the variant on the chip in the machine and does nothing else. That is
// what the settings window's Z80 row needs: it applies live and resets the machine itself
// (UI_DESIGN.md section 6.4), so a reset hidden in here would be a second one - and a
// startup applying the variant to a machine it has just restored from a session would lose
// that session entirely.
func TestSetCPUTypeDoesNotReset(t *testing.T) {
	// The machine is built from the embedded ROM (see newModel) rather than from roms/48.rom:
	// this test has nothing to do with which ROM is on disk, and a test that skips when the
	// file is missing is a test that never runs.
	e := newModel(t, "48k")
	for i := 0; i < 20; i++ {
		e.RunFrame()
	}
	if !e.IsNMOS() {
		t.Fatal("test precondition: the machine should start on the default NMOS")
	}
	ticks, frames := e.TotalTicks(), e.FrameCount()

	e.SetCPUType(false)

	if e.IsNMOS() {
		t.Error("the CPU is still NMOS after CMOS was selected")
	}
	if e.TotalTicks() != ticks || e.FrameCount() != frames {
		t.Errorf("the machine moved: %d ticks, %d frames; want %d and %d",
			e.TotalTicks(), e.FrameCount(), ticks, frames)
	}

	// And it goes back: the setting is not a one-way switch, since the settings window
	// offers both variants as its own buttons.
	e.SetCPUType(true)
	if !e.IsNMOS() {
		t.Error("the CPU did not go back to NMOS")
	}
}

// RunFrame and RunSlice must agree: same emulation, different chunking (AUDIT T7).
func TestRunSliceMatchesRunFrameGrid(t *testing.T) {
	e := newTestEmu(t)
	for i := 0; i < 30; i++ {
		e.RunSlice()
	}
	exact := float64(e.totalTicks) * float64(e.Mixer().SampleRate()) / float64(e.cpuHz)
	got := e.Mixer().Generated()
	t.Logf("RunSlice: %d T-states, %d samples (expected %.1f), %d dropped",
		e.totalTicks, got, exact, e.Mixer().Dropped())

	if d := float64(got) - exact; d < -1 || d > 1 {
		t.Errorf("RunSlice drift %+.1f from %.1f", d, exact)
	}
	if e.Mixer().Dropped() != 0 {
		t.Errorf("RunSlice dropped %d frames", e.Mixer().Dropped())
	}
}

// The beeper must not accumulate unresolved events across a long run.
func TestBeeperEventsDoNotAccumulate(t *testing.T) {
	e := newTestEmu(t)
	for i := 0; i < 200; i++ {
		e.RunFrame()
	}
	if p := e.Beeper().Pending(); p > 8 {
		t.Errorf("%d beeper events queued after 200 frames; not draining", p)
	}
}

// Test48KNoPagingPort verifies a 48K machine does not respond to the 0x7FFD
// paging port: writing it must not page a RAM bank into 0xC000. Real 48K
// hardware has no paging ports; only the TR-DOS ROM switch (via M1 fetch) and
// not a port write changes the mapping.
func Test48KNoPagingPort(t *testing.T) {
	e := New(model.Spectrum48K, &sound.NullOutput{})
	m := e.Mapper()

	m.WriteByte(0xC000, 0xAA)
	e.WritePort(0x7FFD, 0x07) // D0-D2 = 7 (128K would page bank 7 into 0xC000)

	if got := m.ReadByte(0xC000); got != 0xAA {
		t.Errorf("48K: 0x7FFD write paged RAM (0xC000 = 0x%02X, want 0xAA)", got)
	}
}
