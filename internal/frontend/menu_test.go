package frontend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The menubar is data, so what it says can be checked without a window. Two things
// matter: every tool is in it (D4 - the menubar must not be the only way to a
// window, and a window must not be missing from the only list the user can read),
// and an entry's shortcut is the key the registry actually has.
func TestMenuListsEveryTool(t *testing.T) {
	app, _ := testApp()

	items := menuItems(app.Menu(), "Tools")
	if len(items) != 2*len(Tools())-1 {
		t.Fatalf("%d entries for %d tools (with separators), want %d",
			len(items), len(Tools()), 2*len(Tools())-1)
	}

	for _, id := range Tools() {
		found := false
		for _, item := range items {
			if item.Kind == MenuItemCheck && item.Action == toggleActionFor(id) {
				found = true
			}
		}
		if !found {
			t.Errorf("%v has no menubar entry", id)
		}
	}
}

// A tool's entry shows whether the tool is open, which is the one thing the menubar
// has to say about it rather than do.
func TestMenuChecksTheOpenTools(t *testing.T) {
	app, _ := testApp()
	app.OpenTool(ToolDisks)

	for _, item := range menuItems(app.Menu(), "Tools") {
		if item.Action != ActToggleDisks {
			continue
		}
		if !item.Checked {
			t.Error("the disk window is open and its entry is not checked")
		}
	}

	app.CloseTool(ToolDisks)
	for _, item := range menuItems(app.Menu(), "Tools") {
		if item.Action == ActToggleDisks && item.Checked {
			t.Error("the disk window is closed and its entry is still checked")
		}
	}
}

// The shortcut a menu advertises is the one the keymap holds, so the two cannot
// drift apart: a menu that says Cmd+P while the registry binds something else is
// worse than one that says nothing.
func TestMenuShortcutsComeFromTheKeymap(t *testing.T) {
	app, _ := testApp()

	for _, item := range menuItems(app.Menu(), "Machine") {
		if item.Action == ActNone || item.Shortcut == "" {
			continue
		}
		binding, err := ParseBinding(item.Shortcut)
		if err != nil {
			t.Errorf("%v advertises %q, which is not a binding", item.Action, item.Shortcut)
			continue
		}
		bound, ok := app.Keymap().Lookup(binding)
		if !ok || bound != item.Action {
			t.Errorf("the menu says %v is %q, but the keymap has %v there",
				item.Action, item.Shortcut, bound)
		}
	}
}

// An unbindable action is not advertised as a shortcut at all.
func TestMenuWithNoBindingShowsNoShortcut(t *testing.T) {
	app, _ := testApp()
	app.SetKeymap(Keymap{})

	for _, menu := range app.Menu() {
		for _, item := range menu.Items {
			if item.Shortcut != "" {
				t.Errorf("%v advertises %q with an empty keymap", item.Action, item.Shortcut)
			}
		}
	}
}

// The shortcut the menu picks has to be the same one every frame. A plain map
// iteration would choose a different binding of the same action each time, and the
// label would flicker between them.
func TestMenuShortcutIsStable(t *testing.T) {
	app, _ := testApp()
	app.SetKeymap(Merge(DefaultBindings(), Keymap{
		{Key: KeyF7}:                ActPauseToggle,
		{Key: KeyF4, Mods: ModCtrl}: ActPauseToggle,
	}))

	first := ""
	for i := 0; i < 20; i++ {
		got := ""
		for _, item := range menuItems(app.Menu(), "Machine") {
			if item.Action == ActPauseToggle {
				got = item.Shortcut
			}
		}
		if i == 0 {
			first = got
			continue
		}
		if got != first {
			t.Fatalf("the pause shortcut flickered: %q then %q", first, got)
		}
	}
	// Fewest modifiers wins, so the bare key is the one shown.
	if first != "f5" {
		t.Errorf("the pause shortcut is %q, want f5 (the binding with no modifiers)", first)
	}
}

