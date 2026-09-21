package frontend

import (
	"path/filepath"
	"testing"

	"github.com/kiltum/zxgo-v3/internal/emulator"
	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/replay"
	"github.com/kiltum/zxgo-v3/pkg/sound"
)

// TestHandleKeyReachesTheRecorder is the gate on UI_DESIGN.md D9 for the keymap
// path: a host key that a binding does not claim goes to the matrix through the
// emulator's own methods, because the recorder hangs off exactly those. A front end
// that reached the ULA directly would pass every other test on this and produce
// replay files with no input in them.
//
// It uses the real emulator rather than the fake, because the assertion is about a
// recorder, and a fake would only be asserting that the fake was called.
func TestHandleKeyReachesTheRecorder(t *testing.T) {
	emu := emulator.New(model.Spectrum48K, &sound.NullOutput{})
	rec := replay.NewRecorder()
	emu.SetRecorder(rec)

	app := New(NewMachine(emu))
	app.Notice = func(string) {}
	app.Warn = func(string) {}

	app.HandleKey(ToolMain, KeyA, 0, true)
	emu.RunFrame()
	app.HandleKey(ToolMain, KeyA, 0, false)

	events := rec.Events()
	if len(events) != 2 {
		t.Fatalf("recorded %d events, want a press and a release: %+v", len(events), events)
	}
	if events[0].Key == nil || !events[0].Key.Down {
		t.Errorf("first event = %+v, want a press", events[0])
	}
	if events[1].Key == nil || events[1].Key.Down {
		t.Errorf("second event = %+v, want a release", events[1])
	}
	// 'A' is the ZX keyboard's row 1 column 0, and the event has to carry the matrix
	// cell rather than the host key: a replay is played back on a machine that has
	// never heard of a PC keyboard.
	if events[0].Key.Row != 1 || events[0].Key.Col != 0 {
		t.Errorf("recorded (%d,%d), want (1,0) for the A key", events[0].Key.Row, events[0].Key.Col)
	}
}

// The whole path a real key takes: a backend reports the edge, the loop applies it,
// the front end's keymap and matrix decide it is the machine's, and the machine's own
// methods put it in the recording.
func TestBackendEventReachesTheRecorder(t *testing.T) {
	emu := emulator.New(model.Spectrum48K, &sound.NullOutput{})
	rec := replay.NewRecorder()
	emu.SetRecorder(rec)

	app := New(NewMachine(emu))
	app.Notice = func(string) {}
	app.Warn = func(string) {}

	backend := &fakeBackend{batches: [][]Event{
		{{Kind: EvKey, Key: KeyEnter, Down: true, Tool: ToolMain}},
		{{Kind: EvKey, Key: KeyEnter, Down: false, Tool: ToolMain}},
	}}
	(&BackendLoop{App: app, Backend: backend}).Run()

	events := rec.Events()
	if len(events) != 2 {
		t.Fatalf("recorded %d events, want a press and a release: %+v", len(events), events)
	}
	// ENTER is row 6 column 0.
	if events[0].Key == nil || events[0].Key.Row != 6 || events[0].Key.Col != 0 {
		t.Errorf("first event = %+v, want a press of (6,0)", events[0])
	}
	if events[1].Key == nil || events[1].Key.Down {
		t.Errorf("second event = %+v, want a release", events[1])
	}
}

// The screenshot action writes exactly one PNG, into the directory the front end was
// given rather than wherever the process happens to be.
func TestScreenshotActionWritesAFile(t *testing.T) {
	app, _ := testApp()
	dir := t.TempDir()
	app.ScreenshotDir = dir
	var notices []string
	app.Notice = func(msg string) { notices = append(notices, msg) }

	if err := app.Dispatch(ActScreenshot); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	names, err := filepath.Glob(filepath.Join(dir, "*.png"))
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 {
		t.Fatalf("wrote %d screenshots (%v), want 1", len(names), names)
	}
	if len(notices) != 1 {
		t.Errorf("notices = %v, want the path it wrote", notices)
	}
}

// A screenshot that cannot be written is reported rather than fatal: the machine
// carries on, and the user is told why nothing appeared.
func TestScreenshotFailureIsReported(t *testing.T) {
	app, _ := testApp()
	app.ScreenshotDir = filepath.Join(t.TempDir(), "no", "such", "place")
	var warnings []string
	app.Warn = func(msg string) { warnings = append(warnings, msg) }

	// The action itself does not return the error - a dispatched action has nowhere
	// to return it to - so it is reported through Warn.
	app.Apply([]Event{{Kind: EvAction, Action: ActScreenshot}})

	if len(warnings) != 1 {
		t.Errorf("warnings = %v, want one", warnings)
	}
}
