package imgui

/*
#include <SDL3/SDL.h>
*/
import "C"

import (
	"unsafe"

	"github.com/kiltum/zxgo-v3/internal/frontend"
)

// The host gamepad, read into the machine's Kempston joystick.
//
// SDL's gamepad API is used rather than its raw joystick API: a gamepad is
// mapped by SDL itself, so a pad reports its D-pad, its stick and its buttons
// whatever the device's own numbering is, and the emulator does not need a table
// per controller. The price is that an unusual device SDL cannot map shows up as
// a joystick and is ignored; a pad SDL recognises is one that just works.
//
// Nothing here decides what a direction means to the machine beyond which axis
// and which buttons produce it - that is the whole of the mapping, and it is
// deliberately small. Anything cleverer (a keyboard cursor key driving the same
// stick, a per-game button layout) belongs in the front end's matrix map, which
// already offers it.

// padAxisThreshold is how far an analog stick must travel before it counts as a
// direction. SDL axes run to +/-32767, so this is about 40% of the travel: far
// enough out that a stick resting at a slightly off-centre zero - which is what
// a worn one does - does not hold a direction down for ever, and near enough that
// a game does not need the stick pushed to its stop.
const padAxisThreshold = 13000

// gamepad is the open SDL gamepad, or nil. One is enough: a Spectrum has one
// joystick port, and the pad the user is holding is the one they mean.
//
// id is SDL's instance id for it, which is how a removal is matched to the pad
// this backend opened rather than to another one the system happens to have.
type gamepad struct {
	ptr *C.SDL_Gamepad
	id  uint32
}

// initGamepad opens a gamepad that was already connected when the emulator
// started. SDL's own ADDED events bring in any that arrives later, so this is
// only about the pad that was plugged in before the window opened.
func (b *Backend) initGamepad() {
	var count C.int
	ids := C.SDL_GetGamepads(&count)
	if ids == nil || count < 1 {
		return
	}
	defer C.SDL_free(unsafe.Pointer(ids))
	// The first one, and only it: SDL lists them in no order a user chose, and a
	// second pad would have nothing to drive that the first is not already driving.
	b.openGamepad(uint32(*ids))
}

// openGamepad opens one gamepad by instance id, replacing whatever was open.
func (b *Backend) openGamepad(id uint32) {
	if b.pad != nil {
		b.closeGamepad()
	}
	pad := C.SDL_OpenGamepad(C.SDL_JoystickID(id))
	if pad == nil {
		if backendLog != nil {
			backendLog.Warn("a gamepad is connected but SDL could not open it",
				"error", C.GoString(C.SDL_GetError()))
		}
		return
	}
	b.pad = &gamepad{ptr: pad, id: id}
	b.joyKnown = false
	if backendLog != nil {
		backendLog.Info("gamepad connected; it now drives the Kempston joystick",
			"name", C.GoString(C.SDL_GetGamepadName(pad)))
	}
}

// closeGamepad closes the open gamepad, if any, and reports the joystick as
// released. The release is the point: a pad unplugged mid-game would otherwise
// leave whatever direction was held down on the machine for ever.
func (b *Backend) closeGamepad() {
	if b.pad == nil {
		return
	}
	if backendLog != nil {
		backendLog.Info("gamepad disconnected")
	}
	C.SDL_CloseGamepad(b.pad.ptr)
	b.pad = nil
	b.joy = frontend.JoyState{}
	b.joyKnown = false
}

// pollGamepad reads the pad and reports the joystick state when it has changed.
//
// It is read once per Poll rather than tracked from SDL's button and axis events:
// the state of a stick is not an event, it is a thing that is true until it
// changes, and asking the pad directly cannot drift from what the pad thinks. The
// resolution costs nothing - Poll runs every loop iteration, so a direction is
// seen within a frame of the user moving the stick.
func (b *Backend) pollGamepad(events *[]frontend.Event) {
	if b.pad == nil {
		return
	}
	j := readJoystick(b.pad.ptr)
	if b.joyKnown && j == b.joy {
		return
	}
	b.joy = j
	b.joyKnown = true
	*events = append(*events, frontend.Event{
		Kind: frontend.EvJoy,
		Tool: b.focused,
		Joy:  j,
	})
}

// readJoystick turns the pad's axes and buttons into the five Kempston
// directions. The stick and the D-pad are both read into the same directions, so
// either drives the machine and the two together are nothing special - a game
// reads bits, not gestures.
func readJoystick(pad *C.SDL_Gamepad) frontend.JoyState {
	lx := int(C.SDL_GetGamepadAxis(pad, C.SDL_GAMEPAD_AXIS_LEFTX))
	ly := int(C.SDL_GetGamepadAxis(pad, C.SDL_GAMEPAD_AXIS_LEFTY))
	return frontend.JoyState{
		Right: lx > padAxisThreshold || buttonDown(pad, C.SDL_GAMEPAD_BUTTON_DPAD_RIGHT),
		Left:  lx < -padAxisThreshold || buttonDown(pad, C.SDL_GAMEPAD_BUTTON_DPAD_LEFT),
		Down:  ly > padAxisThreshold || buttonDown(pad, C.SDL_GAMEPAD_BUTTON_DPAD_DOWN),
		Up:    ly < -padAxisThreshold || buttonDown(pad, C.SDL_GAMEPAD_BUTTON_DPAD_UP),
		Fire:  fireDown(pad),
	}
}

// fireDown reports the fire button. Three of the four face buttons fire, which is
// what a pad with more buttons than the machine has needs: a Spectrum joystick
// has one button and games disagree about which one a pad user will reach for, so
// the bottom (A/Cross, the one a player's thumb rests on) and the two beside it
// (B/Circle, X/Square) all do it. The top face button is left alone, being the
// one a player is most likely to want for something else.
func fireDown(pad *C.SDL_Gamepad) bool {
	return buttonDown(pad, C.SDL_GAMEPAD_BUTTON_SOUTH) ||
		buttonDown(pad, C.SDL_GAMEPAD_BUTTON_EAST) ||
		buttonDown(pad, C.SDL_GAMEPAD_BUTTON_WEST)
}

func buttonDown(pad *C.SDL_Gamepad, b C.SDL_GamepadButton) bool {
	return bool(C.SDL_GetGamepadButton(pad, b))
}
