package frontend

import (
	"fmt"
	"strings"
)

// Cell is one key of the ZX Spectrum's keyboard matrix: 8 half-rows of 5 keys.
// The row and column go straight to Machine.PressKey/PressKey, which is the
// ULA's own numbering.
type Cell struct{ Row, Col int }

// Chord is the set of ZX keys one host key stands for.
//
// A chord rather than a single cell because a ZX Spectrum has no shift-modified keys:
// what a PC keyboard calls BACKSPACE is CAPS SHIFT and 0 pressed together, BREAK is CAPS
// SHIFT and SPACE, and the cursor keys are CAPS SHIFT with 5, 6, 7 or 8. The machine sees
// the matrix, so "together" is all it needs - the ROM's scanner reads the keys that are
// down, not the order they went down in. (A *timed* sequence - press this, wait, press
// that - is a different feature and is not this.)
//
// A chord of one is a plain key, which is what most of the table is.
type Chord []Cell

// MatrixMap is the PC-to-ZX keyboard mapping: which host key stands for which ZX key, or
// which keys. It is the table the on-screen keyboard's bind mode edits (UI_DESIGN.md
// section 9).
//
// The two shifts are the interesting entries. Left shift is CAPS SHIFT (row 0 column 0)
// and right shift is SYMBOL SHIFT (row 7 column 1); both are ordinary matrix keys a user
// holds, not modifiers, which is why they appear here as well as in ModMask - and why a
// chord that contains one is not the same thing as a binding whose ModMask has one.
type MatrixMap map[Key]Chord

// Lookup reports the ZX keys a host key stands for.
func (m MatrixMap) Lookup(k Key) (Chord, bool) {
	chord, ok := m[k]
	return chord, ok
}

// CapsShift and SymbolShift are the two ZX shift keys, which a chord is usually built
// around: most of what a ZX key does *not* print on its face is behind one of them.
var (
	CapsShift   = Cell{Row: 0, Col: 0}
	SymbolShift = Cell{Row: 7, Col: 1}
)

// DefaultMatrixMap is the embedded PC-to-ZX keyboard: the ZX Spectrum's own
// layout, which the host keyboard mirrors almost exactly, with the two shifts
// standing in for the two ZX shifts.
//
// It was the mapping the removed SDL backend carried as a switch, so it is checked
// against the documented layout by test rather than against anything else
// (`TestDefaultMatrixMatchesTheZXLayout`).
func DefaultMatrixMap() MatrixMap {
	m := MatrixMap{
		KeyLeftShift:  {{0, 0}}, // CAPS SHIFT
		KeyRightShift: {{7, 1}}, // SYMBOL SHIFT

		KeyZ: {{0, 1}}, KeyX: {{0, 2}}, KeyC: {{0, 3}}, KeyV: {{0, 4}},
		KeyA: {{1, 0}}, KeyS: {{1, 1}}, KeyD: {{1, 2}}, KeyF: {{1, 3}}, KeyG: {{1, 4}},
		KeyQ: {{2, 0}}, KeyW: {{2, 1}}, KeyE: {{2, 2}}, KeyR: {{2, 3}}, KeyT: {{2, 4}},
		KeyOne: {{3, 0}}, KeyTwo: {{3, 1}}, KeyThree: {{3, 2}}, KeyFour: {{3, 3}}, KeyFive: {{3, 4}},
		KeyZero: {{4, 0}}, KeyNine: {{4, 1}}, KeyEight: {{4, 2}}, KeySeven: {{4, 3}}, KeySix: {{4, 4}},
		KeyP: {{5, 0}}, KeyO: {{5, 1}}, KeyI: {{5, 2}}, KeyU: {{5, 3}}, KeyY: {{5, 4}},
		KeyEnter: {{6, 0}}, KeyL: {{6, 1}}, KeyK: {{6, 2}}, KeyJ: {{6, 3}}, KeyH: {{6, 4}},
		KeySpace: {{7, 0}}, KeyM: {{7, 2}}, KeyN: {{7, 3}}, KeyB: {{7, 4}},
	}

	// The chords. Every one of these is a host key that has no ZX key of its own,
	// standing in for the shifted ZX key a user means by it - which is what the legends
	// in zxKeyboard are for: BACKSPACE is not printed on a ZX keyboard, but DELETE is,
	// above the 0.
	//
	// ESCAPE is deliberately not here: it toggles the menubar (D5 rule 8) and never
	// reaches the matrix, so a chord for it would be dead. BREAK is CAPS SHIFT and
	// SPACE for anyone who wants it from the on-screen keyboard.
	m[KeyBackspace] = Chord{CapsShift, {Row: 4, Col: 0}} // CAPS SHIFT + 0 = DELETE
	m[KeyDelete] = Chord{CapsShift, {Row: 4, Col: 0}}
	m[KeyUp] = Chord{CapsShift, {Row: 4, Col: 3}} // the ZX cursors are 5 6 7 8
	m[KeyDown] = Chord{CapsShift, {Row: 4, Col: 4}}
	m[KeyLeft] = Chord{CapsShift, {Row: 3, Col: 4}}
	m[KeyRight] = Chord{CapsShift, {Row: 4, Col: 2}}
	return m
}

