package frontend

import "testing"

// The two matrices that decide what a key does are both tables, and both are
// written to disk as text: the action keymap (D8) and the PC-to-ZX keyboard
// (NEXT_STEPS.md section 6). What a file says and what the code holds have to be
// the same thing in both directions, or a rebind silently changes meaning.

func TestBindingRoundTrip(t *testing.T) {
	for _, text := range []string{
		"q", "f5", "cmd+q", "cmd+shift+q", "cmd+,", "ctrl+alt+delete",
		"escape", "shift+left", "lshift", "cmd+space",
	} {
		b, err := ParseBinding(text)
		if err != nil {
			t.Errorf("ParseBinding(%q): %v", text, err)
			continue
		}
		if got := b.String(); got != text {
			t.Errorf("ParseBinding(%q).String() = %q", text, got)
		}
	}
}

// A binding that is half-understood is an error rather than a binding with a
// missing part: a keymap file that says "hyper+q" must be reported, not quietly
// bound to plain Q.
func TestParseBindingRejectsRubbish(t *testing.T) {
	for _, text := range []string{"", "cmd+", "+q", "cmd+nope", "hyper+q", "cmd+shift"} {
		if b, err := ParseBinding(text); err == nil {
			t.Errorf("ParseBinding(%q) = %v, want an error", text, b)
		}
	}
}

func TestKeyRoundTrip(t *testing.T) {
	for _, k := range []Key{KeyA, KeyZ, KeyZero, KeyNine, KeyEscape, KeyF5, KeyComma, KeySpace, KeyLeftShift} {
		got, ok := ParseKey(k.Name())
		if !ok {
			t.Errorf("ParseKey(%q) failed", k.Name())
			continue
		}
		if got != k {
			t.Errorf("ParseKey(%q) = %v, want %v", k.Name(), got, k)
		}
	}
	if _, ok := ParseKey("hyper"); ok {
		t.Error("ParseKey accepted a key that does not exist")
	}
	if KeyNone.Name() != "?" {
		t.Errorf("KeyNone.Name() = %q, want ?", KeyNone.Name())
	}
}

func TestActionRoundTrip(t *testing.T) {
	for _, a := range []Action{
		ActQuit, ActReset, ActNMI, ActPauseToggle, ActStepOne, ActTurboToggle,
		ActOpenTape, ActOpenDisk, ActEjectDisk, ActFDCTimingToggle,
		ActOpenSnapshot, ActSaveSnapshot, ActRecordReplay,
		ActTapePlayPause, ActTapeRewind, ActScreenshot, ActLogMark,
		ActToggleControl, ActToggleDebugger,
	} {
		got, ok := ParseAction(a.String())
		if !ok {
			t.Errorf("ParseAction(%q) failed", a.String())
			continue
		}
		if got != a {
			t.Errorf("ParseAction(%q) = %v, want %v", a.String(), got, a)
		}
	}
}

// The commands the SDL backend already reports by name have to reach the
// registry unchanged, or the CLI's Cmd+P would stop working the day the backend
// routes its chords through the action table (section 8.2).
func TestTheBackendsChordNamesAreActionNames(t *testing.T) {
	for _, name := range []string{"screenshot", "tape-playpause"} {
		if _, ok := ParseAction(name); !ok {
			t.Errorf("%q is a chord the backend reports but not an action name", name)
		}
	}
}

// D8's example, as a test: Cmd+Q is a host action while a plain Q is the
// machine's row 2 column 0.
func TestLookupSeparatesModifiersFromTheMatrix(t *testing.T) {
	km := DefaultBindings()

	if act, ok := km.Lookup(Binding{Key: KeyQ, Mods: ModSuper}); !ok || act != ActQuit {
		t.Errorf("Cmd+Q = %v (%v), want quit", act, ok)
	}
	if act, ok := km.Lookup(Binding{Key: KeyQ}); ok {
		t.Errorf("plain Q is bound to %v: it belongs to the machine's matrix", act)
	}
	if _, ok := DefaultMatrixMap().Lookup(KeyQ); !ok {
		t.Error("plain Q does not reach the matrix either")
	}
}

// Every tool has a binding. It is what keeps the menubar from being the only way
// to reach a window, and a tool window can cover the menubar (D4).
func TestDefaultBindingsCoverEveryTool(t *testing.T) {
	bound := make(map[Action]int)
	for _, act := range DefaultBindings() {
		bound[act]++
	}
	want := []Action{
		ActToggleControl, ActToggleDisks, ActToggleTape, ActToggleKeyboard,
		ActToggleBindings, ActToggleSettings, ActToggleDebugger,
	}
	for _, act := range want {
		if bound[act] == 0 {
			t.Errorf("%v has no default binding", act)
		}
	}
	if len(want) != len(Tools()) {
		t.Errorf("%d tool actions for %d tools", len(want), len(Tools()))
	}
}

// A binding with no modifiers must not take a key the machine needs. The
// function keys are free because a ZX Spectrum has none; a bare letter would not
// be, and this is the rule that says so rather than a comment that hopes so.
func TestBareBindingsDoNotShadowTheMatrix(t *testing.T) {
	matrix := DefaultMatrixMap()
	for binding, act := range DefaultBindings() {
		if binding.Mods != 0 {
			continue
		}
		if chord, ok := matrix.Lookup(binding.Key); ok {
			t.Errorf("%v is bound to %v with no modifiers, but the machine uses it for %v",
				binding, act, chord)
		}
	}
}

// Merge is what makes a keymap file survive a new version: the file holds the
// user's changes, and everything it does not mention keeps its default.
func TestMergeLaysTheUserOverTheDefaults(t *testing.T) {
	user := Keymap{
		{Key: KeyQ}:                 ActScreenshot, // a new binding
		{Key: KeyP, Mods: ModSuper}: ActNone,       // an explicit unbinding
		{Key: KeyF5}:                ActReset,      // a moved default
	}
	km := Merge(DefaultBindings(), user)

	if act, ok := km.Lookup(Binding{Key: KeyQ}); !ok || act != ActScreenshot {
		t.Errorf("the user's own binding = %v (%v)", act, ok)
	}
	if _, ok := km.Lookup(Binding{Key: KeyP, Mods: ModSuper}); ok {
		t.Error("an entry mapped to ActNone still resolved to an action")
	}
	if act, _ := km.Lookup(Binding{Key: KeyF5}); act != ActReset {
		t.Errorf("the user's replacement = %v, want reset", act)
	}
	if act, ok := km.Lookup(Binding{Key: KeyQ, Mods: ModSuper}); !ok || act != ActQuit {
		t.Errorf("a default the user did not mention = %v (%v), want quit", act, ok)
	}
}
