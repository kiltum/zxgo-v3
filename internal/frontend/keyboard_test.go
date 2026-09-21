package frontend

import (
	"testing"

	"github.com/kiltum/zxgo-v3/internal/emulator"
	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/replay"
	"github.com/kiltum/zxgo-v3/pkg/sound"
)

// The keyboard table covers the matrix exactly: forty keys, one per cell, no cell
// twice and none missing. A table with a hole in it draws a keyboard with a dead key,
// and the matrix is small enough to check exhaustively.
func TestZXKeyboardCoversTheMatrix(t *testing.T) {
	if len(zxKeyboard) != ZXKeyboardLen {
		t.Fatalf("the table has %d keys, want %d", len(zxKeyboard), ZXKeyboardLen)
	}

	seen := make(map[[2]int]string, ZXKeyboardLen)
	for _, k := range zxKeyboard {
		if k.Row < 0 || k.Row > 7 || k.Col < 0 || k.Col > 4 {
			t.Errorf("%q is at (%d,%d), outside the 8x5 matrix", k.Key, k.Row, k.Col)
		}
		if k.Key == "" {
			t.Errorf("the key at (%d,%d) has no legend", k.Row, k.Col)
		}
		if other, dup := seen[[2]int{k.Row, k.Col}]; dup {
			t.Errorf("%q and %q are both at (%d,%d)", other, k.Key, k.Row, k.Col)
		}
		seen[[2]int{k.Row, k.Col}] = k.Key
	}
	for row := 0; row < 8; row++ {
		for col := 0; col < 5; col++ {
			if _, ok := seen[[2]int{row, col}]; !ok {
				t.Errorf("(%d,%d) has no key", row, col)
			}
		}
	}
}

// A few keys are pinned against the reference the table was taken from
// (docs/Sinclair ZX Spectrum keyboard layout.html), because the legends are exactly the
// sort of data that can be silently shifted by one: a table with the right shape and
// the wrong legends draws a keyboard that types the wrong thing.
func TestZXKeyboardLegends(t *testing.T) {
	for _, tc := range []struct {
		row, col    int
		key, caps   string
		symbol, ext string
		why         string
	}{
		{0, 0, "CAPS SHIFT", "", "", "", "the left-hand shift, which is a key of the matrix"},
		{7, 1, "SYMBOL SHIFT", "", "", "", "the right-hand one, in half-row 7"},
		{7, 0, "SPACE", "BREAK", "", "", "the space bar, whose CAPS SHIFT legend is BREAK"},
		{6, 0, "ENTER", "", "", "", "the return key"},
		{2, 0, "Q", "PLOT", "<=", "SIN ASN", "a letter: two keywords on both shifts"},
		{1, 0, "A", "NEW", "STOP", "READ ~", "and another"},
		{0, 2, "X", "CLEAR", "£", "EXP INK", "the pound sign, which is SYMBOL SHIFT and X"},
	} {
		k, ok := ZXKeyAt(tc.row, tc.col)
		if !ok {
			t.Errorf("(%d,%d) has no key: %s", tc.row, tc.col, tc.why)
			continue
		}
		if k.Key != tc.key || k.Caps != tc.caps || k.Symbol != tc.symbol || k.Extended != tc.ext {
			t.Errorf("(%d,%d) = %q/%q/%q/%q, want %q/%q/%q/%q (%s)",
				tc.row, tc.col, k.Key, k.Caps, k.Symbol, k.Extended,
				tc.key, tc.caps, tc.symbol, tc.ext, tc.why)
		}
	}

	// A cell outside the matrix is not a key, and asking for one says so rather than
	// returning a zero entry that looks like a blank key.
	if _, ok := ZXKeyAt(8, 0); ok {
		t.Error("half-row 8 is not part of the matrix")
	}
}

