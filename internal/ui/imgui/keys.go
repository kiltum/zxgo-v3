package imgui

/*
#include <SDL3/SDL.h>
*/
import "C"

import "github.com/kiltum/zxgo-v3/internal/frontend"

// This file is the only place the platform's key numbering meets the front end's,
// which is what D8 asks for: SDL scancodes, ImGui keys and a replay's matrix
// coordinates are three different spaces, and a backend is the only thing allowed
// to know more than one of them.
//
// The table is not the PC-to-ZX keyboard: that is frontend.DefaultMatrixMap, and it
// is reached from here only after the front end's keymap has had its say. This one
// answers a smaller question - which key of ours did the user press.

// scancodeKeys maps SDL's scancodes to front end keys.
//
// Scancodes name physical positions rather than characters, which is what the front
// end's keys mean too, so this is a straight correspondence and not a layout
// decision. Punctuation is included because a keymap file may bind it even though
// the ZX keyboard has no matching key.
var scancodeKeys = map[C.SDL_Scancode]frontend.Key{
	C.SDL_SCANCODE_A: frontend.KeyA,
	C.SDL_SCANCODE_B: frontend.KeyB,
	C.SDL_SCANCODE_C: frontend.KeyC,
	C.SDL_SCANCODE_D: frontend.KeyD,
	C.SDL_SCANCODE_E: frontend.KeyE,
	C.SDL_SCANCODE_F: frontend.KeyF,
	C.SDL_SCANCODE_G: frontend.KeyG,
	C.SDL_SCANCODE_H: frontend.KeyH,
	C.SDL_SCANCODE_I: frontend.KeyI,
	C.SDL_SCANCODE_J: frontend.KeyJ,
	C.SDL_SCANCODE_K: frontend.KeyK,
	C.SDL_SCANCODE_L: frontend.KeyL,
	C.SDL_SCANCODE_M: frontend.KeyM,
	C.SDL_SCANCODE_N: frontend.KeyN,
	C.SDL_SCANCODE_O: frontend.KeyO,
	C.SDL_SCANCODE_P: frontend.KeyP,
	C.SDL_SCANCODE_Q: frontend.KeyQ,
	C.SDL_SCANCODE_R: frontend.KeyR,
	C.SDL_SCANCODE_S: frontend.KeyS,
	C.SDL_SCANCODE_T: frontend.KeyT,
	C.SDL_SCANCODE_U: frontend.KeyU,
	C.SDL_SCANCODE_V: frontend.KeyV,
	C.SDL_SCANCODE_W: frontend.KeyW,
	C.SDL_SCANCODE_X: frontend.KeyX,
	C.SDL_SCANCODE_Y: frontend.KeyY,
	C.SDL_SCANCODE_Z: frontend.KeyZ,

	C.SDL_SCANCODE_0: frontend.KeyZero,
	C.SDL_SCANCODE_1: frontend.KeyOne,
	C.SDL_SCANCODE_2: frontend.KeyTwo,
	C.SDL_SCANCODE_3: frontend.KeyThree,
	C.SDL_SCANCODE_4: frontend.KeyFour,
	C.SDL_SCANCODE_5: frontend.KeyFive,
	C.SDL_SCANCODE_6: frontend.KeySix,
	C.SDL_SCANCODE_7: frontend.KeySeven,
	C.SDL_SCANCODE_8: frontend.KeyEight,
	C.SDL_SCANCODE_9: frontend.KeyNine,

	C.SDL_SCANCODE_RETURN:    frontend.KeyEnter,
	C.SDL_SCANCODE_KP_ENTER:  frontend.KeyEnter,
	C.SDL_SCANCODE_SPACE:     frontend.KeySpace,
	C.SDL_SCANCODE_TAB:       frontend.KeyTab,
	C.SDL_SCANCODE_BACKSPACE: frontend.KeyBackspace,
	C.SDL_SCANCODE_DELETE:    frontend.KeyDelete,
	C.SDL_SCANCODE_INSERT:    frontend.KeyInsert,
	C.SDL_SCANCODE_ESCAPE:    frontend.KeyEscape,

	C.SDL_SCANCODE_UP:       frontend.KeyUp,
	C.SDL_SCANCODE_DOWN:     frontend.KeyDown,
	C.SDL_SCANCODE_LEFT:     frontend.KeyLeft,
	C.SDL_SCANCODE_RIGHT:    frontend.KeyRight,
	C.SDL_SCANCODE_HOME:     frontend.KeyHome,
	C.SDL_SCANCODE_END:      frontend.KeyEnd,
	C.SDL_SCANCODE_PAGEUP:   frontend.KeyPageUp,
	C.SDL_SCANCODE_PAGEDOWN: frontend.KeyPageDown,

	C.SDL_SCANCODE_F1:  frontend.KeyF1,
	C.SDL_SCANCODE_F2:  frontend.KeyF2,
	C.SDL_SCANCODE_F3:  frontend.KeyF3,
	C.SDL_SCANCODE_F4:  frontend.KeyF4,
	C.SDL_SCANCODE_F5:  frontend.KeyF5,
	C.SDL_SCANCODE_F6:  frontend.KeyF6,
	C.SDL_SCANCODE_F7:  frontend.KeyF7,
	C.SDL_SCANCODE_F8:  frontend.KeyF8,
	C.SDL_SCANCODE_F9:  frontend.KeyF9,
	C.SDL_SCANCODE_F10: frontend.KeyF10,
	C.SDL_SCANCODE_F11: frontend.KeyF11,
	C.SDL_SCANCODE_F12: frontend.KeyF12,

	C.SDL_SCANCODE_COMMA:        frontend.KeyComma,
	C.SDL_SCANCODE_PERIOD:       frontend.KeyPeriod,
	C.SDL_SCANCODE_SLASH:        frontend.KeySlash,
	C.SDL_SCANCODE_SEMICOLON:    frontend.KeySemicolon,
	C.SDL_SCANCODE_APOSTROPHE:   frontend.KeyApostrophe,
	C.SDL_SCANCODE_MINUS:        frontend.KeyMinus,
	C.SDL_SCANCODE_EQUALS:       frontend.KeyEqual,
	C.SDL_SCANCODE_LEFTBRACKET:  frontend.KeyLeftBracket,
	C.SDL_SCANCODE_RIGHTBRACKET: frontend.KeyRightBracket,
	C.SDL_SCANCODE_BACKSLASH:    frontend.KeyBackslash,
	C.SDL_SCANCODE_GRAVE:        frontend.KeyGrave,

	// The modifiers are keys as well as a state, and on a ZX Spectrum that
	// distinction is load-bearing: CAPS SHIFT and SYMBOL SHIFT are matrix keys a
	// user presses on their own, and left and right shift are two of them
	// (frontend.DefaultMatrixMap).
	C.SDL_SCANCODE_LSHIFT: frontend.KeyLeftShift,
	C.SDL_SCANCODE_RSHIFT: frontend.KeyRightShift,
	C.SDL_SCANCODE_LCTRL:  frontend.KeyLeftCtrl,
	C.SDL_SCANCODE_RCTRL:  frontend.KeyRightCtrl,
	C.SDL_SCANCODE_LALT:   frontend.KeyLeftAlt,
	C.SDL_SCANCODE_RALT:   frontend.KeyRightAlt,
	C.SDL_SCANCODE_LGUI:   frontend.KeyLeftSuper,
	C.SDL_SCANCODE_RGUI:   frontend.KeyRightSuper,
}

