package frontend

import (
	"testing"
	"time"
)

// A first launch: every tool closed, the main window placed, the menubar shown
// (a user who has never seen the menus cannot know ESC brings them back), and the
// on-screen keyboard created not-focusable (D4).
func TestDefaultWindows(t *testing.T) {
	app, _ := testApp()

	if !app.MainWindow().Rect.Placed() {
		t.Error("the main window has no default geometry")
	}
	if !app.MenuVisible() {
		t.Error("the menubar is hidden on a first launch")
	}
	if open := app.OpenTools(); len(open) != 0 {
		t.Errorf("tools are open on a first launch: %v", open)
	}

	windows := app.Windows()
	if len(windows) != len(Tools()) {
		t.Fatalf("%d windows for %d tools", len(windows), len(Tools()))
	}
	for _, w := range windows {
		if w.ID == ToolKeyboard && !w.Flags.Has(NotFocusable) {
			t.Error("the on-screen keyboard would take keyboard focus from the machine")
		}
		if w.ID != ToolKeyboard && w.Flags.Has(NotFocusable) {
			t.Errorf("%v is not-focusable, but only the on-screen keyboard is", w.ID)
		}
		if !w.Rect.Placed() {
			t.Errorf("%v has no default geometry", w.ID)
		}
	}
}

// The menubar item opens and raises rather than toggling, because a second click
// on "Tape" should not close the window the user is looking at (D4). The
// shortcut toggles.
func TestOpenRaisesAndToggleCloses(t *testing.T) {
	app, _ := testApp()

	app.OpenTool(ToolTape)
	if !openIs(app, ToolTape) {
		t.Fatal("the tool did not open")
	}
	app.OpenTool(ToolTape)
	if !openIs(app, ToolTape) {
		t.Error("opening an open tool closed it")
	}
	if app.Focused() != ToolTape {
		t.Errorf("focus = %v, want the tool that was opened", app.Focused())
	}

	app.ToggleTool(ToolTape)
	if openIs(app, ToolTape) {
		t.Error("the shortcut toggle did not close the tool")
	}
	app.ToggleTool(ToolTape)
	if !openIs(app, ToolTape) {
		t.Error("the shortcut toggle did not reopen the tool")
	}
}

// The main window has no open flag: closing it means leaving, which is the quit
// action rather than a window operation.
func TestTheMainWindowCannotBeClosed(t *testing.T) {
	app, _ := testApp()

	app.CloseTool(ToolMain)
	app.ToggleTool(ToolMain)

	if !app.MainWindow().Rect.Placed() {
		t.Error("the main window lost its geometry to a close")
	}
	if app.Focused() != ToolMain {
		t.Errorf("focus = %v, want the main window", app.Focused())
	}
}

// Closing a tool that had focus hands focus back to the main window, so the
// machine hears the keyboard again rather than nothing hearing it.
func TestClosingTheFocusedToolReturnsFocusToMain(t *testing.T) {
	app, _ := testApp()
	app.OpenTool(ToolDebugger)

	app.CloseTool(ToolDebugger)

	if app.Focused() != ToolMain {
		t.Errorf("focus = %v, want the main window", app.Focused())
	}
}

// The clamp is what stops a window saved on a monitor that is no longer attached
// from opening offscreen (D4), and it has to shrink as well as move: a rect saved
// on a larger display is too big to fit, and moving it would leave most of it
// outside.
func TestClampRect(t *testing.T) {
	laptop := Rect{W: 1440, H: 900, X: 0, Y: 0}
	second := Rect{W: 1920, H: 1080, X: 1440, Y: 0}
	both := []Rect{laptop, second}

	for _, tc := range []struct {
		name     string
		r        Rect
		displays []Rect
		want     Rect
	}{
		{"already inside", Rect{X: 100, Y: 100, W: 400, H: 300}, both, Rect{X: 100, Y: 100, W: 400, H: 300}},
		// A rect that straddles two displays goes to the one it overlaps most
		// rather than staying half off both, which is the same rule that puts a
		// window back on the laptop when its monitor is unplugged.
		{"straddling two displays", Rect{X: 1300, Y: 800, W: 400, H: 300}, both, Rect{X: 1440, Y: 780, W: 400, H: 300}},
		{"before the origin", Rect{X: -200, Y: -100, W: 400, H: 300}, both, Rect{X: 0, Y: 0, W: 400, H: 300}},
		{"bigger than the display", Rect{X: 100, Y: 100, W: 3000, H: 2000}, []Rect{laptop}, Rect{X: 0, Y: 0, W: 1440, H: 900}},
		{"kept on its own display", Rect{X: 1500, Y: 100, W: 400, H: 300}, both, Rect{X: 1500, Y: 100, W: 400, H: 300}},
		{"pulled back from a display that is gone", Rect{X: 1500, Y: 100, W: 400, H: 300}, []Rect{laptop}, Rect{X: 1040, Y: 100, W: 400, H: 300}},
		{"never placed", Rect{}, both, Rect{}},
		{"no displays", Rect{X: 5000, Y: 5000, W: 400, H: 300}, nil, Rect{X: 5000, Y: 5000, W: 400, H: 300}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClampRect(tc.r, tc.displays); got != tc.want {
				t.Errorf("ClampRect(%+v) = %+v, want %+v", tc.r, got, tc.want)
			}
		})
	}
}

