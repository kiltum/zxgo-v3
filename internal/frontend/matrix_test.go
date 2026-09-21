package frontend

import "testing"

// The PC-to-ZX keyboard has to cover the machine's whole matrix exactly once. Two
// host keys on one ZX key would be a keyboard with a dead key on it, and a
// missing entry is a ZX key no host key can press - including the one the
// on-screen keyboard's bind mode would be editing.
func TestDefaultMatrixCoversEveryZXKeyOnce(t *testing.T) {
	const zxKeys = 40 // 8 half-rows of 5

	m := DefaultMatrixMap()

	seen := make(map[Cell]Key, zxKeys)
	single := 0
	for key, chord := range m {
		if len(chord) == 0 {
			t.Errorf("%v maps to nothing", key)
			continue
		}
		for _, cell := range chord {
			if cell.Row < 0 || cell.Row > 7 || cell.Col < 0 || cell.Col > 4 {
				t.Errorf("%v maps to (%d,%d), which is outside the 8x5 matrix",
					key, cell.Row, cell.Col)
			}
		}
		// Only the plain keys have to be unique: a chord deliberately reuses cells -
		// CAPS SHIFT is in all six of them - and a duplicate *plain* key would be a ZX
		// key no host key can reach.
		if len(chord) != 1 {
			continue
		}
		single++
		if other, dup := seen[chord[0]]; dup {
			t.Errorf("%v and %v both map to %v", other, key, chord[0])
		}
		seen[chord[0]] = key
	}
	if single != zxKeys {
		t.Errorf("%d host keys stand for exactly one ZX key, want one for each of the %d",
			single, zxKeys)
	}
	for row := 0; row < 8; row++ {
		for col := 0; col < 5; col++ {
			if _, ok := seen[Cell{Row: row, Col: col}]; !ok {
				t.Errorf("(%d,%d) has no host key", row, col)
			}
		}
	}
}

// The rows the ZX Spectrum labels, pinned as they are documented, because a
// mapping that is merely plausible still produces a keyboard that types the
// wrong thing.
func TestDefaultMatrixMatchesTheZXLayout(t *testing.T) {
	m := DefaultMatrixMap()
	for _, tc := range []struct {
		key  Key
		cell Cell
		what string
	}{
		{KeyLeftShift, Cell{0, 0}, "CAPS SHIFT"},
		{KeyRightShift, Cell{7, 1}, "SYMBOL SHIFT"},
		{KeyZ, Cell{0, 1}, "Z"},
		{KeyV, Cell{0, 4}, "V"},
		{KeyA, Cell{1, 0}, "A"},
		{KeyQ, Cell{2, 0}, "Q"},
		{KeyOne, Cell{3, 0}, "1"},
		{KeyFive, Cell{3, 4}, "5"},
		{KeyZero, Cell{4, 0}, "0"},
		{KeySix, Cell{4, 4}, "6"},
		{KeyP, Cell{5, 0}, "P"},
		{KeyY, Cell{5, 4}, "Y"},
		{KeyEnter, Cell{6, 0}, "ENTER"},
		{KeyH, Cell{6, 4}, "H"},
		{KeySpace, Cell{7, 0}, "SPACE"},
		{KeyM, Cell{7, 2}, "M"},
		{KeyB, Cell{7, 4}, "B"},
	} {
		chord, ok := m.Lookup(tc.key)
		if !ok {
			t.Errorf("%s (%v) has no ZX key", tc.what, tc.key)
			continue
		}
		if len(chord) != 1 || chord[0] != tc.cell {
			t.Errorf("%s = %v, want just %v", tc.what, chord, tc.cell)
		}
	}

	// The one that is easy to get wrong: SYMBOL SHIFT and M share half-row 7, so
	// M is column 2 and the shift is column 1. A map that put M at column 1 would
	// look right and type M every time the user meant SYMBOL SHIFT.
	if chord, _ := m.Lookup(KeyM); len(chord) == 1 && chord[0].Col == 1 {
		t.Error("M is on SYMBOL SHIFT's column")
	}
}

// A chord presses its keys together and lets go of them together, which is what a ZX
// Spectrum needs: it has no shift-modified keys, so BACKSPACE is CAPS SHIFT and 0 held
// down at the same time, and the ROM's scanner reads what is down rather than the order
// it went down in.
func TestChordsPressAndReleaseTogether(t *testing.T) {
	app, m := testApp()

	// BACKSPACE is CAPS SHIFT and 0, which the machine knows as DELETE.
	//
	// HandleKey's second return is "the keymap claimed the edge", so it is false for a
	// key the machine gets: what says the key arrived is the machine's own state.
	app.HandleKey(ToolMain, KeyBackspace, 0, true)
	if len(m.pressed) != 2 {
		t.Fatalf("BACKSPACE pressed %v, want CAPS SHIFT and 0", m.pressed)
	}
	want := []Cell{CapsShift, {Row: 4, Col: 0}}
	for _, cell := range want {
		if !m.down[cell] {
			t.Errorf("%v is not down after BACKSPACE", cell)
		}
	}

	app.HandleKey(ToolMain, KeyBackspace, 0, false)
	for _, cell := range want {
		if m.down[cell] {
			t.Errorf("%v is still down after the release", cell)
		}
	}
	if len(m.released) != 2 {
		t.Errorf("released %v, want both keys of the chord", m.released)
	}

	// The cursor keys are the other three: CAPS SHIFT with 5, 6, 7 and 8.
	for _, tc := range []struct {
		key  Key
		cell Cell
	}{
		{KeyLeft, Cell{Row: 3, Col: 4}},
		{KeyDown, Cell{Row: 4, Col: 4}},
		{KeyUp, Cell{Row: 4, Col: 3}},
		{KeyRight, Cell{Row: 4, Col: 2}},
	} {
		chord, ok := DefaultMatrixMap().Lookup(tc.key)
		if !ok {
			t.Errorf("%v has no ZX key", tc.key)
			continue
		}
		if len(chord) != 2 || chord[0] != CapsShift || chord[1] != tc.cell {
			t.Errorf("%v = %v, want CAPS SHIFT and %v", tc.key, chord, tc.cell)
		}
	}
}

// A chord that contains a key the *host* keyboard is also holding does not let go of it
// early: the holder counting is per cell, so the two sources are one ZX key (D5 rule 4).
//
// CAPS SHIFT is the case that matters, because it is in every chord and is also a key a
// user holds on its own.
func TestAChordDoesNotReleaseACellTheHostStillHolds(t *testing.T) {
	app, m := testApp()

	// The user holds left shift, which is CAPS SHIFT, and then presses BACKSPACE, whose
	// chord is CAPS SHIFT and 0.
	app.HandleKey(ToolMain, KeyLeftShift, 0, true)
	app.HandleKey(ToolMain, KeyBackspace, 0, true)
	app.HandleKey(ToolMain, KeyBackspace, 0, false)

	if !m.down[CapsShift] {
		t.Error("let go of CAPS SHIFT while the host key for it is still held")
	}
	if m.down[Cell{Row: 4, Col: 0}] {
		t.Error("the 0 of the chord is still down")
	}

	// And letting go of the host shift finally releases it.
	app.HandleKey(ToolMain, KeyLeftShift, 0, false)
	if m.down[CapsShift] {
		t.Error("CAPS SHIFT is still down after the host key was released")
	}
}