// scancodeToKey translates one scancode, reporting false for a key the front end
// does not know: an unknown key is not an error, it is a key with no meaning here,
// and inventing one would put it in the keymap editor's list.
func scancodeToKey(sc C.SDL_Scancode) (frontend.Key, bool) {
	key, ok := scancodeKeys[sc]
	return key, ok
}

// modsFromSDL translates SDL's modifier state into the front end's mask. Both sides
// of a modifier count as the same one: a binding says "shift", not "left shift",
// and which shift the user holds is the matrix's business, not the keymap's.
func modsFromSDL(mod C.Uint16) frontend.ModMask {
	var m frontend.ModMask
	if mod&(C.SDL_KMOD_LSHIFT|C.SDL_KMOD_RSHIFT) != 0 {
		m |= frontend.ModShift
	}
	if mod&(C.SDL_KMOD_LCTRL|C.SDL_KMOD_RCTRL) != 0 {
		m |= frontend.ModCtrl
	}
	if mod&(C.SDL_KMOD_LALT|C.SDL_KMOD_RALT) != 0 {
		m |= frontend.ModAlt
	}
	if mod&(C.SDL_KMOD_LGUI|C.SDL_KMOD_RGUI) != 0 {
		m |= frontend.ModSuper
	}
	return m
}
