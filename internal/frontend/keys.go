package frontend

import "strings"

// Key is a host key, in the front end's own numbering.
//
// It is deliberately not an SDL scancode (UI_DESIGN.md D8). SDL scancodes, ImGui
// keys and a replay file's matrix coordinates are three different key spaces, and
// only a backend may produce the first two; adopting one of them here would put a
// platform's numbering inside the keymap file, so a keymap a user edits on Linux
// would mean something else on macOS.
//
// Letters and digits carry their ASCII values, which makes both the file and the
// tests read as "q" and "3". Named keys start at keyNamed and are nowhere near
// the printable range, so a key can never be ambiguous.
type Key uint32

// keyNamed is the first value that is not a printable ASCII key.
const keyNamed Key = 0x100

// The letters and digits are the printable ASCII keys, spelled out so that a
// table reads as a keyboard rather than as a list of runes. A keymap file writes
// them as themselves, which is the point of putting them at their ASCII values.
const (
	KeyA Key = 'a'
	KeyB Key = 'b'
	KeyC Key = 'c'
	KeyD Key = 'd'
	KeyE Key = 'e'
	KeyF Key = 'f'
	KeyG Key = 'g'
	KeyH Key = 'h'
	KeyI Key = 'i'
	KeyJ Key = 'j'
	KeyK Key = 'k'
	KeyL Key = 'l'
	KeyM Key = 'm'
	KeyN Key = 'n'
	KeyO Key = 'o'
	KeyP Key = 'p'
	KeyQ Key = 'q'
	KeyR Key = 'r'
	KeyS Key = 's'
	KeyT Key = 't'
	KeyU Key = 'u'
	KeyV Key = 'v'
	KeyW Key = 'w'
	KeyX Key = 'x'
	KeyY Key = 'y'
	KeyZ Key = 'z'

	KeyZero  Key = '0'
	KeyOne   Key = '1'
	KeyTwo   Key = '2'
	KeyThree Key = '3'
	KeyFour  Key = '4'
	KeyFive  Key = '5'
	KeySix   Key = '6'
	KeySeven Key = '7'
	KeyEight Key = '8'
	KeyNine  Key = '9'
)

const (
	KeyEscape Key = keyNamed + iota
	KeyEnter
	KeySpace
	KeyTab
	KeyBackspace
	KeyDelete
	KeyInsert
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown
	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyF10
	KeyF11
	KeyF12
	KeyComma
	KeyPeriod
	KeySlash
	KeySemicolon
	KeyApostrophe
	KeyMinus
	KeyEqual
	KeyLeftBracket
	KeyRightBracket
	KeyBackslash
	KeyGrave
	KeyLeftShift
	KeyRightShift
	KeyLeftCtrl
	KeyRightCtrl
	KeyLeftAlt
	KeyRightAlt
	KeyLeftSuper
	KeyRightSuper
)

// keyNames spells the named keys, both ways round: a keymap file is written and
// read with these strings, and a test that fails should say "escape" rather than
// 256.
//
// Keys are named after the labels on a US layout, but they mean physical
// positions, which is what SDL scancodes mean too: "q" is the key where a US
// keyboard has its Q, and a user on another layout gets the ZX key their fingers
// already expect. A name can never be read as a position on some other layout,
// so this is the only convention that survives the file being shared.
//
// Modifiers appear here as well as in ModMask, and they are not the same thing:
// the mask is the state held *with* a key, and these are the key itself. On a ZX
// Spectrum that distinction is load-bearing - CAPS SHIFT and SYMBOL SHIFT are
// matrix keys (row 0 column 0, and row 7 column 1) that a user presses on their
// own, and left and right shift are two different ones.
var keyNames = map[Key]string{
	KeyEscape:       "escape",
	KeyEnter:        "enter",
	KeySpace:        "space",
	KeyTab:          "tab",
	KeyBackspace:    "backspace",
	KeyDelete:       "delete",
	KeyInsert:       "insert",
	KeyUp:           "up",
	KeyDown:         "down",
	KeyLeft:         "left",
	KeyRight:        "right",
	KeyHome:         "home",
	KeyEnd:          "end",
	KeyPageUp:       "pageup",
	KeyPageDown:     "pagedown",
	KeyF1:           "f1",
	KeyF2:           "f2",
	KeyF3:           "f3",
	KeyF4:           "f4",
	KeyF5:           "f5",
	KeyF6:           "f6",
	KeyF7:           "f7",
	KeyF8:           "f8",
	KeyF9:           "f9",
	KeyF10:          "f10",
	KeyF11:          "f11",
	KeyF12:          "f12",
	KeyComma:        ",",
	KeyPeriod:       ".",
	KeySlash:        "/",
	KeySemicolon:    ";",
	KeyApostrophe:   "'",
	KeyMinus:        "-",
	KeyEqual:        "=",
	KeyLeftBracket:  "[",
	KeyRightBracket: "]",
	KeyBackslash:    "\\",
	KeyGrave:        "`",
	KeyLeftShift:    "lshift",
	KeyRightShift:   "rshift",
	KeyLeftCtrl:     "lctrl",
	KeyRightCtrl:    "rctrl",
	KeyLeftAlt:      "lalt",
	KeyRightAlt:     "ralt",
	KeyLeftSuper:    "lcmd",
	KeyRightSuper:   "rcmd",
}

