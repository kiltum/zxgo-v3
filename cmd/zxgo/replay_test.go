package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kiltum/zxgo-v3/internal/emulator"
	"github.com/kiltum/zxgo-v3/pkg/media"
	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/replay"
	"github.com/kiltum/zxgo-v3/pkg/sound"
)

// TestApplyRecordedSettingsTurnsOnEveryField exists because five of the six
// settings were wired and fast tape was not: it was recorded, printed back as
// "Fast tape: playback runs unthrottled", and never applied, so a replay of a
// fast load ran throttled while claiming otherwise. Applying settings is a
// place where the compiler cannot help -- a field that nobody reads looks
// exactly like a field that works -- so this checks each one has an effect
// rather than trusting the shape of the code.
func TestApplyRecordedSettingsTurnsOnEveryField(t *testing.T) {
	// Every switch off by default on a 48K, so a field that is wired shows up as
	// a change and a field that is forgotten shows up as silence.
	cfg := model.Spectrum48K
	if cfg.HasTurboSound || cfg.HasTurboSoundFM || cfg.HasGS ||
		cfg.HasSnowEffect || cfg.NoFDCTiming {
		t.Fatalf("test precondition: 48K should start with every switch off")
	}

	all := replay.Settings{
		TurboSound:   true,
		TurboSoundFM: true,
		GeneralSound: true,
		Snow:         true,
		NoFDCTiming:  true,
		FastTape:     true,
	}
	fastTape := applyRecordedSettings(&cfg, all)

	effects := []struct {
		name string
		got  bool
	}{
		{"TurboSound", cfg.HasTurboSound},
		{"TurboSoundFM", cfg.HasTurboSoundFM},
		{"GeneralSound", cfg.HasGS},
		{"Snow", cfg.HasSnowEffect},
		{"NoFDCTiming", cfg.NoFDCTiming},
		{"FastTape", fastTape},
	}
	for _, e := range effects {
		if !e.got {
			t.Errorf("%s was recorded but not applied", e.name)
		}
	}
}

// Applying a replay must never turn off a switch the model itself enables: the
// Pentagon has TurboSound whatever any file says, and a recording made on it
// does not carry a "turn it on" for a machine that already had it.
func TestApplyRecordedSettingsKeepsModelDefaults(t *testing.T) {
	cfg := model.Pentagon128
	if !cfg.HasTurboSound {
		t.Fatalf("test precondition: the Pentagon config should have TurboSound on")
	}

	fastTape := applyRecordedSettings(&cfg, replay.Settings{})

	if !cfg.HasTurboSound {
		t.Error("an empty settings section turned off the model's own TurboSound")
	}
	if fastTape {
		t.Error("fast tape was reported on when nothing asked for it")
	}
}

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
	media := replay.Media{Tape: "game.tzx"}
	if err := saveReplay(emu, rec, path, "pentagon", media, true); err != nil {
		t.Fatalf("saveReplay: %v", err)
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
	if loaded.Media != media {
		t.Errorf("media = %+v, want %+v", loaded.Media, media)
	}
	if len(loaded.Events) != 2 {
		t.Fatalf("recorded %d events, want 2 (press and release)", len(loaded.Events))
	}
	if !loaded.Events[0].Key.Down || loaded.Events[1].Key.Down {
		t.Errorf("events = %+v, want a press then a release", loaded.Events)
	}
}

// resolveMedia takes a recorded path as it stands, then beside the replay file,
// which is where a session someone sent you usually keeps its tape.
func TestResolveMedia(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "game.tzx"), []byte("tape"), 0o644); err != nil {
		t.Fatal(err)
	}
	replayPath := filepath.Join(dir, "session.replay")

	// Beside the replay file: found.
	var missing []string
	if got := resolveMedia("game.tzx", replayPath, &missing); got != filepath.Join(dir, "game.tzx") {
		t.Errorf("got %q, want the copy beside the replay", got)
	}
	if len(missing) != 0 {
		t.Errorf("reported missing: %v", missing)
	}

	// Nowhere: reported, and the caller carries on with no media rather than
	// refusing to play the keys.
	if got := resolveMedia("absent.tzx", replayPath, &missing); got != "" {
		t.Errorf("got %q, want empty for a missing path", got)
	}
	if len(missing) != 1 || missing[0] != "absent.tzx" {
		t.Errorf("missing = %v, want [absent.tzx]", missing)
	}

	// Nothing recorded is not a missing file.
	if got := resolveMedia("", replayPath, &missing); got != "" {
		t.Errorf("got %q, want empty for an unrecorded medium", got)
	}
	if len(missing) != 1 {
		t.Errorf("missing = %v, want the earlier entry only", missing)
	}

	// An absolute path that exists is used as given, not looked up beside.
	abs := filepath.Join(dir, "game.tzx")
	if got := resolveMedia(abs, filepath.Join(t.TempDir(), "other.replay"), &missing); got != abs {
		t.Errorf("got %q, want the absolute path %q", got, abs)
	}
}

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

	playing, ok := toggleTapePlayback(emu)
	if !ok {
		t.Fatal("toggleTapePlayback reported no tape")
	}
	if !playing || !emu.TapePlayback().IsPlaying() {
		t.Error("the tape did not start playing")
	}
	emu.RunFrame()

	playing, ok = toggleTapePlayback(emu)
	if !ok {
		t.Fatal("toggleTapePlayback reported no tape")
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

	if _, ok := toggleTapePlayback(emu); ok {
		t.Error("toggleTapePlayback reported a tape with none mounted")
	}
	if n := len(rec.Events()); n != 0 {
		t.Errorf("recorded %d events with no tape mounted, want 0", n)
	}
}

// A replay that names no media still plays: the missing-file warning must not
// turn into a failure to start.
func TestLoadReplayRejectsForeignFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notareplay.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadReplay(path); err == nil {
		t.Fatal("loadReplay accepted a file that is not a replay")
	}
	if _, err := loadReplay(filepath.Join(t.TempDir(), "nope.replay")); err == nil {
		t.Fatal("loadReplay accepted a missing file")
	}
}
