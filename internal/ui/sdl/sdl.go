// Package sdl is the SDL3 UI backend for zxgo-v3.
// It lives in its own package so internal/ui builds without SDL3: the headless
// worker imports internal/ui and uses the null backend instead.

package sdl

/*
#cgo pkg-config: sdl3

#include <SDL3/SDL.h>
#include <SDL3/SDL_main.h>
#include <stdlib.h>

static inline Uint32 evType(SDL_Event *e)    { return e->type; }
static inline SDL_Scancode evScancode(SDL_Event *e) { return e->key.scancode; }
static inline bool evKeyDown(SDL_Event *e)   { return e->key.down; }

// Tell SDL we handle main ourselves (Go's main, not SDL_main).
static inline void sdlReady(void) { SDL_SetMainReady(); }
*/
import "C"
import (
	"fmt"
	"log/slog"
	"unsafe"

	"github.com/kiltum/zxgo-v3/internal/ui"
)

// uiLog is the logger for UI subsystem debug output.
// nil means the caller has not wired a logger (UI operations are silent).
var uiLog *slog.Logger

// SetLogger stores the logger used by the UI subsystem.
func SetLogger(log *slog.Logger) { uiLog = log }

type sdlUI struct {
	window   *C.SDL_Window
	renderer *C.SDL_Renderer
	texture  *C.SDL_Texture
	width    int // framebuffer width in pixels
	height   int // framebuffer height in pixels
}

func New(width, height int) ui.UI {
	return &sdlUI{width: width, height: height}
}

func (u *sdlUI) Init() error {
	// SDL_SetMainReady must be called before any other SDL function
	// to tell SDL we're managing main() ourselves (not SDL_main).
	C.sdlReady()

	if !C.SDL_Init(C.SDL_INIT_VIDEO | C.SDL_INIT_AUDIO) {
		msg := C.GoString(C.SDL_GetError())
		if uiLog != nil {
			uiLog.Error("SDL_Init failed", "error", msg)
		}
		return fmt.Errorf("SDL_Init: %s", msg)
	}

	title := C.CString("zxgo-v3")
	defer C.free(unsafe.Pointer(title))
	win := C.SDL_CreateWindow(
		title,
		C.int(u.width*2), C.int(u.height*2),
		C.SDL_WINDOW_RESIZABLE,
	)
	if win == nil {
		msg := C.GoString(C.SDL_GetError())
		C.SDL_Quit()
		if uiLog != nil {
			uiLog.Error("SDL_CreateWindow failed", "error", msg)
		}
		return fmt.Errorf("SDL_CreateWindow: %s", msg)
	}
	u.window = win

	renderer := C.SDL_CreateRenderer(win, nil)
	if renderer == nil {
		msg := C.GoString(C.SDL_GetError())
		C.SDL_DestroyWindow(win)
		C.SDL_Quit()
		if uiLog != nil {
			uiLog.Error("SDL_CreateRenderer failed", "error", msg)
		}
		return fmt.Errorf("SDL_CreateRenderer: %s", msg)
	}
	u.renderer = renderer

	// Enable VSync -- prevents tearing and caps rendering to display refresh rate.
	C.SDL_SetRenderVSync(renderer, 1)

	C.SDL_SetRenderLogicalPresentation(renderer, C.int(u.width), C.int(u.height), C.SDL_LOGICAL_PRESENTATION_INTEGER_SCALE) // May be LETTERBOX?

	texture := C.SDL_CreateTexture(
		renderer,
		C.SDL_PIXELFORMAT_ARGB8888,
		C.SDL_TEXTUREACCESS_STREAMING,
		C.int(u.width), C.int(u.height),
	)
	if texture == nil {
		msg := C.GoString(C.SDL_GetError())
		C.SDL_DestroyRenderer(renderer)
		C.SDL_DestroyWindow(win)
		C.SDL_Quit()
		if uiLog != nil {
			uiLog.Error("SDL_CreateTexture failed", "error", msg)
		}
		return fmt.Errorf("SDL_CreateTexture: %s", msg)
	}
	u.texture = texture
	C.SDL_SetTextureScaleMode(texture, C.SDL_SCALEMODE_NEAREST)
	return nil
}

// ProcessEvents processes SDL events and returns false when quit requested.
// onTTYKey handles ZX Spectrum keyboard matrix keys.
// onCommand handles emulator commands (tape control, etc) - only Ghost keys!
func (u *sdlUI) ProcessEvents(
	onTTYKey func(row, col int, pressed bool),
	onCommand func(command string, pressed bool),
) bool {
	var ev C.SDL_Event
	for C.SDL_PollEvent(&ev) {
		switch C.evType(&ev) {
		case C.SDL_EVENT_QUIT:
			return false
		case C.SDL_EVENT_KEY_DOWN, C.SDL_EVENT_KEY_UP:
			sc := C.evScancode(&ev)
			down := bool(C.evKeyDown(&ev))

			// Cmd+Q = quit (Ghost key - does not exist on ZX Spectrum)
			if sc == C.SDL_SCANCODE_Q && down {
				mods := C.SDL_GetModState()
				if (mods & C.SDL_KMOD_GUI) != 0 {
					return false
				}
			}

			// Cmd+P = tape playback control (Ghost key - does not exist on ZX Spectrum)
			if sc == C.SDL_SCANCODE_P && down {
				mods := C.SDL_GetModState()
				if (mods & C.SDL_KMOD_GUI) != 0 {
					onCommand("tape-playpause", true)
					continue // Skip ZX key dispatch for command keys
				}
			}

			// Cmd+F1 = print MARK to log (not a ZX key - emulator debug)
			if sc == C.SDL_SCANCODE_F1 && down {
				mods := C.SDL_GetModState()
				if (mods & C.SDL_KMOD_GUI) != 0 {
					if uiLog != nil {
						uiLog.Info("MARK", "type", "debug")
					}
					continue
				}
			}

			dispatchKey(sc, down, onTTYKey)
		}
	}
	return true
}

