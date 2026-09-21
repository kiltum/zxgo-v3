package frontend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kiltum/zxgo-v3/internal/emulator"
	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/replay"
	"github.com/kiltum/zxgo-v3/pkg/sound"
)

// TestSaveReplayRoundTrip is the write half of the CLI's contract: what is saved
// must load back with the same machine, the same switches and the same events,
// and the switches must come from the configured machine rather than from the
// flags -- a Pentagon's always-on TurboSound is a property of the model, not of
// a flag someone typed.
func TestSaveReplayRoundTrip(t *testing.T) {
	cfg := model.Pentagon128
	if !cfg.HasTurboSound {
		t.Fatalf("test precondition: the Pentagon config should have TurboSound on")
	}

	emu := emulator.New(cfg, &sound.NullOutput{})
	rec := replay.NewRecorder()
	emu.SetRecorder(rec)
	emu.PressKey(6, 0)
	emu.RunFrame()
	emu.ReleaseKey(6, 0)

	path := filepath.Join(t.TempDir(), "session.replay")
	recorded := MediaPaths{Tape: "game.tzx"}
	summary, err := SaveReplay(NewMachine(emu), rec, path, recorded, true)
	if err != nil {
		t.Fatalf("SaveReplay: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	loaded, err := replay.Load(f)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Model != "pentagon" || loaded.ModelName != cfg.Name {
		t.Errorf("machine = %q/%q, want pentagon/%q", loaded.Model, loaded.ModelName, cfg.Name)
	}
	if loaded.CPUHz != emu.CPUHz() {
		t.Errorf("cpu_hz = %d, want %d", loaded.CPUHz, emu.CPUHz())
	}
	if !loaded.Settings.TurboSound {
		t.Error("TurboSound was on for the machine but not recorded")
	}
	if !loaded.Settings.FastTape {
		t.Error("fast tape was passed but not recorded")
	}
	if loaded.Media.Tape != recorded.Tape {
		t.Errorf("media = %+v, want the tape %q", loaded.Media, recorded.Tape)
	}
	if len(loaded.Events) != 2 {
		t.Fatalf("recorded %d events, want 2 (press and release)", len(loaded.Events))
	}
	if !loaded.Events[0].Key.Down || loaded.Events[1].Key.Down {
		t.Errorf("events = %+v, want a press then a release", loaded.Events)
	}

	// The summary the caller prints must describe the file that was written:
	// two key events, no tape events, and a duration taken from the emulated
	// frame rather than from the host clock.
	if summary.Path != path {
		t.Errorf("summary.Path = %q, want %q", summary.Path, path)
	}
	if summary.KeyEvents != 2 || summary.TapeEvents != 0 {
		t.Errorf("summary = %d keys / %d tape events, want 2 / 0",
			summary.KeyEvents, summary.TapeEvents)
	}
	if summary.Duration <= 0 {
		t.Errorf("summary.Duration = %v, want the one frame that was run", summary.Duration)
	}
}
