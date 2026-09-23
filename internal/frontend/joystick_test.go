package frontend

import "testing"

// The joystick is the machine's input, not a widget's: it is gated on which window
// has focus, and a state that has not changed is not written again. Both of those
// are rules of this package, so both are tested here rather than through a
// backend.

func TestJoystickReachesTheMachine(t *testing.T) {
	app, m := testApp()
	app.Apply([]Event{{Kind: EvJoy, Tool: ToolMain, Joy: JoyState{Right: true, Fire: true}}})

	if len(m.joy) != 1 {
		t.Fatalf("the machine was told %d times, want 1: %v", len(m.joy), m.joy)
	}
	if got := m.joy[0]; got != (JoyState{Right: true, Fire: true}) {
		t.Errorf("joystick = %+v, want right and fire", got)
	}
}

// A pad resting at centre reports nothing to write: the state would be identical to
// the one the machine already has, and a replay of a session with a pad plugged in
// would otherwise be nothing but repetitions of "still centred".
func TestJoystickIsWrittenOnlyWhenItChanges(t *testing.T) {
	app, m := testApp()
	held := JoyState{Up: true}
	app.Apply([]Event{
		{Kind: EvJoy, Tool: ToolMain, Joy: held},
		{Kind: EvJoy, Tool: ToolMain, Joy: held},
		{Kind: EvJoy, Tool: ToolMain, Joy: JoyState{}},
		{Kind: EvJoy, Tool: ToolMain, Joy: JoyState{}},
	})

	if len(m.joy) != 2 {
		t.Fatalf("writes = %d, want 2 (held, released): %v", len(m.joy), m.joy)
	}
	if m.joy[0] != held || m.joy[1] != (JoyState{}) {
		t.Errorf("writes = %v, want the hold then the release", m.joy)
	}
}

// The joystick drives the machine only while the main window has focus (D5 rule 1,
// applied to a device that is not the keyboard), and goes back to a released stick
// when focus leaves rather than staying where it was.
func TestJoystickFollowsFocus(t *testing.T) {
	t.Run("a pad read in another window never reaches the machine", func(t *testing.T) {
		app, m := testApp()
		app.SetFocus(ToolDebugger)
		app.Apply([]Event{{Kind: EvJoy, Tool: ToolDebugger, Joy: JoyState{Left: true}}})

		// Nothing is written at all, and nothing needs to be: the machine's port is
		// already released, which is the state the gate resolves the pad to.
		if len(m.joy) != 0 {
			t.Errorf("writes = %v, want none", m.joy)
		}
	})

	t.Run("losing focus releases what was held", func(t *testing.T) {
		app, m := testApp()
		app.Apply([]Event{{Kind: EvJoy, Tool: ToolMain, Joy: JoyState{Left: true}}})
		app.Apply([]Event{{Kind: EvFocusChange, Tool: ToolTape}})

		if len(m.joy) != 2 {
			t.Fatalf("writes = %d, want the hold then the release: %v", len(m.joy), m.joy)
		}
		if last := m.joy[len(m.joy)-1]; last != (JoyState{}) {
			t.Errorf("the last write was %+v, want a released stick", last)
		}
	})

	// The direction is held across the switch: the front end released it when focus
	// went, so the backend reports it again when focus comes back, and the machine
	// has to be told even though the front end's own record never changed.
	t.Run("regaining focus re-writes what is held", func(t *testing.T) {
		app, m := testApp()
		held := JoyState{Right: true}
		app.Apply([]Event{
			{Kind: EvJoy, Tool: ToolMain, Joy: held},
			{Kind: EvFocusChange, Tool: ToolTape},
			{Kind: EvJoy, Tool: ToolMain, Joy: held},
		})

		if len(m.joy) != 3 {
			t.Fatalf("writes = %d, want 3: %v", len(m.joy), m.joy)
		}
		if last := m.joy[len(m.joy)-1]; last != held {
			t.Errorf("the last write was %+v, want the held direction back", last)
		}
	})
}

// A reset clears the machine's port, so the front end writes the state it knows
// back: a stick held across a reset would otherwise be a stick the machine cannot
// see, because nothing about it has changed for the front end to report.
func TestResetRestoresTheJoystick(t *testing.T) {
	app, m := testApp()
	held := JoyState{Down: true, Fire: true}
	app.Apply([]Event{{Kind: EvJoy, Tool: ToolMain, Joy: held}})

	if err := app.Dispatch(ActReset); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if m.resets != 1 {
		t.Fatalf("resets = %d, want 1", m.resets)
	}
	if len(m.joy) != 2 || m.joy[1] != held {
		t.Errorf("writes after the reset = %v, want the held state written again", m.joy)
	}

	// No reset, no rewrite: the rule is about the machine having been cleared, not
	// about the joystick being written whenever anything happens.
	app2, m2 := testApp()
	app2.Apply([]Event{{Kind: EvJoy, Tool: ToolMain, Joy: held}})
	app2.Apply([]Event{{Kind: EvJoy, Tool: ToolMain, Joy: held}})
	if len(m2.joy) != 1 {
		t.Errorf("writes = %v, want one", m2.joy)
	}
}

func TestJoyStateAny(t *testing.T) {
	if (JoyState{}).Any() {
		t.Error("a released stick reported as held")
	}
	for _, j := range []JoyState{
		{Right: true}, {Left: true}, {Down: true}, {Up: true}, {Fire: true},
	} {
		if !j.Any() {
			t.Errorf("%+v did not report as held", j)
		}
	}
}
