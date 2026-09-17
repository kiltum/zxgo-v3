package emulator

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/kiltum/zxgo-v3/pkg/media"
	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/replay"
	"github.com/kiltum/zxgo-v3/pkg/sound"
)

// keyScript is a press and release of one matrix cell at a chosen frame.
type keyScript struct {
	frame    int
	row, col int
	down     bool
}

// recordSession runs frames on a machine while applying script, recording the
// input as it goes, and returns the .replay document that describes it.
func recordSession(t *testing.T, e *Emulator, frames int, script []keyScript) *replay.File {
	t.Helper()

	rec := replay.NewRecorder()
	e.SetRecorder(rec)

	byFrame := make(map[int][]keyScript, len(script))
	for _, ev := range script {
		byFrame[ev.frame] = append(byFrame[ev.frame], ev)
	}

	for f := 0; f < frames; f++ {
		for _, ev := range byFrame[f] {
			if ev.down {
				e.PressKey(ev.row, ev.col)
			} else {
				e.ReleaseKey(ev.row, ev.col)
			}
		}
		e.RunFrame()
	}

	doc := replay.NewFile()
	doc.Model = "48k"
	doc.ModelName = e.ModelConfig().Name
	doc.CPUHz = e.CPUHz()
	doc.Events = rec.Events()
	return doc
}

// replaySession runs frames on a machine driven by doc, with no recorder.
func replaySession(t *testing.T, e *Emulator, doc *replay.File, frames int) {
	t.Helper()

	// Through the file, so what is played is what would be read from disk.
	var buf bytes.Buffer
	if err := replay.Save(&buf, doc); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := replay.Load(&buf)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	e.SetPlayer(replay.NewPlayer(loaded.Events, loaded.CPUHz, e.CPUHz()))

	for f := 0; f < frames; f++ {
		e.RunFrame()
	}
	if !e.Player().Done() {
		t.Errorf("replay finished with %d events unplayed", e.Player().Remaining())
	}
}

// TestReplayReproducesSession is the property the whole feature rests on: a
// recorded session played back into a fresh machine leaves it in exactly the
// state it was recorded in.
//
// The comparison is the whole SNA image rather than a few registers, and a third
// machine runs the same frames with no input as a control: if the keystrokes had
// changed nothing, the control would match and the test would be proving an
// empty claim.
//
// The presses start at frame 120 because the 48K ROM is still running its RAM
// test and printing the copyright before that -- a key pressed at frame 20
// reaches the matrix and is never looked at, which made an earlier version of
// this test pass while proving nothing.
func TestReplayReproducesSession(t *testing.T) {
	cfg := model.Spectrum48K
	const frames = 260
	// Row/col of W, S and K on the 8x5 matrix -- ordinary keys the ROM's editor
	// will consume and echo to the screen, so the presses reach memory.
	script := []keyScript{
		{frame: 120, row: 2, col: 1, down: true},
		{frame: 126, row: 2, col: 1, down: false},
		{frame: 150, row: 1, col: 1, down: true},
		{frame: 156, row: 1, col: 1, down: false},
		{frame: 180, row: 6, col: 2, down: true},
		{frame: 186, row: 6, col: 2, down: false},
	}

	recorded := New(cfg, &sound.NullOutput{})
	doc := recordSession(t, recorded, frames, script)

	if len(doc.Events) != len(script) {
		t.Fatalf("recorded %d events, want %d", len(doc.Events), len(script))
	}
	// Events are stamped from the frame boundary they were applied at, so the
	// first is at a non-zero tick and they are in order.
	for i := 1; i < len(doc.Events); i++ {
		if doc.Events[i].Tick < doc.Events[i-1].Tick {
			t.Fatalf("events out of order at %d: %d then %d",
				i, doc.Events[i-1].Tick, doc.Events[i].Tick)
		}
	}
	if doc.Events[0].Tick <= 0 {
		t.Errorf("first event tick = %d, want a positive tick", doc.Events[0].Tick)
	}

	played := New(cfg, &sound.NullOutput{})
	replaySession(t, played, doc, frames)

	if got, want := played.TotalTicks(), recorded.TotalTicks(); got != want {
		t.Errorf("replay ran %d T-states, recorded %d", got, want)
	}
	if got, want := saveBytes(t, played), saveBytes(t, recorded); !bytes.Equal(got, want) {
		t.Errorf("replayed machine differs from the recorded one (%d vs %d bytes)%s",
			len(got), len(want), firstDiff(got, want))
	}

	// Control: the same frames with no input must NOT match, or the comparison
	// above would pass even if the events were dropped entirely.
	control := New(cfg, &sound.NullOutput{})
	for f := 0; f < frames; f++ {
		control.RunFrame()
	}
	if bytes.Equal(saveBytes(t, control), saveBytes(t, recorded)) {
		t.Error("the recorded session left the machine identical to no input at all: " +
			"the test cannot detect a dropped event")
	}
}

