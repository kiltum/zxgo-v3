package frontend

import (
	"testing"

	"github.com/kiltum/zxgo-v3/internal/emulator"
	"github.com/kiltum/zxgo-v3/pkg/media"
	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/replay"
	"github.com/kiltum/zxgo-v3/pkg/sound"
)

// TestToggleTapePlaybackIsRecorded guards the path Cmd+P actually takes.
//
// The transport used to be driven straight on the Playback object, which the
// recorder cannot see: a session could type LOAD "", press Cmd+P, watch the game
// load, save -- and produce a file with no tape event, so the replay sat at a
// black screen waiting for a tape that never started. The bug survived a test
// that called StartTapePlayback directly, because that is not what the keyboard
// does.
func TestToggleTapePlaybackIsRecorded(t *testing.T) {
	tapePath := "../../testdata/batty.tap"
	tape, err := media.LoadTapeFile(tapePath)
	if err != nil {
		t.Skipf("testdata %s not available: %v", tapePath, err)
	}

	emu := emulator.New(model.Spectrum48K, &sound.NullOutput{})
	if err := emu.LoadTape(tape); err != nil {
		t.Fatalf("LoadTape: %v", err)
	}
	rec := replay.NewRecorder()
	emu.SetRecorder(rec)
	emu.RunFrame()

	playing, ok := ToggleTapePlayback(NewMachine(emu))
	if !ok {
		t.Fatal("ToggleTapePlayback reported no tape")
	}
	if !playing || !emu.TapePlayback().IsPlaying() {
		t.Error("the tape did not start playing")
	}
	emu.RunFrame()

	playing, ok = ToggleTapePlayback(NewMachine(emu))
	if !ok {
		t.Fatal("ToggleTapePlayback reported no tape")
	}
	if playing || emu.TapePlayback().IsPlaying() {
		t.Error("the tape did not pause")
	}

	events := rec.Events()
	if len(events) != 2 {
		t.Fatalf("recorded %d events, want 2 -- Cmd+P is not reaching the recorder: %+v",
			len(events), events)
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

// An empty tape slot is not an error: the command does nothing and nothing is
// recorded, rather than a tape event a replay could not act on.
func TestToggleTapePlaybackWithoutTape(t *testing.T) {
	emu := emulator.New(model.Spectrum48K, &sound.NullOutput{})
	rec := replay.NewRecorder()
	emu.SetRecorder(rec)

	if _, ok := ToggleTapePlayback(NewMachine(emu)); ok {
		t.Error("ToggleTapePlayback reported a tape with none mounted")
	}
	if n := len(rec.Events()); n != 0 {
		t.Errorf("recorded %d events with no tape mounted, want 0", n)
	}
}
