package media

import "testing"

// A block knows where it starts, and the state names the block the head is in.
func TestTapeStateNamesTheCurrentBlock(t *testing.T) {
	tape, err := LoadTapeFile("../../testdata/batty.tap")
	if err != nil {
		t.Skipf("testdata tape not available: %v", err)
	}
	pb := NewPlayback(tape)

	state := pb.StateSnapshot()
	if state.Format != "TAP" || state.Pulses == 0 || len(state.Blocks) == 0 {
		t.Fatalf("state = %+v, want a loaded TAP with pulses and blocks", state)
	}
	if state.Playing || state.Ended {
		t.Errorf("a tape that was never started reports playing=%v ended=%v", state.Playing, state.Ended)
	}
	if state.Current != 0 {
		t.Errorf("current block = %d at the start, want the first", state.Current)
	}
	if !state.Blocks[0].Current {
		t.Error("the first block is not marked current at the start")
	}

	// The offsets are non-decreasing and start at zero, which is what makes "the
	// block the head is in" answerable at all.
	if state.Blocks[0].StartsAt != 0 {
		t.Errorf("the first block starts at pulse %d, want 0", state.Blocks[0].StartsAt)
	}
	for i := 1; i < len(state.Blocks); i++ {
		if state.Blocks[i].StartsAt < state.Blocks[i-1].StartsAt {
			t.Errorf("block %d starts at %d, before block %d at %d",
				i, state.Blocks[i].StartsAt, i-1, state.Blocks[i-1].StartsAt)
		}
	}

	// The header is parsed rather than described, and against the real file rather
	// than a shape: batty.tap is one BASIC program followed by four blocks of code,
	// each with its own 19-byte header. A parse that reads the type or the name from
	// the wrong offset still produces *something* - a name with the type byte stuck on
	// the front of it, or a type taken from a parameter's high byte - so the check is
	// the names and types Fuse would show for this tape (ref/fuse-1.9.0/tape.c).
	want := []struct{ symbol, name string }{
		{"P", "BATTY"}, {"d", ""},
		{"B", "bat1"}, {"d", ""},
		{"B", "bat2"}, {"d", ""},
		{"B", "bat3"}, {"d", ""},
		{"B", "bat4"}, {"d", ""},
	}
	if len(state.Blocks) != len(want) {
		t.Fatalf("read %d blocks, want %d", len(state.Blocks), len(want))
	}
	for i, w := range want {
		got := state.Blocks[i]
		if got.Symbol != w.symbol || got.Name != w.name {
			t.Errorf("block %d = %s %q, want %s %q",
				i+1, got.Symbol, got.Name, w.symbol, w.name)
		}
	}

	// Move the head into the second block and ask again.
	pb.Pos = state.Blocks[1].StartsAt
	state = pb.StateSnapshot()
	if state.Current != 1 {
		t.Errorf("current block = %d at pulse %d, want 1", state.Current, pb.Pos)
	}
}

// A tape that is not there is not a crash: a window asks for its state every frame.
func TestTapeStateWithoutATape(t *testing.T) {
	var pb *Playback
	state := pb.StateSnapshot()
	if state.FileName != "" || state.Current != -1 || len(state.Blocks) != 0 {
		t.Errorf("state = %+v, want an empty one with no current block", state)
	}

	empty := NewPlayback(&Tape{})
	state = empty.StateSnapshot()
	if state.Current != -1 {
		t.Errorf("current block = %d for an empty tape, want -1", state.Current)
	}
}

// Percent is a percentage, 0 to 100, in the units Playback.PercentComplete reports.
//
// It was a fraction in the comment and a percentage in the value, and the tape window
// printed "10000.0%" at the end of a tape: two places deciding the units and
// disagreeing. This is the one that pins them.
func TestTapeStatePercentUnits(t *testing.T) {
	tape, err := LoadTapeFile("../../testdata/batty.tap")
	if err != nil {
		t.Skipf("testdata tape not available: %v", err)
	}
	pb := NewPlayback(tape)

	if got := pb.StateSnapshot().Percent; got != 0 {
		t.Errorf("percent at the start = %v, want 0", got)
	}

	pb.Pos = len(tape.Pulses) / 2
	if got := pb.StateSnapshot().Percent; got < 49 || got > 51 {
		t.Errorf("percent halfway = %v, want about 50", got)
	}

	pb.Pos = len(tape.Pulses)
	if got := pb.StateSnapshot().Percent; got < 99.9 || got > 100 {
		t.Errorf("percent at the end = %v, want 100 and not 10000", got)
	}
}