// The status read-out is what the menubar draws on the right (D4), so it has to
// say what the machine is doing.
func TestStatusText(t *testing.T) {
	app, m := testApp()
	m.fastTape = true
	app.Pause()
	app.Tick(time.Now())

	text := app.StatusText()
	if want := "ZX Spectrum 48K"; !strings.Contains(text, want) {
		t.Errorf("status = %q, want it to name the machine (%q)", text, want)
	}
	if !strings.Contains(text, "paused") {
		t.Errorf("status = %q, want it to say the machine is paused", text)
	}
	if !strings.Contains(text, "fast tape") {
		t.Errorf("status = %q, want it to say a fast load is running", text)
	}
	if !strings.Contains(text, "fps") {
		t.Errorf("status = %q, want a frame rate", text)
	}
}

// The main view is what the backend draws, and it has to agree with the app: one
// place decides whether the menubar is shown, and the screen comes from the machine.
func TestMainViewMatchesTheApp(t *testing.T) {
	app, _ := testApp()
	app.ToggleMenu()

	view := app.MainView()

	if view.MenuVisible {
		t.Error("the main view says the menubar is shown after it was hidden")
	}
	if len(view.Menu) == 0 {
		t.Error("the main view has no menu")
	}
	if view.Screen.W != app.Screen().W || view.Screen.H != app.Screen().H {
		t.Errorf("the view's screen is %dx%d, the machine's is %dx%d",
			view.Screen.W, view.Screen.H, app.Screen().W, app.Screen().H)
	}
}

// ---------------------------------------------------------------- the store

// The layout round-trips: what a save writes is what a load reads back, so a tool
// opened and moved comes back opened and moved (D3, D4).
func TestLayoutRoundTrip(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	app, _ := testApp()
	app.SetDisplays([]Rect{{W: 1920, H: 1080}})
	app.OpenTool(ToolDisks)
	app.SetRect(ToolDisks, Rect{X: 100, Y: 120, W: 480, H: 440})
	app.SetRect(ToolTape, Rect{X: 700, Y: 80, W: 440, H: 320})
	app.ToggleMenu() // hidden

	if err := store.SaveLayout(app.Layout()); err != nil {
		t.Fatalf("SaveLayout: %v", err)
	}

	loaded, err := store.LoadLayout()
	if err != nil {
		t.Fatalf("LoadLayout: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadLayout found nothing after SaveLayout wrote a file")
	}

	restored, _ := testApp()
	restored.SetDisplays([]Rect{{W: 1920, H: 1080}})
	restored.SetLayout(*loaded)

	if restored.MenuVisible() {
		t.Error("the hidden menubar came back shown")
	}
	if !openIs(restored, ToolDisks) {
		t.Error("the open tool came back closed")
	}
	for _, w := range restored.Windows() {
		switch w.ID {
		case ToolDisks:
			if w.Rect != (Rect{X: 100, Y: 120, W: 480, H: 440}) {
				t.Errorf("the disk window came back at %+v", w.Rect)
			}
		case ToolTape:
			if w.Rect != (Rect{X: 700, Y: 80, W: 440, H: 320}) {
				t.Errorf("the tape window came back at %+v", w.Rect)
			}
		}
	}
}

