package frontend

// Apply folds what a backend saw into the app's state. It is the one entry point
// for backend events, so a second backend cannot introduce a second set of rules
// (UI_DESIGN.md section 5.4).
//
// Every case here is a call to a setter that exists on its own - which is what
// makes the event union a transport rather than a place where decisions live. The
// setters are tested directly; this is tested for the wiring.
func (a *App) Apply(events []Event) {
	for _, ev := range events {
		switch ev.Kind {
		case EvKey:
			a.SetCapturing(ev.Captured)
			a.SetDialogOpen(ev.DialogOpen)
			// ESC is window-local (D5 rule 8) and it is handled here rather than in
			// HandleKey, because it depends on *which* window the key came from and
			// HandleKey is about keys, not windows. In the main window it shows and
			// hides the menubar - the only way back if the user hid it, since a
			// hidden menubar has no menu item to show it again. A tool window's ESC
			// is that tool's business, and none of them claims it yet.
			//
			// Rule 9 governs it too: while a widget wants the keyboard, ESC belongs
			// to ImGui, which uses it to close a popup or clear the active item.
			if ev.Key == KeyEscape && ev.Tool == ToolMain {
				if ev.Down && !ev.Captured {
					a.ToggleMenu()
				}
				continue
			}
			a.HandleKey(ev.Tool, ev.Key, ev.Mods, ev.Down)

		case EvCell:
			// An on-screen key. It goes through the same holder counting a physical key
			// does, so a click and a key held on the host keyboard are one ZX key and
			// neither releases the other (D5 rule 4).
			a.MatrixCell(ev.Cell.Row, ev.Cell.Col, ev.Down)

		case EvJoy:
			// A host joystick. It is not a matrix key and shares nothing with one: a
			// Kempston joystick is a port the machine reads, so it goes to the machine
			// through its own method rather than through the matrix at all.
			//
			// Rule 1 is applied here for the same reason it is applied to a key in
			// HandleKey, and from the event rather than from App's own focus: the pad
			// drives the machine while the main window has focus, so a gamepad held in
			// the settings window is not also steering the game behind it. What was
			// held is released on the way out by ReleaseHeld.
			j := ev.Joy
			if ev.Tool != ToolMain {
				j = JoyState{}
			}
			a.SetJoystick(j)

		case EvAction:
			// The menubar and a tool's buttons ask for an action by name; what it
			// does is Dispatch's business, exactly as for a shortcut.
			if err := a.Dispatch(ev.Action); err != nil {
				a.NotifyWarn(err.Error())
			}

		case EvQuit:
			a.quit = true

		case EvFocusChange:
			// Losing focus mid-press must not leave the machine holding a key:
			// SDL does not deliver the releases of keys held when focus goes
			// (D5 rule 4).
			if ev.Tool != a.focused {
				a.ReleaseHeld()
			}
			a.SetFocus(ev.Tool)

		case EvWindowMoved, EvWindowResized:
			a.SetRect(ev.Tool, ev.Rect)

		case EvWindowClosed:
			// Closing the main window is the exit (D4); closing a tool closes that
			// tool.
			if ev.Tool == ToolMain {
				a.quit = true
				continue
			}
			a.CloseTool(ev.Tool)

		case EvDialogResult:
			// The answer to a dialog the app asked for. Which one it was comes from
			// what was asked rather than from the event: the event says what was
			// chosen, and the app already knows what it wanted.
			a.dialogResult = ev.Path
			a.SetDialogOpen(false)
			if err := a.applyDialogResult(ev.Dialog, ev.Path); err != nil {
				a.NotifyWarn(err.Error())
			}
		}
	}
}

// DialogResult is the file the last native dialog returned, or empty for none and
// for a cancelled dialog. M4 is its first consumer (the disk and tape windows);
// until then it is here so an event that arrives is not thrown away.
func (a *App) DialogResult() string { return a.dialogResult }

// MainView is what the main window draws.
func (a *App) MainView() MainView {
	return MainView{
		Screen:      a.Machine.Screen(),
		Menu:        a.Menu(),
		Status:      a.StatusText(),
		MenuVisible: a.menuVisible,
	}
}

// StatusText is the right-aligned menubar read-out (D4): the frame rate, what the
// machine is doing, which machine it is, and the fast-tape flag when it is on.
// Hiding the menubar hides it too, which D4 accepts.
func (a *App) StatusText() string {
	s := a.views.Status
	text := s.Model
	if s.Run == StatePaused {
		text += " | paused"
	}
	if s.Run == StateStep {
		text += " | step"
	}
	if s.FastTape {
		text += " | fast tape"
	}
	return text + " | " + formatFPS(s.FPS)
}