// keyByName is keyNames reversed, built once rather than kept in step by hand.
var keyByName = func() map[string]Key {
	m := make(map[string]Key, len(keyNames))
	for k, name := range keyNames {
		m[name] = k
	}
	return m
}()

// KeyNone is the zero Key: no key at all, which is what an unparsed or absent
// binding holds.
const KeyNone Key = 0

// Name spells the key the way a keymap file does. An unknown key spells as "?":
// a keymap that names a key this build does not know is a file to report, not a
// reason to fail.
func (k Key) Name() string {
	if name, ok := keyNames[k]; ok {
		return name
	}
	if k > 0 && k < keyNamed {
		return string(rune(k))
	}
	return "?"
}

// ParseKey reads a key name, case-insensitively, as a keymap file writes it.
// Letters and digits are single characters; everything else is a name from the
// table above ("escape", "f5", ",").
func ParseKey(name string) (Key, bool) {
	s := strings.ToLower(strings.TrimSpace(name))
	if s == "" {
		return KeyNone, false
	}
	if len(s) == 1 {
		r := rune(s[0])
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ' ' {
			if r == ' ' {
				return KeySpace, true
			}
			return Key(r), true
		}
	}
	if k, ok := keyByName[s]; ok {
		return k, true
	}
	return KeyNone, false
}

// ModMask is a set of modifier keys held with a key. UI_DESIGN.md D8: a binding
// is a (key, mods) pair, which is what makes `Cmd+Q` a host action while a plain
// `Q` is the machine's row 2 column 0.
type ModMask uint8

const (
	ModSuper ModMask = 1 << iota // Cmd on macOS, the Windows key elsewhere
	ModCtrl
	ModAlt
	ModShift
)

// modNames spells the modifiers, both ways round, as a keymap file writes them.
// Both the platform names are accepted on the way in and the canonical one is
// written on the way out, so a file is the same on every platform.
var modNames = []struct {
	mask ModMask
	name string
	also []string
}{
	{ModSuper, "cmd", []string{"super", "win", "meta"}},
	{ModCtrl, "ctrl", []string{"control"}},
	{ModAlt, "alt", []string{"opt", "option"}},
	{ModShift, "shift", nil},
}

// Name spells the modifiers in a stable order, so the same binding always writes
// the same string and a keymap file does not churn between saves.
func (m ModMask) Name() string {
	if m == 0 {
		return ""
	}
	var parts []string
	for _, e := range modNames {
		if m&e.mask != 0 {
			parts = append(parts, e.name)
		}
	}
	return strings.Join(parts, "+")
}

// ParseMod reads one modifier name, or reports that it is not one.
func ParseMod(name string) (ModMask, bool) {
	s := strings.ToLower(strings.TrimSpace(name))
	for _, e := range modNames {
		if s == e.name {
			return e.mask, true
		}
		for _, alt := range e.also {
			if s == alt {
				return e.mask, true
			}
		}
	}
	return 0, false
}
