package frontend

import "testing"

// fakeBackend is a WindowBackend with no windows: it hands the loop a script of
// events and records what it was asked to draw, which is what the loop's own
// behaviour is about.
type fakeBackend struct {
	displays []Rect
	// batches is one poll's worth of events per entry; a poll past the end reports
	// a quit.
	batches [][]Event

	polls       int
	dialogs     []DialogRequest
	saveDialogs []DialogRequest
	raised      []ToolID
	synced      []WindowsView
	mains       []MainView
	tools       []ToolID
	destroyed   bool
}

func (f *fakeBackend) Init() error { return nil }

func (f *fakeBackend) Poll(events *[]Event) bool {
	f.polls++
	if f.polls > len(f.batches) {
		return false
	}
	*events = append(*events, f.batches[f.polls-1]...)
	return true
}

func (f *fakeBackend) MainWindowSize() (int, int) { return 768, 576 }

func (f *fakeBackend) Displays() []Rect { return f.displays }

func (f *fakeBackend) PresentMain(main MainView) { f.mains = append(f.mains, main) }

func (f *fakeBackend) SyncWindows(want WindowsView) { f.synced = append(f.synced, want) }

func (f *fakeBackend) PresentTool(id ToolID, _ Views) { f.tools = append(f.tools, id) }

func (f *fakeBackend) OpenFileDialog(req DialogRequest) { f.dialogs = append(f.dialogs, req) }

func (f *fakeBackend) SaveFileDialog(req DialogRequest) { f.saveDialogs = append(f.saveDialogs, req) }

func (f *fakeBackend) RaiseWindow(id ToolID) { f.raised = append(f.raised, id) }

func (f *fakeBackend) Destroy() { f.destroyed = true }

var _ WindowBackend = (*fakeBackend)(nil)

// The loop drives the sections of 5.4 in order: poll, apply, tick, make the windows
// match, and draw. A tool opened by a key has to have its window before anything is
// drawn in it.
func TestBackendLoopDrivesTheBackend(t *testing.T) {
	app, m := testApp()
	backend := &fakeBackend{
		displays: []Rect{{W: 1920, H: 1080}},
		batches: [][]Event{
			{{Kind: EvAction, Action: ActToggleTape}}, // open the tape window
			{{Kind: EvAction, Action: ActToggleDebugger}},
		},
	}

	loop := &BackendLoop{App: app, Backend: backend}
	loop.Run()

	if backend.polls != 3 {
		t.Errorf("polled %d times, want 3 (two batches and the quit)", backend.polls)
	}
	if m.slices == 0 {
		t.Error("the machine never ran")
	}
	if !openIs(app, ToolTape) || !openIs(app, ToolDebugger) {
		t.Error("the actions the backend reported did not reach the app")
	}

	// The tape tool was open by the time it was drawn, and drawn after the windows
	// were made to match.
	if len(backend.tools) == 0 || backend.tools[0] != ToolTape {
		t.Errorf("tools drawn = %v, want the tape window first", backend.tools)
	}
	if len(backend.synced) == 0 {
		t.Fatal("SyncWindows was never called")
	}
	if !openIn(backend.synced[1], ToolTape) {
		t.Error("the tape window was not in the wanted set on the tick it opened")
	}
}

// The displays come from the backend and are handed to the app before anything is
// placed, which is what a restored rect is clamped against (D4).
func TestBackendLoopFeedsTheDisplays(t *testing.T) {
	app, _ := testApp()
	backend := &fakeBackend{displays: []Rect{{W: 1024, H: 768}}}
	layout := Layout{
		Main:  WindowState{ID: ToolMain, Rect: Rect{X: 5000, Y: 5000, W: 800, H: 600}},
		Tools: nil,
	}
	app.SetLayout(layout)

	(&BackendLoop{App: app, Backend: backend}).Run()

	if got := app.MainWindow().Rect; got.X+got.W > 1024 || got.Y+got.H > 768 {
		t.Errorf("the main window is at %+v, outside the only display", got)
	}
}

// The layout is handed over when it changes and not every iteration: D3 says
// "written whenever it changes", and a file rewritten sixty times a second with the
// same bytes is a file that wears the disk out for nothing.
func TestBackendLoopPersistsTheLayoutOnChange(t *testing.T) {
	app, _ := testApp()
	backend := &fakeBackend{
		batches: [][]Event{
			nil, // nothing happens
			{{Kind: EvAction, Action: ActToggleDisks}}, // now something does
			nil,
			nil,
		},
	}

	var saves []Layout
	loop := &BackendLoop{
		App:           app,
		Backend:       backend,
		LayoutChanged: func(l Layout) { saves = append(saves, l) },
	}
	loop.Run()

	if len(saves) != 1 {
		t.Fatalf("saved the layout %d times, want once (the change)", len(saves))
	}
	if !openIn(WindowsView{Tools: saves[0].Tools}, ToolDisks) {
		t.Error("the saved layout does not have the window that changed it")
	}
}

// A quit from the backend stops the loop before it draws anything else, so a
// closing window does not get a frame on the way out.
func TestBackendLoopStopsOnQuit(t *testing.T) {
	app, _ := testApp()
	backend := &fakeBackend{batches: [][]Event{{{Kind: EvQuit}}}}

	(&BackendLoop{App: app, Backend: backend}).Run()

	if backend.polls != 1 {
		t.Errorf("polled %d times, want 1", backend.polls)
	}
	if len(backend.mains) != 0 {
		t.Errorf("drew %d frames after a quit, want none", len(backend.mains))
	}
}

// openIn reports whether a tool is open in a wanted set.
func openIn(want WindowsView, id ToolID) bool {
	for _, w := range want.Tools {
		if w.ID == id {
			return w.Open
		}
	}
	return false
}

// The loop carries a raise request to the backend, and only the ones that were asked
// for: a raise per iteration is the behaviour that made the emulator impossible to put
// behind another program.
func TestLoopHandsOverRaiseRequests(t *testing.T) {
	app, _ := testApp()
	backend := &fakeBackend{
		batches: [][]Event{
			{{Kind: EvAction, Action: ActToggleTape}}, // opens the tape window
			nil,
			nil,
		},
	}

	(&BackendLoop{App: app, Backend: backend}).Run()

	if len(backend.raised) != 1 {
		t.Fatalf("raised %d windows over four iterations, want 1 (the open)", len(backend.raised))
	}
	if backend.raised[0] != ToolTape {
		t.Errorf("raised %v, want the tape window", backend.raised[0])
	}
}