// ZXKeyName is a ZX key's name for a file: its own legend, lowercased with the spaces
// turned into dashes, so CAPS SHIFT is "caps-shift" and the 0 key is "0".
//
// The legends are unique across the keyboard (checked by test), which is what makes them
// usable as names - there is no key whose legend another key shares.
func ZXKeyName(cell Cell) (string, bool) {
	key, ok := ZXKeyAt(cell.Row, cell.Col)
	if !ok {
		return "", false
	}
	return strings.ReplaceAll(strings.ToLower(key.Key), " ", "-"), true
}

// ParseZXKeyName reads a ZX key's name as a file spells it.
func ParseZXKeyName(name string) (Cell, bool) {
	want := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), " ", "-")
	if want == "" {
		return Cell{}, false
	}
	for _, k := range zxKeyboard {
		if strings.ReplaceAll(strings.ToLower(k.Key), " ", "-") == want {
			return Cell{Row: k.Row, Col: k.Col}, true
		}
	}
	return Cell{}, false
}

// String spells a chord the way a file writes it: the names joined with "+", or "none"
// for a host key that is not to reach the machine at all.
func (c Chord) String() string {
	if len(c) == 0 {
		return ZXChordNone
	}
	parts := make([]string, 0, len(c))
	for _, cell := range c {
		name, ok := ZXKeyName(cell)
		if !ok {
			// A cell that is not a key: spelled as itself, so a file that has one says so
			// rather than silently dropping it.
			name = fmt.Sprintf("(%d,%d)", cell.Row, cell.Col)
		}
		parts = append(parts, name)
	}
	return strings.Join(parts, "+")
}

// ZXChordNone is the name a file uses for "this host key does not reach the machine".
const ZXChordNone = "none"

// ParseZXChord reads a chord such as "caps-shift+0" or "symbol-shift+x".
//
// "none" is a valid chord of nothing: it is how a user takes a host key off the machine
// without it falling through to something else.
func ParseZXChord(text string) (Chord, bool) {
	s := strings.ToLower(strings.TrimSpace(text))
	if s == ZXChordNone || s == "" {
		return Chord{}, true
	}
	var chord Chord
	for _, part := range strings.Split(s, "+") {
		cell, ok := ParseZXKeyName(part)
		if !ok {
			return nil, false
		}
		chord = append(chord, cell)
	}
	return chord, true
}

// MergeMatrix lays a user's keyboard mapping over the default one, the way Merge does for
// the action keymap: the user's entries win, including one that takes a host key off the
// machine, and everything they do not mention keeps its default.
func MergeMatrix(base, over MatrixMap) MatrixMap {
	merged := make(MatrixMap, len(base)+len(over))
	for key, chord := range base {
		merged[key] = chord
	}
	for key, chord := range over {
		merged[key] = chord
	}
	return merged
}