// firstDiff renders the offset of the first differing byte, for a failure
// message that points at the divergence instead of dumping two images.
func firstDiff(a, b []byte) string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return fmt.Sprintf(" (first difference at byte %d)", i)
		}
	}
	if len(a) != len(b) {
		return " (lengths differ)"
	}
	return ""
}

// TestReplayReproducesFastTapeLoad covers the case that decides the time base:
// with -fast-tape the emulator runs unthrottled, so wall-clock time and emulated
// time part company completely. Recording in T-states is what survives it -- the
// tape is where it would be at the same tick on any host.
func TestReplayReproducesFastTapeLoad(t *testing.T) {
	const tapePath = "../../testdata/batty.tap"
	if _, err := os.Stat(tapePath); err != nil {
		t.Skipf("testdata %s not available", tapePath)
	}

	newMachine := func(t *testing.T) *Emulator {
		t.Helper()
		e := New(model.Spectrum48K, &sound.NullOutput{})
		tape, err := media.LoadTapeFile(tapePath)
		if err != nil {
			t.Fatalf("LoadTapeFile: %v", err)
		}
		if err := e.LoadTape(tape); err != nil {
			t.Fatalf("LoadTape: %v", err)
		}
		e.SetFastTape(true)
		// A key held before the burst and released inside it: the burst is where
		// host time and emulated time diverge most, so an event on either side of
		// it is what a wall-clock stamp would misplace.
		return e
	}

	const frames = 240
	script := []keyScript{
		{frame: 0, row: 7, col: 0, down: true}, // SPACE, held into the burst
		{frame: 5, row: 7, col: 0, down: false},
		{frame: 200, row: 6, col: 0, down: true}, // ENTER, after it
		{frame: 204, row: 6, col: 0, down: false},
	}

	recorded := newMachine(t)
	rec := replay.NewRecorder()
	recorded.SetRecorder(rec)
	recorded.StartTapePlayback() // recorded at tick 0, the origin a replay starts from

	byFrame := make(map[int][]keyScript, len(script))
	for _, ev := range script {
		byFrame[ev.frame] = append(byFrame[ev.frame], ev)
	}
	for f := 0; f < frames; f++ {
		for _, ev := range byFrame[f] {
			if ev.down {
				recorded.PressKey(ev.row, ev.col)
			} else {
				recorded.ReleaseKey(ev.row, ev.col)
			}
		}
		recorded.RunFrame()
	}

	// The tape event must have been captured, or the replay would run silent.
	var tapeEvents int
	for _, ev := range rec.Events() {
		if ev.Tape != "" {
			tapeEvents++
		}
	}
	if tapeEvents != 1 {
		t.Fatalf("recorded %d tape events, want 1 (the play)", tapeEvents)
	}

	doc := replay.NewFile()
	doc.Model = "48k"
	doc.CPUHz = recorded.CPUHz()
	doc.Settings.FastTape = true
	doc.Events = rec.Events()

	played := newMachine(t)
	replaySession(t, played, doc, frames)

	if got, want := played.TotalTicks(), recorded.TotalTicks(); got != want {
		t.Errorf("replay ran %d T-states, recorded %d", got, want)
	}
	if got, want := saveBytes(t, played), saveBytes(t, recorded); !bytes.Equal(got, want) {
		t.Errorf("replayed machine differs from the recorded one (%d vs %d bytes)%s",
			len(got), len(want), firstDiff(got, want))
	}

	// The tape is the part a wall-clock basis would have got wrong: its position
	// is a function of the ticks that passed while it played.
	ra, rb := recorded.TapePlayback(), played.TapePlayback()
	if ra == nil || rb == nil {
		t.Fatal("tape playback missing")
	}
	if ra.Pos != rb.Pos {
		t.Errorf("tape position %d after replay, want %d", rb.Pos, ra.Pos)
	}
	if ra.TapeTick != rb.TapeTick {
		t.Errorf("tape tick %d after replay, want %d", rb.TapeTick, ra.TapeTick)
	}
	if ra.Active != rb.Active {
		t.Errorf("tape active = %v after replay, want %v", rb.Active, ra.Active)
	}
}

