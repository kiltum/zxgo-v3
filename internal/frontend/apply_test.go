package frontend

import (
	"testing"
)

// A backend's events go in through one door, and every case is a call to a setter
// that was tested on its own. What this checks is the wiring: that an event reaches
// the setter it names.
func TestApplyReachesTheSetters(t *testing.T) {
	t.Run("key", func(t *testing.T) {
		app, m := testApp()
		app.Apply([]Event{{Kind: EvKey, Key: KeyA, Down: true}})
		if len(m.pressed) != 1 {
			t.Errorf("an EvKey did not reach the machine: %v", m.pressed)
		}
	})

	t.Run("action", func(t *testing.T) {
		app, m := testApp()
		app.Apply([]Event{{Kind: EvAction, Action: ActReset}})
		if m.resets != 1 {
			t.Errorf("an EvAction did not dispatch: %d resets", m.resets)
		}
	})

	t.Run("quit", func(t *testing.T) {
		app, _ := testApp()
		app.Apply([]Event{{Kind: EvQuit}})
		if !app.Quitting() {
			t.Error("an EvQuit did not stop the loop")
		}
	})

	t.Run("focus", func(t *testing.T) {
		app, _ := testApp()
		app.Apply([]Event{{Kind: EvFocusChange, Tool: ToolDebugger}})
		if app.Focused() != ToolDebugger {
			t.Errorf("focus = %v, want the debugger", app.Focused())
		}
	})

	t.Run("window moved", func(t *testing.T) {
		app, _ := testApp()
		app.OpenTool(ToolTape)
		app.Apply([]Event{{Kind: EvWindowMoved, Tool: ToolTape, Rect: Rect{X: 40, Y: 50, W: 440, H: 320}}})
		for _, w := range app.Windows() {
			if w.ID == ToolTape && (w.Rect.X != 40 || w.Rect.Y != 50) {
				t.Errorf("the tape window is at %+v, want (40,50)", w.Rect)
			}
		}
	})

	t.Run("window closed", func(t *testing.T) {
		app, _ := testApp()
		app.OpenTool(ToolDisks)
		app.Apply([]Event{{Kind: EvWindowClosed, Tool: ToolDisks}})
		if openIs(app, ToolDisks) {
			t.Error("closing a tool's window left it open")
		}
	})

	t.Run("dialog", func(t *testing.T) {
		app, _ := testApp()
		app.Apply([]Event{{Kind: EvDialogResult, Path: "/tmp/game.tzx"}})
		if got := app.DialogResult(); got != "/tmp/game.tzx" {
			t.Errorf("DialogResult = %q", got)
		}
	})
}

// Closing the main window is the exit (D4), not a window operation: it is the one
// window with no open flag to clear.
func TestApplyClosingTheMainWindowQuits(t *testing.T) {
	app, _ := testApp()

	app.Apply([]Event{{Kind: EvWindowClosed, Tool: ToolMain}})

	if !app.Quitting() {
		t.Error("closing the main window did not ask to quit")
	}
}

// Losing focus mid-press must not leave the machine holding a key: SDL does not
// deliver the releases of keys held when focus goes (D5 rule 4).
func TestApplyFocusChangeReleasesHeldKeys(t *testing.T) {
	app, m := testApp()
	app.HandleKey(ToolMain, KeyA, 0, true)

	app.Apply([]Event{{Kind: EvFocusChange, Tool: ToolDebugger}})

	if len(m.released) != 1 {
		t.Errorf("released %v, want the held key", m.released)
	}
	// And a release that arrives afterwards is not delivered twice.
	app.HandleKey(ToolMain, KeyA, 0, false)
	if len(m.released) != 1 {
		t.Errorf("released %v after the focus change, want no more", m.released)
	}
}

// The capture flag rides on the key event, because only the toolkit knows when a
// widget wants the keyboard and only the front end knows what that means.
func TestApplyTakesCaptureFromTheKeyEvent(t *testing.T) {
	app, m := testApp()

	app.Apply([]Event{{Kind: EvKey, Key: KeyA, Down: true, Captured: true}})
	if len(m.pressed) != 0 {
		t.Error("a captured key reached the machine")
	}

	app.Apply([]Event{{Kind: EvKey, Key: KeyA, Down: true, Captured: false}})
	if len(m.pressed) != 1 {
		t.Error("an uncaptured key did not reach the machine")
	}
}

// ESC is window-local (D5 rule 8). In the main window it toggles the menubar, which
// is the only way back from a hidden menu - a hidden menubar has no menu item to
// show it again - and it is not a binding, so it is not in the keymap.
func TestEscapeTogglesTheMenubar(t *testing.T) {
	app, m := testApp()

	if !app.MenuVisible() {
		t.Fatal("test precondition: the menubar should start shown")
	}

	app.Apply([]Event{{Kind: EvKey, Key: KeyEscape, Down: true, Tool: ToolMain}})
	if app.MenuVisible() {
		t.Error("ESC did not hide the menubar")
	}
	app.Apply([]Event{{Kind: EvKey, Key: KeyEscape, Down: true, Tool: ToolMain}})
	if !app.MenuVisible() {
		t.Error("ESC did not show the menubar again")
	}

	// The release is not a second toggle, and ESC never reaches the machine.
	app.Apply([]Event{{Kind: EvKey, Key: KeyEscape, Down: false, Tool: ToolMain}})
	if !app.MenuVisible() {
		t.Error("the release of ESC toggled the menubar again")
	}
	if len(m.pressed) != 0 || len(m.released) != 0 {
		t.Errorf("ESC reached the machine: %v / %v", m.pressed, m.released)
	}
}

// A tool window owns its own ESC (D5 rule 8), and none of them claims it yet, so it
// does nothing there rather than reaching the menubar behind it.
func TestEscapeIsWindowLocal(t *testing.T) {
	app, _ := testApp()

	app.Apply([]Event{{Kind: EvKey, Key: KeyEscape, Down: true, Tool: ToolDebugger}})

	if !app.MenuVisible() {
		t.Error("ESC in a tool window reached the main window's menubar")
	}
}

// While a widget wants the keyboard, ESC belongs to the toolkit, which uses it to
// close a popup or clear the active item (D5 rule 9).
func TestEscapeYieldsToAWidget(t *testing.T) {
	app, _ := testApp()

	app.Apply([]Event{{Kind: EvKey, Key: KeyEscape, Down: true, Tool: ToolMain, Captured: true}})

	if !app.MenuVisible() {
		t.Error("ESC was taken from a widget that wanted the keyboard")
	}
}