func (u *sdlUI) RenderFrame(screen []uint32) {
	ptr := unsafe.Pointer(&screen[0])
	C.SDL_UpdateTexture(u.texture, nil, ptr, C.int(u.width*4))
	C.SDL_RenderTexture(u.renderer, u.texture, nil, nil)
	C.SDL_RenderPresent(u.renderer)
}

func (u *sdlUI) SetTitle(title string) {
	cTitle := C.CString(title)
	C.SDL_SetWindowTitle(u.window, cTitle)
	C.free(unsafe.Pointer(cTitle))
}

func (u *sdlUI) Destroy() {
	if u.texture != nil {
		C.SDL_DestroyTexture(u.texture)
	}
	if u.renderer != nil {
		C.SDL_DestroyRenderer(u.renderer)
	}
	if u.window != nil {
		C.SDL_DestroyWindow(u.window)
	}
	C.SDL_Quit()
}

func dispatchKey(sc C.SDL_Scancode, pressed bool, onKey func(row, col int, pressed bool)) {
	// Map SDL scancodes directly (integer values, no string ambiguity).
	// SDL scancode values: LSHIFT=225, RSHIFT=229, LCTRL=224, RCTRL=228
	switch sc {
	// --- CAPS SHIFT (Left Shift) --- row 0, col 0
	case 225: // SDL_SCANCODE_LSHIFT
		onKey(0, 0, pressed)

	// --- SYMBOL SHIFT (Right Shift on Mac -- no RightCtrl) --- row 7, col 1
	case 229: // SDL_SCANCODE_RSHIFT
		onKey(7, 1, pressed)

	// Row 0: Z, X, C, V
	case C.SDL_SCANCODE_Z:
		onKey(0, 1, pressed)
	case C.SDL_SCANCODE_X:
		onKey(0, 2, pressed)
	case C.SDL_SCANCODE_C:
		onKey(0, 3, pressed)
	case C.SDL_SCANCODE_V:
		onKey(0, 4, pressed)
	// Row 1: A, S, D, F, G
	case C.SDL_SCANCODE_A:
		onKey(1, 0, pressed)
	case C.SDL_SCANCODE_S:
		onKey(1, 1, pressed)
	case C.SDL_SCANCODE_D:
		onKey(1, 2, pressed)
	case C.SDL_SCANCODE_F:
		onKey(1, 3, pressed)
	case C.SDL_SCANCODE_G:
		onKey(1, 4, pressed)
	// Row 2: Q, W, E, R, T
	case C.SDL_SCANCODE_Q:
		onKey(2, 0, pressed)
	case C.SDL_SCANCODE_W:
		onKey(2, 1, pressed)
	case C.SDL_SCANCODE_E:
		onKey(2, 2, pressed)
	case C.SDL_SCANCODE_R:
		onKey(2, 3, pressed)
	case C.SDL_SCANCODE_T:
		onKey(2, 4, pressed)
	// Row 3: 1, 2, 3, 4, 5
	case C.SDL_SCANCODE_1:
		onKey(3, 0, pressed)
	case C.SDL_SCANCODE_2:
		onKey(3, 1, pressed)
	case C.SDL_SCANCODE_3:
		onKey(3, 2, pressed)
	case C.SDL_SCANCODE_4:
		onKey(3, 3, pressed)
	case C.SDL_SCANCODE_5:
		onKey(3, 4, pressed)
	// Row 4: 0, 9, 8, 7, 6
	case C.SDL_SCANCODE_0:
		onKey(4, 0, pressed)
	case C.SDL_SCANCODE_9:
		onKey(4, 1, pressed)
	case C.SDL_SCANCODE_8:
		onKey(4, 2, pressed)
	case C.SDL_SCANCODE_7:
		onKey(4, 3, pressed)
	case C.SDL_SCANCODE_6:
		onKey(4, 4, pressed)
	// Row 5: P, O, I, U, Y
	case C.SDL_SCANCODE_P:
		onKey(5, 0, pressed)
	case C.SDL_SCANCODE_O:
		onKey(5, 1, pressed)
	case C.SDL_SCANCODE_I:
		onKey(5, 2, pressed)
	case C.SDL_SCANCODE_U:
		onKey(5, 3, pressed)
	case C.SDL_SCANCODE_Y:
		onKey(5, 4, pressed)
	// Row 6: ENTER, L, K, J, H
	case C.SDL_SCANCODE_RETURN, C.SDL_SCANCODE_KP_ENTER:
		onKey(6, 0, pressed)
	case C.SDL_SCANCODE_L:
		onKey(6, 1, pressed)
	case C.SDL_SCANCODE_K:
		onKey(6, 2, pressed)
	case C.SDL_SCANCODE_J:
		onKey(6, 3, pressed)
	case C.SDL_SCANCODE_H:
		onKey(6, 4, pressed)
	// Row 7: SPACE, M, N, B (SYMBOL SHIFT is Right Shift, handled above)
	case C.SDL_SCANCODE_SPACE:
		onKey(7, 0, pressed)
	case C.SDL_SCANCODE_M:
		onKey(7, 2, pressed)
	case C.SDL_SCANCODE_N:
		onKey(7, 3, pressed)
	case C.SDL_SCANCODE_B:
		onKey(7, 4, pressed)
	}
}