// TestReplayRecordsTapeTransport: the transport is not a key, and a recording
// that could not express it would be unable to reproduce a tape load.
func TestReplayRecordsTapeTransport(t *testing.T) {
	e := New(model.Spectrum48K, &sound.NullOutput{})

	// No recorder attached: the transport must still work.
	e.StartTapePlayback()
	e.StopTapePlayback()

	tape, err := media.LoadTapeFile("../../testdata/batty.tap")
	if err != nil {
		t.Skipf("testdata not available: %v", err)
	}
	if err := e.LoadTape(tape); err != nil {
		t.Fatalf("LoadTape: %v", err)
	}

	rec := replay.NewRecorder()
	e.SetRecorder(rec)
	e.RunFrame()

	e.StartTapePlayback()
	e.RunFrame()
	e.RunFrame()
	e.StopTapePlayback()
	e.RunFrame()

	events := rec.Events()
	if len(events) != 2 {
		t.Fatalf("recorded %d events, want 2 (play and pause)", len(events))
	}
	if events[0].Tape != replay.TapePlay {
		t.Errorf("first event = %+v, want a play", events[0])
	}
	if events[1].Tape != replay.TapePause {
		t.Errorf("second event = %+v, want a pause", events[1])
	}
	if events[0].Tick >= events[1].Tick {
		t.Errorf("play at %d, pause at %d: the pause must come later",
			events[0].Tick, events[1].Tick)
	}
}

// A replay records itself too: playback goes through the same input methods, so
// attaching both is a re-record rather than a special case.
func TestRecordingWhilePlaying(t *testing.T) {
	cfg := model.Spectrum48K

	source := New(cfg, &sound.NullOutput{})
	doc := recordSession(t, source, 60, []keyScript{
		{frame: 10, row: 2, col: 1, down: true},
		{frame: 12, row: 2, col: 1, down: false},
	})

	// Machine B plays the recording and records what it plays.
	second := New(cfg, &sound.NullOutput{})
	rec := replay.NewRecorder()
	second.SetRecorder(rec)
	second.SetPlayer(replay.NewPlayer(doc.Events, doc.CPUHz, second.CPUHz()))
	for f := 0; f < 60; f++ {
		second.RunFrame()
	}

	got := rec.Events()
	if len(got) != len(doc.Events) {
		t.Fatalf("re-recorded %d events, want %d", len(got), len(doc.Events))
	}
	for i := range got {
		if got[i].Tick != doc.Events[i].Tick {
			t.Errorf("event %d re-recorded at tick %d, want %d", i, got[i].Tick, doc.Events[i].Tick)
		}
		if got[i].Key == nil || doc.Events[i].Key == nil {
			t.Fatalf("event %d kind changed: %+v vs %+v", i, got[i], doc.Events[i])
		}
		if *got[i].Key != *doc.Events[i].Key {
			t.Errorf("event %d = %+v, want %+v", i, *got[i].Key, *doc.Events[i].Key)
		}
	}
}

// A player left attached past the end of its events must not disturb the
// machine: the session carries on with no input, which is what a user expects
// when a replay runs out mid-play.
func TestPlayerPastEndIsInert(t *testing.T) {
	e := New(model.Spectrum48K, &sound.NullOutput{})
	e.SetPlayer(replay.NewPlayer(nil, 3500000, e.CPUHz()))
	for f := 0; f < 30; f++ {
		e.RunFrame()
	}

	control := New(model.Spectrum48K, &sound.NullOutput{})
	for f := 0; f < 30; f++ {
		control.RunFrame()
	}

	if got, want := saveBytes(t, e), saveBytes(t, control); !bytes.Equal(got, want) {
		t.Error("an empty player changed the machine")
	}
}