// A layout read from disk is clamped on the way in, and anything the file does
// not mention keeps its default: a layout file from an older version cannot empty
// the window set or move a window offscreen.
func TestSetLayoutClampsAndKeepsDefaults(t *testing.T) {
	app, _ := testApp()
	app.SetDisplays([]Rect{{W: 1440, H: 900}})

	app.SetLayout(Layout{
		Main: WindowState{ID: ToolMain, Rect: Rect{X: 9000, Y: 9000, W: 800, H: 600}},
		Tools: []WindowState{
			{ID: ToolDisks, Open: true, Rect: Rect{X: 9000, Y: 9000, W: 480, H: 440}},
			{ID: ToolMain, Open: true}, // not a tool: ignored
		},
		MenuVisible: false,
	})

	if got := app.MainWindow().Rect; got.X+got.W > 1440 || got.Y+got.H > 900 {
		t.Errorf("the main window came back offscreen at %+v", got)
	}
	if !openIs(app, ToolDisks) {
		t.Error("the layout's open tool is not open")
	}
	if app.MenuVisible() {
		t.Error("the layout said the menubar was hidden and it came back shown")
	}

	// A tool the layout did not mention keeps its default geometry and stays
	// closed, and the tool that only said "open" keeps its default size.
	if openIs(app, ToolTape) {
		t.Error("a tool the layout did not mention came back open")
	}
	for _, w := range app.Windows() {
		if !w.Rect.Placed() {
			t.Errorf("%v lost its default geometry to a partial layout", w.ID)
		}
	}
}

// A tool's flags are a property of the tool, not a preference: a layout file that
// says the on-screen keyboard should take keyboard focus is a file to ignore,
// because that would break the one thing the window exists to avoid.
func TestSetLayoutIgnoresFlagChanges(t *testing.T) {
	app, _ := testApp()

	app.SetLayout(Layout{
		Tools: []WindowState{{ID: ToolKeyboard, Open: true, Flags: AlwaysOnTop}},
	})

	for _, w := range app.Windows() {
		if w.ID != ToolKeyboard {
			continue
		}
		if !w.Flags.Has(NotFocusable) {
			t.Error("the layout file took NotFocusable away from the keyboard window")
		}
	}
}

// A monitor that is unplugged while the emulator runs takes its windows with it,
// clamped back onto the displays that are left.
func TestSetDisplaysClampsWhatIsAlreadyPlaced(t *testing.T) {
	app, _ := testApp()
	app.SetDisplays([]Rect{{W: 1440, H: 900}, {X: 1440, W: 1920, H: 1080}})
	app.SetRect(ToolTape, Rect{X: 2000, Y: 100, W: 440, H: 320})

	// The second monitor is unplugged.
	app.SetDisplays([]Rect{{W: 1440, H: 900}})

	for _, w := range app.Windows() {
		if w.ID != ToolTape {
			continue
		}
		if w.Rect.X+w.Rect.W > 1440 {
			t.Errorf("the tape window stayed offscreen at %+v", w.Rect)
		}
		if w.Rect.W != 440 || w.Rect.H != 320 {
			t.Errorf("the tape window was resized to %+v", w.Rect)
		}
	}
}

// openIs reports whether a tool is open.
func openIs(app *App, id ToolID) bool {
	for _, w := range app.Windows() {
		if w.ID == id {
			return w.Open
		}
	}
	return false
}

// Bringing a window to the front is a request, not state a backend re-reads: the
// backend used to raise the focused window on every frame, and because raising a
// window also activates its application, the emulator could never be sent behind
// anything else. This pins the request side of that.
func TestOpeningAToolAsksForARaise(t *testing.T) {
	app, _ := testApp()

	// Opening a tool asks for it to come forward (D4). The first time is the window
	// appearing; the second is the "already open" case the rule is about.
	for i := 0; i < 2; i++ {
		app.OpenTool(ToolDebugger)
		id, ok := app.TakeRaiseRequest()
		if !ok {
			t.Fatalf("open %d: opening a tool did not ask for a raise", i+1)
		}
		if id != ToolDebugger {
			t.Errorf("open %d: asked to raise %v, want the debugger", i+1, id)
		}
		if _, again := app.TakeRaiseRequest(); again {
			t.Errorf("open %d: the same raise was asked for twice", i+1)
		}
	}

	// Drawing a frame asks for nothing: that is the bug this test exists for.
	app.Tick(time.Now())
	app.WindowsView()
	if _, ok := app.TakeRaiseRequest(); ok {
		t.Error("something unrelated to opening a tool asked for a raise")
	}

	// A real focus change - the user clicking a window - is not a request either: the
	// window manager has already done it.
	app.Apply([]Event{{Kind: EvFocusChange, Tool: ToolTape}})
	if _, ok := app.TakeRaiseRequest(); ok {
		t.Error("a focus report asked for a raise")
	}
	// The focused window is still recorded, for the routing rules that need it.
	if app.Focused() != ToolTape {
		t.Errorf("focus = %v, want the tape window", app.Focused())
	}
}
