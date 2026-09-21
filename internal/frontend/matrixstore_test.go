package frontend

import (
	"os"
	"strings"
	"testing"
)

// A chord is spelled in a file by the names of the keys in it, and every chord the
// emulator ships can be written down and read back: a name that cannot round-trip is a
// binding a user cannot edit.
func TestChordNamesRoundTrip(t *testing.T) {
	for _, tc := range []string{"caps-shift+0", "caps-shift+7", "symbol-shift+x", "a", "space", "none"} {
		chord, ok := ParseZXChord(tc)
		if !ok {
			t.Errorf("ParseZXChord(%q) failed", tc)
			continue
		}
		if got := chord.String(); got != tc {
			t.Errorf("ParseZXChord(%q).String() = %q", tc, got)
		}
	}

	// Every key of the keyboard has a name, and the names are unique - which is what
	// makes them usable in a file.
	seen := make(map[string]Cell, ZXKeyboardLen)
	for _, key := range zxKeyboard {
		name, ok := ZXKeyName(Cell{Row: key.Row, Col: key.Col})
		if !ok {
			t.Errorf("(%d,%d) has no name", key.Row, key.Col)
			continue
		}
		if other, dup := seen[name]; dup {
			t.Errorf("%q names both (%d,%d) and (%d,%d)",
				name, other.Row, other.Col, key.Row, key.Col)
		}
		seen[name] = Cell{Row: key.Row, Col: key.Col}

		back, ok := ParseZXKeyName(name)
		if !ok || back != (Cell{Row: key.Row, Col: key.Col}) {
			t.Errorf("%q came back as %v (%v)", name, back, ok)
		}
	}

	for _, rubbish := range []string{"caps-shift+nope", "shift+0", "caps-shift+"} {
		if chord, ok := ParseZXChord(rubbish); ok {
			t.Errorf("ParseZXChord(%q) = %v, want an error", rubbish, chord)
		}
	}
}

// The keyboard mapping is editable: a file overrides the emulator's own mapping for the
// keys it mentions and leaves the rest alone. BACKSPACE is the case that matters, because
// the emulator maps it to CAPS SHIFT and 0 and a user may well want it elsewhere.
func TestMatrixFileOverridesTheDefaults(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	body := `{
	  "version": 1,
	  "keys": {
	    "backspace": "symbol-shift+x",
	    "up": "caps-shift+7",
	    "tab": "none"
	  }
	}`
	if err := os.WriteFile(store.MatrixPath(), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	user, err := store.LoadMatrix()
	if err != nil {
		t.Fatalf("LoadMatrix: %v", err)
	}
	app, m := testApp()
	app.SetMatrix(MergeMatrix(DefaultMatrixMap(), user))

	// BACKSPACE is where the file put it: SYMBOL SHIFT and X, which the ZX knows as the
	// pound sign. The default chord is gone with it.
	app.HandleKey(ToolMain, KeyBackspace, 0, true)
	if !m.down[SymbolShift] || !m.down[Cell{Row: 0, Col: 2}] {
		t.Errorf("BACKSPACE pressed %v, want SYMBOL SHIFT and X", m.pressed)
	}
	if m.down[CapsShift] && m.down[Cell{Row: 4, Col: 0}] {
		t.Error("the default chord for BACKSPACE fired as well as the file's")
	}
	app.HandleKey(ToolMain, KeyBackspace, 0, false)

	// A key the file did not mention keeps the default, which the emulator does not ship
	// (UP is a chord) - the merge is over the defaults, not a replacement of them.
	if _, ok := MergeMatrix(DefaultMatrixMap(), user).Lookup(KeyLeft); !ok {
		t.Error("a host key the file did not mention lost its mapping")
	}

	// And "none" takes a host key off the machine.
	before := len(m.pressed)
	app.HandleKey(ToolMain, KeyTab, 0, true)
	if len(m.pressed) != before {
		t.Errorf("TAB reached the machine after the file took it off: %v", m.pressed[len(m.pressed)-1])
	}
}

// A file that cannot be read is reported and the rest applies, and one that is not there
// is a user who has not changed anything.
func TestMatrixFileToleratesRubbish(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir}

	if m, err := store.LoadMatrix(); err != nil || m != nil {
		t.Errorf("a missing keyboard map = %v, %v; want nothing and no error", m, err)
	}

	body := `{"version":1,"keys":{"backspace":"caps-shift+0","hyper+q":"a","a":"nope"}}`
	if err := os.WriteFile(store.MatrixPath(), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadMatrix()
	if err == nil {
		t.Fatal("entries that cannot be read were accepted silently")
	}
	if !strings.Contains(err.Error(), "hyper+q: a") || !strings.Contains(err.Error(), "a: nope") {
		t.Errorf("the report does not name both entries: %v", err)
	}
	if len(loaded) != 1 {
		t.Errorf("read %d entries, want just the good one: %v", len(loaded), loaded)
	}
}

// What the editor will write: the difference against the defaults, so a key back to its
// default is left out and one the user took off the machine is written as "none".
func TestDiffMatrix(t *testing.T) {
	base := DefaultMatrixMap()

	current := MergeMatrix(base, MatrixMap{
		KeyBackspace: {SymbolShift, {Row: 0, Col: 2}}, // moved
		KeyLeft:      {},                              // taken off the machine
		KeyTab:       {},                              // and a key that had no default
	})
	diff := DiffMatrix(base, current)

	// Two changes: one moved, one removed. Tab is empty in both, so there is nothing to
	// record for it - a key the default map does not mention cannot be taken off it.
	if len(diff) != 2 {
		t.Fatalf("the diff has %d entries, want the two changes: %v", len(diff), diff)
	}
	if got := diff[KeyBackspace].String(); got != "symbol-shift+x" {
		t.Errorf("the moved key is recorded as %q", got)
	}
	if got := diff[KeyLeft].String(); got != "none" {
		t.Errorf("the removed key is recorded as %q, want none", got)
	}
	// An unchanged key is not in the diff at all - the file holds the changes.
	if _, ok := diff[KeyUp]; ok {
		t.Error("an unchanged key is in the diff")
	}
	// And applying the diff to the defaults gives back the keyboard it came from.
	if got := MergeMatrix(base, diff); !chordEqual(got[KeyBackspace], current[KeyBackspace]) {
		t.Error("the diff does not reproduce the keyboard it came from")
	}
}
