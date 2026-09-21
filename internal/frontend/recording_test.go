package frontend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kiltum/zxgo-v3/internal/emulator"
	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/replay"
	"github.com/kiltum/zxgo-v3/pkg/sound"
)

// Recording from the front end records what the machine's input methods see, and the file
// it writes is one the machine can be driven from again. It uses a real emulator because
// the claim is about a replay file, and a fake would only be asserting that the fake was
// called.
func TestRecordingRoundTrip(t *testing.T) {
	emu := emulator.New(model.Spectrum48K, &sound.NullOutput{})
	app := New(NewMachine(emu))
	app.Notice = func(string) {}
	app.Warn = func(string) {}

	if app.Recording() {
		t.Fatal("a fresh app is recording")
	}

	// The user asks for a recording and names the file.
	if err := app.Dispatch(ActRecordReplay); err != nil {
		t.Fatalf("Dispatch(ActRecordReplay): %v", err)
	}
	req, ok := app.TakeDialogRequest()
	if !ok || req.Kind != DialogSaveReplay {
		t.Fatalf("the record action asked for %v", req.Kind)
	}

	dir := t.TempDir()
	app.Apply([]Event{{Kind: EvDialogResult, Tool: ToolMain,
		Path: filepath.Join(dir, "session"), Dialog: DialogSaveReplay}})
	if !app.Recording() {
		t.Fatal("the app is not recording after the file was named")
	}
	// The name got the extension a save would give it.
	want := filepath.Join(dir, "session"+ReplayExt)
	if got := app.RecordingPath(); got != want {
		t.Errorf("recording to %q, want %q", got, want)
	}

	// Input: a host key and an on-screen one, which is what a replay has to carry.
	app.HandleKey(ToolMain, KeyA, 0, true)
	emu.RunFrame()
	app.HandleKey(ToolMain, KeyA, 0, false)
	app.Apply([]Event{{Kind: EvCell, Tool: ToolKeyboard, Cell: Cell{Row: 7, Col: 0}, Down: true}})
	app.Apply([]Event{{Kind: EvCell, Tool: ToolKeyboard, Cell: Cell{Row: 7, Col: 0}, Down: false}})

	// The media the recording should name, as the machine was given them.
	app.SetMediaPaths(MediaPaths{Tape: "game.tzx", Disk: "game.trd"})

	msg, err := app.SaveRecording()
	if err != nil {
		t.Fatalf("SaveRecording: %v", err)
	}
	if !strings.Contains(msg, want) {
		t.Errorf("the report does not name the file: %q", msg)
	}

	f, err := os.Open(want)
	if err != nil {
		t.Fatalf("the recording was not written: %v", err)
	}
	defer f.Close()
	file, err := replay.Load(f)
	if err != nil {
		t.Fatalf("the recording is not a replay: %v", err)
	}
	if len(file.Events) != 4 {
		t.Errorf("recorded %d events, want four (two keys, two edges): %+v",
			len(file.Events), file.Events)
	}
	if file.Media.Tape != "game.tzx" || file.Media.Disk != "game.trd" {
		t.Errorf("the recording names %+v, want the media it was given", file.Media)
	}
	if file.Model != model.Spectrum48K.Key {
		t.Errorf("the recording names model %q, want %q", file.Model, model.Spectrum48K.Key)
	}
}

// A recording is written once, on the way out, and asking again after nothing was recorded
// writes nothing: the exit path calls this whatever happened, and a file with no events in
// it would be a lie about the session.
func TestSaveRecordingWithNothingRecorded(t *testing.T) {
	app, _ := testApp()

	msg, err := app.SaveRecording()
	if err != nil {
		t.Errorf("SaveRecording with no recording: %v", err)
	}
	if msg != "" {
		t.Errorf("SaveRecording reported %q with no recording", msg)
	}
}