// A file that cannot be read is reported and ignored, never fatal: the emulator
// starts with the defaults (D3).
func TestLoadLayoutRejectsRubbish(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir}

	for _, tc := range []struct{ name, body string }{
		{"not json", "this is not a layout"},
		{"empty", ""},
		{"from the future", `{"version":99,"main":{"tool":"main"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(dir, "layout.json"), []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := store.LoadLayout(); err == nil {
				t.Error("a layout file that cannot be read was accepted")
			}
		})
	}
}

// A missing file is a first launch, not a failure, and it reports *nothing to
// restore* rather than an empty layout. That distinction is load-bearing: a zero
// layout means "the menubar is hidden", so a caller that could not tell it from
// "no file" would hide the menu on a first launch, and a user who has never seen
// the menus does not know ESC brings them back (D4).
func TestLoadLayoutWithNoFile(t *testing.T) {
	store := Store{Dir: t.TempDir()}

	layout, err := store.LoadLayout()
	if err != nil {
		t.Fatalf("a missing layout file is not an error: %v", err)
	}
	if layout != nil {
		t.Errorf("a missing file produced a layout: %+v", layout)
	}

	// And the defaults it falls back to show the menubar.
	app, _ := testApp()
	if !app.MenuVisible() {
		t.Error("a first launch hides the menubar")
	}

	// A file that mentions nothing about the menu also leaves it shown: absent is
	// not the same as false.
	if err := os.WriteFile(store.LayoutPath(), []byte(`{"version":1,"tools":[{"tool":"tape","open":true,"w":440,"h":320}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	partial, err := store.LoadLayout()
	if err != nil {
		t.Fatalf("LoadLayout: %v", err)
	}
	app2, _ := testApp()
	app2.SetLayout(*partial)
	if !app2.MenuVisible() {
		t.Error("a layout file that does not mention the menubar hid it")
	}
}

// A layout that names a tool this build does not know keeps the ones it does, and
// one with no tools at all leaves the defaults in place (D3: never fatal, never
// silently destructive).
func TestLoadLayoutToleratesUnknownTools(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir}
	body := `{"version":1,"menu_visible":true,
	  "main":{"tool":"main","x":10,"y":20,"w":800,"h":600},
	  "tools":[{"tool":"disks","open":true,"x":5,"y":6,"w":480,"h":440},
	           {"tool":"holodeck","open":true,"w":100,"h":100}]}`
	if err := os.WriteFile(filepath.Join(dir, "layout.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	loadedPtr, err := store.LoadLayout()
	if err != nil {
		t.Fatalf("LoadLayout: %v", err)
	}
	loaded := *loadedPtr
	if len(loaded.Tools) != 1 || loaded.Tools[0].ID != ToolDisks {
		t.Errorf("tools = %+v, want just the disk window", loaded.Tools)
	}
	if loaded.Main.Rect != (Rect{X: 10, Y: 20, W: 800, H: 600}) {
		t.Errorf("main rect = %+v", loaded.Main.Rect)
	}
}

// A layout is equal to itself and unequal when anything it carries changes, which
// is how the loop knows to write the file (D3).
func TestLayoutEqual(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	app, _ := testApp()
	app.OpenTool(ToolTape)
	base := app.Layout()

	if !base.Equal(app.Layout()) {
		t.Error("a layout differs from itself")
	}

	app.ToggleMenu()
	if base.Equal(app.Layout()) {
		t.Error("hiding the menubar did not change the layout")
	}
	_ = store
}

// menuItems is the entries of one menu by name, which is what the menubar tests
// ask about.
func menuItems(menus []Menu, label string) []MenuItem {
	for _, m := range menus {
		if m.Label == label {
			return m.Items
		}
	}
	return nil
}

// Fast tape is a tape concern, so its control lives in the tape window with the
// transport it affects rather than in the Machine menu. A control in two places is
// two places to change it, and this one had no business being away from the tape.
func TestTurboIsNotInTheMachineMenu(t *testing.T) {
	app, _ := testApp()

	for _, item := range menuItems(app.Menu(), "Machine") {
		if item.Action == ActTurboToggle {
			t.Error("the Machine menu still offers the fast tape toggle")
		}
	}

	// It is still an action, and still bound: the tape window's button dispatches the
	// same one, and F9 works from the keyboard.
	if _, ok := app.Keymap().Lookup(Binding{Key: KeyF9}); !ok {
		t.Error("the fast tape action lost its binding as well as its menu entry")
	}
	if err := app.Dispatch(ActTurboToggle); err != nil {
		t.Errorf("Dispatch(ActTurboToggle): %v", err)
	}
}
