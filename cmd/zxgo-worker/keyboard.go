package main

import "github.com/kiltum/zxgo-v3/internal/emulator"

// keyPress is one ZX keyboard matrix key plus optional shift keys.
type keyPress struct {
	row   int
	col   int
	shift bool // SYMBOL SHIFT (row 7, col 1)
	caps  bool // CAPS SHIFT (row 0, col 0)
}

// keyMap maps an ASCII rune to its keyboard matrix position. Letters and digits
// are unshifted; symbols need SYMBOL SHIFT. Letter positions are exact; the
// symbol positions are the standard 48K layout.
var keyMap = map[rune]keyPress{
	'A': {1, 0, false, false}, 'B': {7, 4, false, false}, 'C': {0, 3, false, false},
	'D': {1, 2, false, false}, 'E': {2, 2, false, false}, 'F': {1, 3, false, false},
	'G': {1, 4, false, false}, 'H': {6, 4, false, false}, 'I': {5, 2, false, false},
	'J': {6, 3, false, false}, 'K': {6, 2, false, false}, 'L': {6, 1, false, false},
	'M': {7, 2, false, false}, 'N': {7, 3, false, false}, 'O': {5, 1, false, false},
	'P': {5, 0, false, false}, 'Q': {2, 0, false, false}, 'R': {2, 3, false, false},
	'S': {1, 1, false, false}, 'T': {2, 4, false, false}, 'U': {5, 3, false, false},
	'V': {0, 4, false, false}, 'W': {2, 1, false, false}, 'X': {0, 2, false, false},
	'Y': {5, 4, false, false}, 'Z': {0, 1, false, false},

	'0': {4, 0, false, false}, '1': {3, 0, false, false}, '2': {3, 1, false, false},
	'3': {3, 2, false, false}, '4': {3, 3, false, false}, '5': {3, 4, false, false},
	'6': {4, 4, false, false}, '7': {4, 3, false, false}, '8': {4, 2, false, false},
	'9': {4, 1, false, false},

	' ':  {7, 0, false, false},
	'\n': {6, 0, false, false}, '\r': {6, 0, false, false},

	// Symbols (SYMBOL SHIFT held). From docs/keyboard_cheat.html.
	'"':  {5, 0, true, false}, // SS+P
	'\'': {4, 3, true, false}, // SS+7
	'.':  {7, 2, true, false}, // SS+M
	',':  {7, 3, true, false}, // SS+N
	':':  {0, 1, true, false}, // SS+Z
	';':  {5, 1, true, false}, // SS+O
	'?':  {0, 3, true, false}, // SS+C
	'!':  {3, 0, true, false}, // SS+1
	'(':  {4, 2, true, false}, // SS+8
	')':  {4, 1, true, false}, // SS+9
	'-':  {6, 3, true, false}, // SS+J
	'+':  {6, 2, true, false}, // SS+K
	'=':  {6, 1, true, false}, // SS+L
	'*':  {7, 4, true, false}, // SS+B
	'/':  {0, 4, true, false}, // SS+V
	'#':  {3, 2, true, false}, // SS+3
	'$':  {3, 3, true, false}, // SS+4
	'%':  {3, 4, true, false}, // SS+5
	'&':  {4, 4, true, false}, // SS+6
	'@':  {3, 1, true, false}, // SS+2
	'<':  {2, 3, true, false}, // SS+R
	'>':  {2, 4, true, false}, // SS+T
	'_':  {4, 0, true, false}, // SS+0
}

// keywordKeys maps an uppercase TR-DOS keyword to the key presses that produce
// it in tokenized input (from docs/keyboard_cheat.html). K-mode keywords are a
// single unshifted key; E-mode keywords need CS+SS (extended mode) first, then
// SYMBOL SHIFT + key.
var keywordKeys = map[string][]keyPress{
	"RUN":    {{row: 2, col: 3}},
	"LOAD":   {{row: 6, col: 3}},
	"SAVE":   {{row: 1, col: 1}},
	"NEW":    {{row: 1, col: 0}},
	"LIST":   {{row: 6, col: 2}},
	"CLEAR":  {{row: 0, col: 2}},
	"CLS":    {{row: 0, col: 4}},
	"COPY":   {{row: 0, col: 1}},
	"CAT":    {{row: 7, col: 1, caps: true}, {row: 4, col: 1, shift: true}},
	"FORMAT": {{row: 7, col: 1, caps: true}, {row: 4, col: 0, shift: true}},
	"ERASE":  {{row: 7, col: 1, caps: true}, {row: 4, col: 3, shift: true}},
	"MOVE":   {{row: 7, col: 1, caps: true}, {row: 4, col: 4, shift: true}},
}

// msToFrames converts a millisecond duration to whole 20 ms frames (50 Hz),
// floored at one frame, with a default when the value is not positive.
func msToFrames(ms, def int) int {
	if ms <= 0 {
		ms = def
	}
	f := ms / 20
	if f < 1 {
		f = 1
	}
	return f
}

// typeKey presses (and holds) a matrix key plus its shift modifiers for
// downFrames frames, then releases them for upFrames frames. Holding long
// enough lets the ROM key-scan see the press edge; the release wait lets it see
// the release edge.
func typeKey(e *emulator.Emulator, kp keyPress, downFrames, upFrames int) {
	if kp.caps {
		e.PressKey(0, 0) // CAPS SHIFT
	}
	if kp.shift {
		e.PressKey(7, 1) // SYMBOL SHIFT
	}
	e.PressKey(kp.row, kp.col)
	for i := 0; i < downFrames; i++ {
		e.RunFrame()
	}
	e.ReleaseKey(kp.row, kp.col)
	if kp.shift {
		e.ReleaseKey(7, 1)
	}
	if kp.caps {
		e.ReleaseKey(0, 0)
	}
	for i := 0; i < upFrames; i++ {
		e.RunFrame()
	}
}

// typeText types a string through the ZX keyboard. It understands the tokenized
// input: uppercase words that are known keywords (RUN, LOAD, CAT, ...) are typed
// as their key; other uppercase letters are typed with CAPS SHIFT (uppercase,
// since filenames are case-sensitive); lowercase letters and symbols are typed
// literally. A space immediately after a keyword is skipped (the ROM editor
// inserts it automatically).
func typeText(e *emulator.Emulator, text string, downFrames, upFrames int) {
	for i := 0; i < len(text); {
		ch := text[i]
		switch {
		case ch >= 'A' && ch <= 'Z':
			j := i
			for j < len(text) && text[j] >= 'A' && text[j] <= 'Z' {
				j++
			}
			word := text[i:j]
			if keys, ok := keywordKeys[word]; ok {
				for _, kp := range keys {
					typeKey(e, kp, downFrames, upFrames)
				}
				if j < len(text) && text[j] == ' ' {
					j++ // ROM inserts a space after a keyword
				}
			} else {
				for _, c := range word {
					kp := keyMap[rune(c)]
					kp.caps = true
					typeKey(e, kp, downFrames, upFrames)
				}
			}
			i = j
		case ch >= 'a' && ch <= 'z':
			typeKey(e, keyMap[rune(ch-'a'+'A')], downFrames, upFrames)
			i++
		default:
			if kp, ok := keyMap[rune(ch)]; ok {
				typeKey(e, kp, downFrames, upFrames)
			}
			i++
		}
	}
}