// The drawing order covers the matrix exactly too, and maps to the same keys the matrix
// order does: two orders, one keyboard. Getting this wrong draws a keyboard whose keys
// are in the wrong places while every legend is right, which is the sort of thing that
// looks correct until someone types on it.
func TestKeyboardRowsAreTheSameKeyboard(t *testing.T) {
	rows := KeyboardRows()

	seen := make(map[Cell]string, ZXKeyboardLen)
	for _, row := range rows {
		for _, cell := range row {
			key, ok := ZXKeyAt(cell.Row, cell.Col)
			if !ok {
				t.Errorf("(%d,%d) is drawn but is not a key", cell.Row, cell.Col)
				continue
			}
			if other, dup := seen[cell]; dup {
				t.Errorf("%q and %q are both drawn at (%d,%d)", other, key.Key, cell.Row, cell.Col)
			}
			seen[cell] = key.Key
		}
	}
	if len(seen) != ZXKeyboardLen {
		t.Errorf("the drawing covers %d keys, want %d", len(seen), ZXKeyboardLen)
	}

	// The rows as a user sees them: the digits left to right, then Q, A and Z.
	for _, tc := range []struct {
		pos  Cell
		want string
	}{
		{Cell{0, 0}, "1"}, {Cell{0, 9}, "0"},
		{Cell{1, 0}, "Q"}, {Cell{1, 9}, "P"},
		{Cell{2, 0}, "A"}, {Cell{2, 9}, "ENTER"},
		{Cell{3, 0}, "CAPS SHIFT"}, {Cell{3, 9}, "SPACE"},
	} {
		key, ok := ZXKeyAt(rows[tc.pos.Row][tc.pos.Col].Row, rows[tc.pos.Row][tc.pos.Col].Col)
		if !ok {
			t.Fatalf("row %d position %d is not a key", tc.pos.Row, tc.pos.Col)
		}
		if key.Key != tc.want {
			t.Errorf("row %d position %d is %q, want %q", tc.pos.Row, tc.pos.Col, key.Key, tc.want)
		}
	}
}

// An on-screen key drives the matrix and records to a replay, which is M5's first *done
// when*.
//
// It uses the real emulator and a real recorder, because the claim is about what the
// machine and the recording see: a fake would only be asserting that the fake was called.
func TestOnScreenKeyDrivesTheMatrixAndRecords(t *testing.T) {
	emu := emulator.New(model.Spectrum48K, &sound.NullOutput{})
	rec := replay.NewRecorder()
	emu.SetRecorder(rec)

	app := New(NewMachine(emu))
	app.Notice = func(string) {}
	app.Warn = func(string) {}

	// Q is half-row 2 bit 0. The backend reports the two edges of the click, because a
	// matrix wants edges and ImGui reports whether a key is held now.
	q := Cell{Row: 2, Col: 0}
	app.Apply([]Event{{Kind: EvCell, Tool: ToolKeyboard, Cell: q, Down: true}})
	if !emu.ULA().IsKeyDown(q.Row, q.Col) {
		t.Error("a click on an on-screen key did not press the machine's key")
	}
	emu.RunFrame()
	app.Apply([]Event{{Kind: EvCell, Tool: ToolKeyboard, Cell: q, Down: false}})
	if emu.ULA().IsKeyDown(q.Row, q.Col) {
		t.Error("the release did not take the machine's key up")
	}

	events := rec.Events()
	if len(events) != 2 {
		t.Fatalf("recorded %d events, want a press and a release: %+v", len(events), events)
	}
	if events[0].Key == nil || !events[0].Key.Down || events[0].Key.Row != 2 || events[0].Key.Col != 0 {
		t.Errorf("first event = %+v, want a press of (2,0)", events[0])
	}
	if events[1].Key == nil || events[1].Key.Down {
		t.Errorf("second event = %+v, want a release", events[1])
	}
}

// The keyboard window is not focusable, so clicking it never takes keyboard focus from
// the machine - and its clicks reach the machine whatever window the front end thinks has
// focus, because a click on a key is the machine's key rather than a host key that has to
// be routed (D5 rule 1 is about host keys).
func TestOnScreenKeysAreNotGatedByWindowFocus(t *testing.T) {
	app, m := testApp()
	app.OpenTool(ToolKeyboard) // a tool window is "focused" as far as the front end knows

	app.Apply([]Event{{Kind: EvCell, Tool: ToolKeyboard, Cell: Cell{Row: 7, Col: 0}, Down: true}})

	if len(m.pressed) != 1 {
		t.Errorf("a click on an on-screen key was dropped: %v", m.pressed)
	}
	if got := m.pressed[0]; got != (Cell{Row: 7, Col: 0}) {
		t.Errorf("pressed %v, want the space bar", got)
	}
}
