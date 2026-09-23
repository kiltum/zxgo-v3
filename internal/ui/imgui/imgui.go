// Package imgui is zxgo-v3's desktop front end: one OS window per tool, each with
// its own ImGui context (UI_DESIGN.md D1), drawn through SDL3.
//
// It is a backend and nothing else. Which windows exist, where they go, what the
// menubar says, what a key means and when to present are all decisions of
// internal/frontend, which this package reads as plain data and reports back as
// plain events. Nothing here invents an item or decides what a click does.
//
// The vendored Dear ImGui sources are in this directory; VENDOR.md records the
// pinned commit, the file list and the two claims about the backends that were
// checked before any of this was written.
package imgui

/*
#cgo pkg-config: sdl3
#cgo CXXFLAGS: -std=c++11
#cgo linux LDFLAGS: -lstdc++

#include <SDL3/SDL.h>
#include <SDL3/SDL_main.h>
#include <stdlib.h>

// ZxView is opaque to Go: it has C++ members, and its definition lives in
// zxwrap.cpp where cgo never sees it. Only the pointer crosses the boundary.
typedef struct ZxView ZxView;

ZxView* zx_view_new(const char* title, int w, int h,
                    unsigned long long sdl_flags, const char* ini_path, int vsync);
void    zx_view_free(ZxView* v);
unsigned int zx_view_id(ZxView* v);
void    zx_view_geometry(ZxView* v, int* x, int* y, int* w, int* h);
void    zx_view_set_geometry(ZxView* v, int x, int y, int w, int h);
void    zx_view_size(ZxView* v, int* w, int* h);
void    zx_view_pixel_size(ZxView* v, int* w, int* h);
void    zx_view_show(ZxView* v);
void    zx_view_hide(ZxView* v);
void    zx_view_raise(ZxView* v);
int     zx_view_minimized(ZxView* v);
int     zx_view_keyboard_focused(ZxView* v);
int     zx_view_mouse_focused(ZxView* v);
int     zx_view_process_event(ZxView* v, SDL_Event* event);
void    zx_frame_begin(ZxView* v);
void    zx_frame_end(ZxView* v, int r, int g, int b, int a);
int     zx_wants_keyboard(ZxView* v);
int     zx_wants_mouse(ZxView* v);
int     zx_menubar_begin(ZxView* v);
void    zx_menubar_end(ZxView* v);
int     zx_menu_begin(ZxView* v, const char* label);
void    zx_menu_end(ZxView* v);
unsigned int zx_menu_item(ZxView* v, unsigned int id, const char* label,
                          const char* shortcut, int enabled, int checked);
void    zx_menu_separator(ZxView* v);
void    zx_status_text(ZxView* v, const char* text);
int     zx_screen_upload(ZxView* v, const unsigned int* pixels, int w, int h);
void    zx_draw_screen(ZxView* v);
void    zx_main_window_frame(ZxView* v);
void    zx_main_window_frame_end(ZxView* v);
void    zx_open_file_dialog(ZxView* v, const char* kind, const char* default_location);
void    zx_save_file_dialog(ZxView* v, const char* kind, const char* default_location);
int     zx_dialog_pending(ZxView* v);
int     zx_dialog_take(char* out, int size);
int     zx_tool_begin(ZxView* v, const char* title, int* wants_close);
void    zx_tool_end(ZxView* v);
int     zx_read_pixels(ZxView* v, void* dst, int w, int h);
void    zx_text(ZxView* v, const char* text);
void    zx_text_disabled(ZxView* v, const char* text);
void    zx_label_value(ZxView* v, const char* label, const char* value);
int     zx_button_state(ZxView* v, const char* label, int enabled);
int     zx_radio(ZxView* v, const char* label, int selected);
int     zx_checkbox(ZxView* v, const char* label, int checked);
void    zx_key(ZxView* v, const char* label, float w, float h, int down, int* held);
void    zx_same_line(ZxView* v);
void    zx_separator(ZxView* v);
void    zx_spacing(ZxView* v);
void    zx_heading(ZxView* v, const char* text);

// Event accessors. cgo cannot read a C union's members directly, and SDL_Event
// is a union, so the fields the backend needs each get a small accessor here -
// the same arrangement internal/ui/sdl uses.
static inline Uint32 evType(SDL_Event* e)          { return e->type; }
static inline Uint32 evWindowID(SDL_Event* e)      { return e->window.windowID; }
static inline SDL_Scancode evScancode(SDL_Event* e){ return e->key.scancode; }
static inline bool evKeyDown(SDL_Event* e)         { return e->key.down; }
static inline bool evKeyRepeat(SDL_Event* e)       { return e->key.repeat; }
static inline Uint16 evMods(SDL_Event* e)          { return e->key.mod; }
static inline int evMouseX(SDL_Event* e)           { return (int)e->motion.x; }
static inline int evMouseY(SDL_Event* e)           { return (int)e->motion.y; }
// The gamepad device events (added, removed) carry SDL's instance id in "which",
// which is what tells one pad from another.
static inline SDL_JoystickID evWhich(SDL_Event* e) { return e->gdevice.which; }

// Tell SDL we handle main ourselves (Go's main, not SDL_main).
static inline void sdlReady(void) { SDL_SetMainReady(); }
*/
import "C"

import (
	"fmt"
	"log/slog"
	"strings"
	"unsafe"

	"github.com/kiltum/zxgo-v3/internal/frontend"
)

// backendLog is the logger for backend diagnostics. nil means silent.
var backendLog *slog.Logger

// SetLogger stores the logger the backend uses.
func SetLogger(log *slog.Logger) { backendLog = log }

// view is one OS window: its ZxView, the tool it belongs to, and whether it is
// shown.
type view struct {
	ptr  *C.ZxView
	tool frontend.ToolID
	// shown is what the window was last told to be. SyncWindows works from the
	// wanted set, and a window it has already matched is left alone so a drag is
	// not fought over.
	shown bool
	// rect is the geometry the window had when it was last matched, so that a
	// user's own move is noticed rather than overwritten with a stale value.
	rect frontend.Rect
}

// Backend is the ImGui/SDL3 implementation of ui.Backend.
type Backend struct {
	// IniDir is where each context's ini file goes. Empty means "no persistence",
	// which is what a test wants. One file per context: several contexts sharing
	// one path each rewrite it keeping only the entries they know
	// (UI_DESIGN.md section 5.6).
	IniDir string

	// Vsync should be on for the main window and off for the tools: a VSync'd
	// present blocks until that window's next vblank, so several windows would
	// serialize their waits and hand pacing to the display (D7).
	Vsync bool

	main  *view
	tools map[frontend.ToolID]*view

	// pending holds the events the drawing code produced - a menubar item, a
	// window's close button. They are handed over by the next Poll, because Poll is
	// the only way events reach the front end (section 5.4).
	pending []frontend.Event

	focused    frontend.ToolID
	quit       bool
	dialogKind frontend.DialogKind // the kind of the dialog now up
	dialogBuf  []byte              // reused so a Poll does not allocate

	// keyCells is what the on-screen keyboard reported as held last frame, so the two
	// edges of a click can be told apart.
	keyCells map[frontend.Cell]bool

	// pad is the open SDL gamepad driving the Kempston joystick, or nil. joy is the
	// state last reported to the front end, and joyKnown whether one has been reported
	// at all - a report is due whenever the pad's state differs from joy, or when
	// nothing has been said yet (see reportFocus and closeGamepad).
	pad      *gamepad
	joy      frontend.JoyState
	joyKnown bool
}

// New returns a backend that has not opened anything yet: Init does that, so a
// caller can report the failure before building a machine around it.
func New() *Backend {
	return &Backend{
		Vsync:   true,
		tools:   make(map[frontend.ToolID]*view),
		focused: frontend.ToolMain,
	}
}

// Init initialises SDL and opens the main window.
//
// The window is created at the default size the front end supplies through
// SetWindows; until then it opens at a size that fits a 48K screen at 2x, which is
// what the SDL backend has always used.
func (b *Backend) Init() error {
	C.sdlReady()
	// Video and gamepad: the audio device belongs to the emulator, which opens its
	// own (pkg/sound/sdl3). The window backend has no business holding a second one.
	// The gamepad subsystem is initialised here because the window loop is what polls
	// for one; it costs nothing when no pad is connected.
	if !C.SDL_Init(C.SDL_INIT_VIDEO | C.SDL_INIT_GAMEPAD) {
		return fmt.Errorf("imgui: SDL_Init: %s", C.GoString(C.SDL_GetError()))
	}

	main, err := b.openView(frontend.ToolMain, 768, 576, frontend.WindowFlags(0))
	if err != nil {
		C.SDL_Quit()
		return err
	}
	b.main = main
	// The window is created hidden and shown by the first SyncWindows, so it
	// appears in the place the saved layout puts it rather than jumping there.
	b.main.shown = false

	// A gamepad is not required, so a machine with none is a machine that runs
	// exactly as it did. The subsystem's failure is not the emulator's: SDL_Init
	// above already asked for SDL_INIT_GAMEPAD, and this only picks up what is
	// plugged in.
	b.initGamepad()
	return nil
}

// Poll pumps SDL and reports what happened. It returns false when the user asked
// to quit.
func (b *Backend) Poll(events *[]frontend.Event) bool {
	// Anything the last frame's drawing produced goes first, so a menubar click is
	// acted on in the same iteration the front end sees it.
	if len(b.pending) > 0 {
		*events = append(*events, b.pending...)
		b.pending = b.pending[:0]
	}

	// A file dialog's answer, if one came in since the last poll. Drained here rather
	// than from the callback because the callback may run on another thread: it only
	// drops the path in a slot, and this is where it is read (D2).
	if b.dialogBuf == nil {
		b.dialogBuf = make([]byte, 4096)
	}
	if state := C.zx_dialog_take((*C.char)(unsafe.Pointer(&b.dialogBuf[0])), C.int(len(b.dialogBuf))); state != 0 {
		path := ""
		if state == 1 {
			path = C.GoString((*C.char)(unsafe.Pointer(&b.dialogBuf[0])))
		} else if state == 3 && backendLog != nil {
			// An error rather than a cancel: the answer is the same - nothing was
			// chosen - but it is not the user's doing, so it is worth a line.
			backendLog.Error("the file dialog failed", "dialog", b.dialogKind.String())
		}
		*events = append(*events, frontend.Event{
			Kind:   frontend.EvDialogResult,
			Tool:   frontend.ToolMain,
			Path:   path,
			Dialog: b.dialogKind,
		})
	}

	var ev C.SDL_Event
	for C.SDL_PollEvent(&ev) {
		b.handleSDL(&ev, events)
	}

	if b.quit {
		return false
	}
	b.reportFocus(events)
	// After focus, so that a joystick report produced by the focus itself (see
	// reportFocus) is ordered after the focus change it belongs to: the front end
	// gates the joystick on which window has focus, and would drop it otherwise.
	b.pollGamepad(events)
	return true
}

// handleSDL translates one SDL event and gives it to every context.
//
// Broadcasting rather than dispatching by hand is deliberate: ImGui's SDL3 backend
// filters every event by its own window id itself (VENDOR.md), so a context
// ignores what is not its own and no event can be handed to the wrong window by a
// mistake here.
func (b *Backend) handleSDL(ev *C.SDL_Event, events *[]frontend.Event) {
	for _, v := range b.views() {
		C.zx_view_process_event(v.ptr, ev)
	}

	switch C.evType(ev) {
	case C.SDL_EVENT_QUIT:
		b.quit = true

	case C.SDL_EVENT_KEY_DOWN, C.SDL_EVENT_KEY_UP:
		// A repeat is not an edge: the front end's swallow rule and the machine's
		// matrix both work in presses and releases, and a held key repeating would
		// read as a stream of new presses.
		if C.evKeyRepeat(ev) {
			return
		}
		key, ok := scancodeToKey(C.evScancode(ev))
		if !ok {
			return
		}
		*events = append(*events, frontend.Event{
			Kind:       frontend.EvKey,
			Tool:       b.focused,
			Key:        key,
			Mods:       modsFromSDL(C.evMods(ev)),
			Down:       bool(C.evKeyDown(ev)),
			Captured:   b.capturing(),
			DialogOpen: b.dialogPending(),
		})

	case C.SDL_EVENT_GAMEPAD_ADDED:
		// A pad plugged in while the emulator runs drives the joystick from the next
		// read, exactly as one that was there at startup.
		b.openGamepad(uint32(C.evWhich(ev)))

	case C.SDL_EVENT_GAMEPAD_REMOVED:
		// Only the pad that is driving the machine matters; a second one coming and
		// going is not the emulator's business.
		if b.pad != nil && uint32(C.evWhich(ev)) == b.pad.id {
			b.closeGamepad()
			b.pollGamepad(events)
		}

	case C.SDL_EVENT_WINDOW_CLOSE_REQUESTED:
		*events = append(*events, frontend.Event{
			Kind: frontend.EvWindowClosed,
			Tool: b.toolForWindowID(uint32(C.evWindowID(ev))),
		})

	case C.SDL_EVENT_WINDOW_MOVED, C.SDL_EVENT_WINDOW_RESIZED:
		tool := b.toolForWindowID(uint32(C.evWindowID(ev)))
		kind := frontend.EvWindowMoved
		if C.evType(ev) == C.SDL_EVENT_WINDOW_RESIZED {
			kind = frontend.EvWindowResized
		}
		*events = append(*events, frontend.Event{
			Kind: kind,
			Tool: tool,
			Rect: b.viewRect(tool),
		})
	}
}

// capturing reports whether a widget in the window that has the keyboard wants it.
//
// ImGui computes WantCaptureKeyboard during NewFrame, so this is the previous
// frame's answer: one frame of lag on "a text field is focused", which is at most
// 33 ms at 60 Hz and is the only value there is.
func (b *Backend) capturing() bool {
	v := b.tools[b.focused]
	if b.focused == frontend.ToolMain {
		v = b.main
	}
	if v == nil {
		return false
	}
	return C.zx_wants_keyboard(v.ptr) != 0
}

// reportFocus emits an EvFocusChange when the focused window has changed, which is
// what D5's routing turns on. It is read from SDL rather than tracked from events:
// a window manager reports focus in its own way and SDL's answer is the one that
// matches what the user sees.
func (b *Backend) reportFocus(events *[]frontend.Event) {
	focused := frontend.ToolMain
	for _, v := range b.views() {
		if C.zx_view_keyboard_focused(v.ptr) != 0 {
			focused = v.tool
			break
		}
	}
	if focused == b.focused {
		return
	}
	b.focused = focused
	// The joystick is reported again when the main window takes focus back, even
	// though nothing about the pad changed. The front end releases whatever it is
	// holding when focus leaves (D5 rule 4), so a direction held across the switch
	// and still held on the way back would otherwise never be re-sent, and the
	// machine would stand still until the user let go and pushed again.
	if focused == frontend.ToolMain {
		b.joyKnown = false
	}
	*events = append(*events, frontend.Event{Kind: frontend.EvFocusChange, Tool: focused})
}

// OpenFileDialog asks the user for a file. It returns immediately: the answer arrives
// as an EvDialogResult on a later Poll, because SDL's dialog is asynchronous (D2).
//
// The dialog is modal to the main window. It is not modal to the *process* - the
// machine keeps running behind it, which is what a user waiting for a tape to be
// chosen wants, and why D5 rule 10 has to suppress the keyboard rather than the
// dialog blocking it.
func (b *Backend) OpenFileDialog(req frontend.DialogRequest) {
	b.fileDialog(req, false)
}

// SaveFileDialog asks the user to name a file to write, by the same protocol and with
// the same answer: an EvDialogResult carrying the name.
func (b *Backend) SaveFileDialog(req frontend.DialogRequest) {
	b.fileDialog(req, true)
}

func (b *Backend) fileDialog(req frontend.DialogRequest, save bool) {
	if b.main == nil {
		// No dialog can be shown, so answer with nothing rather than staying silent:
		// the answer is what clears the front end's "a dialog is up" state, and that
		// state holds the keyboard back from the machine (D5 rule 10). A silent
		// failure would leave the emulator deaf.
		b.pending = append(b.pending, frontend.Event{
			Kind:   frontend.EvDialogResult,
			Tool:   frontend.ToolMain,
			Dialog: req.Kind,
		})
		return
	}
	b.dialogKind = req.Kind

	kind := C.CString(req.Kind.String())
	location := C.CString(req.Default)
	if save {
		C.zx_save_file_dialog(b.main.ptr, kind, location)
	} else {
		C.zx_open_file_dialog(b.main.ptr, kind, location)
	}
	C.free(unsafe.Pointer(kind))
	C.free(unsafe.Pointer(location))
}

// dialogPending reports whether a dialog is up, which travels on each key event so
// the front end can hold the keyboard back (D5 rule 10).
func (b *Backend) dialogPending() bool {
	if b.main == nil {
		return false
	}
	return C.zx_dialog_pending(b.main.ptr) != 0
}

// MainWindowSize is the main window's client area in pixels, which is what the
// screen is scaled to fill.
func (b *Backend) MainWindowSize() (w, h int) {
	if b.main == nil {
		return 0, 0
	}
	return b.pixelSize(b.main)
}

// Displays reports the usable bounds of every attached display, in the point-based
// coordinates a saved rect is measured in (section 5.4).
func (b *Backend) Displays() []frontend.Rect {
	var count C.int
	ids := C.SDL_GetDisplays(&count)
	if ids == nil || count <= 0 {
		return nil
	}
	// SDL hands back memory it allocated, and SDL_free is what releases it.
	defer C.SDL_free(unsafe.Pointer(ids))

	list := unsafe.Slice(ids, int(count))
	out := make([]frontend.Rect, 0, len(list))
	for _, id := range list {
		var r C.SDL_Rect
		if !C.SDL_GetDisplayUsableBounds(id, &r) {
			continue
		}
		out = append(out, frontend.Rect{
			X: int(r.x), Y: int(r.y), W: int(r.w), H: int(r.h),
		})
	}
	return out
}

// SyncWindows makes the OS windows match what the front end wants: open the
// missing, hide the extra, move and resize the rest, and raise the one that should
// be in front.
//
// Idempotent by construction: a window whose geometry already matches is not
// touched, so a user dragging one is not fought over.
func (b *Backend) SyncWindows(views frontend.WindowsView) {
	if b.main != nil {
		b.match(b.main, views.Main)
	}

	want := make(map[frontend.ToolID]frontend.WindowState, len(views.Tools))
	for _, w := range views.Tools {
		want[w.ID] = w
	}

	// A tool the front end no longer wants open is hidden, not destroyed: the
	// window keeps its position for the next time, which is what makes reopening a
	// tool come back where it was (D4).
	for id, v := range b.tools {
		if w, ok := want[id]; ok && w.Open {
			b.match(v, w)
			continue
		}
		if v.shown {
			C.zx_view_hide(v.ptr)
			v.shown = false
		}
	}

	for _, w := range views.Tools {
		if !w.Open {
			continue
		}
		v := b.tools[w.ID]
		if v == nil {
			var err error
			v, err = b.openView(w.ID, w.Rect.W, w.Rect.H, w.Flags)
			if err != nil {
				if backendLog != nil {
					backendLog.Error("opening a tool window", "tool", w.ID.String(), "error", err)
				}
				continue
			}
			b.tools[w.ID] = v
			// A window that has just been created goes to the front, once, so it is
			// visible without the front end having to ask.
			C.zx_view_raise(v.ptr)
		}
		b.match(v, w)
	}

}

// RaiseWindow brings a window to the front. It is called when the front end asks for
// one - a tool opened that was already open (D4) - and not from SyncWindows, which
// runs every frame.
//
// That distinction is the whole reason this method exists. Raising a window also
// activates its application, so a backend that raised on every tick could never be
// sent behind another program: the emulator came back to the front sixty times a
// second, and the only way to use another application was to quit.
func (b *Backend) RaiseWindow(id frontend.ToolID) {
	if id == frontend.ToolMain {
		if b.main != nil {
			C.zx_view_raise(b.main.ptr)
		}
		return
	}
	if v := b.tools[id]; v != nil && v.shown {
		C.zx_view_raise(v.ptr)
	}
}

// match brings one window in line with what the front end wants. Only the
// differences are applied.
func (b *Backend) match(v *view, want frontend.WindowState) {
	if !want.Open && v.tool != frontend.ToolMain {
		if v.shown {
			C.zx_view_hide(v.ptr)
			v.shown = false
		}
		return
	}
	if !v.shown {
		C.zx_view_show(v.ptr)
		v.shown = true
	}
	if !want.Rect.Placed() {
		return
	}
	// Compare against the geometry the window had when it was last matched rather
	// than against SDL's current answer: after a user's drag the two differ, and
	// that difference is what says "leave it alone".
	have := b.viewRect(v.tool)
	if have == want.Rect {
		return
	}
	C.zx_view_set_geometry(v.ptr, C.int(want.Rect.X), C.int(want.Rect.Y),
		C.int(want.Rect.W), C.int(want.Rect.H))
	v.rect = want.Rect
}

// PresentMain draws the main window: the menubar, and the machine's screen.
func (b *Backend) PresentMain(main frontend.MainView) {
	if b.main == nil || C.zx_view_minimized(b.main.ptr) != 0 {
		return
	}

	C.zx_frame_begin(b.main.ptr)

	// The screen: uploaded every present, because the framebuffer is the machine's
	// and there is no way to know whether it changed.
	s := main.Screen
	if len(s.Pix) > 0 && s.W > 0 && s.H > 0 {
		C.zx_screen_upload(b.main.ptr, (*C.uint)(unsafe.Pointer(&s.Pix[0])), C.int(s.W), C.int(s.H))
	}

	// The picture first, then the menubar over it: both windows are
	// NoBringToFrontOnFocus, so ImGui draws them in this order and the bar lands on
	// top. The bar takes no room from the screen, which is what keeps the picture
	// still when the bar is shown or hidden (D4).
	C.zx_main_window_frame(b.main.ptr)
	C.zx_draw_screen(b.main.ptr)
	C.zx_main_window_frame_end(b.main.ptr)

	if main.MenuVisible {
		b.drawMenu(b.main, main.Menu, main.Status)
	}

	C.zx_frame_end(b.main.ptr, 0, 0, 0, 255)
}

// drawMenu draws the menubar from the front end's model. Every item carries the
// action it stands for, and a click is reported as an EvAction rather than run
// here: what an action does is Dispatch's business (section 5.4).
func (b *Backend) drawMenu(v *view, menu []frontend.Menu, status string) {
	if C.zx_menubar_begin(v.ptr) == 0 {
		return
	}
	for _, m := range menu {
		if len(m.Items) == 0 {
			continue
		}
		label := C.CString(m.Label)
		opened := C.zx_menu_begin(v.ptr, label)
		C.free(unsafe.Pointer(label))
		// Only an open menu has an end: ImGui asserts on the mismatch, and the
		// menubar's own pairing follows the same rule.
		if opened == 0 {
			continue
		}

		for _, item := range m.Items {
			switch item.Kind {
			case frontend.MenuItemSeparator:
				C.zx_menu_separator(v.ptr)
				continue
			}
			label := C.CString(item.Label)
			shortcut := C.CString(item.Shortcut)
			checked := C.int(-1)
			if item.Kind == frontend.MenuItemCheck {
				checked = 0
				if item.Checked {
					checked = 1
				}
			}
			clicked := C.zx_menu_item(v.ptr, C.uint(item.Action+1), label, shortcut,
				boolInt(item.Enabled), checked)
			C.free(unsafe.Pointer(label))
			C.free(unsafe.Pointer(shortcut))
			if clicked != 0 {
				b.pending = append(b.pending, frontend.Event{
					Kind:   frontend.EvAction,
					Action: item.Action,
				})
			}
		}
		C.zx_menu_end(v.ptr)
	}
	if status != "" {
		text := C.CString(status)
		C.zx_status_text(v.ptr, text)
		C.free(unsafe.Pointer(text))
	}
	C.zx_menubar_end(v.ptr)
}

// PresentTool draws one tool window.
//
// Every control in a tool is a button for an action that already exists: the window
// reports the action and the front end decides what it does, exactly as the menubar
// does. That is why adding a window does not add a second way to drive the machine.
func (b *Backend) PresentTool(id frontend.ToolID, views frontend.Views) {
	v := b.tools[id]
	if v == nil || !v.shown || C.zx_view_minimized(v.ptr) != 0 {
		return
	}

	C.zx_frame_begin(v.ptr)

	title := C.CString(frontend.ToolTitle(id))
	var wantsClose C.int
	visible := C.zx_tool_begin(v.ptr, title, &wantsClose)
	C.free(unsafe.Pointer(title))

	if visible != 0 {
		switch id {
		case frontend.ToolControl:
			b.drawControl(v, views)
		case frontend.ToolTape:
			b.drawTape(v, views)
		case frontend.ToolDisks:
			b.drawDisks(v, views)
		case frontend.ToolKeyboard:
			b.drawOnScreenKeyboard(v, views)
		case frontend.ToolSettings:
			b.drawSettings(v, views)
		default:
			// A tool whose contents arrive later says so rather than showing a blank
			// rectangle, so an empty window is distinguishable from one that failed
			// to draw.
			b.text(v, "Nothing here yet.")
		}
	}
	C.zx_tool_end(v.ptr)
	C.zx_frame_end(v.ptr, 32, 32, 32, 255)

	if wantsClose != 0 {
		b.pending = append(b.pending, frontend.Event{
			Kind: frontend.EvWindowClosed,
			Tool: id,
		})
	}
}

// drawControl is the machine control window: what the machine is doing, and the
// buttons that change it.
func (b *Backend) drawControl(v *view, views frontend.Views) {
	s := views.Status

	b.heading(v, "State")
	b.labelValue(v, "Run", s.Run.String())
	b.labelValue(v, "Machine", s.Model)
	b.labelValue(v, "Clock", fmt.Sprintf("%d ticks, %d frames", s.Ticks, s.Frames))
	b.labelValue(v, "Speed", formatFPS(s.FPS))
	if s.FastTape {
		b.labelValue(v, "Tape", "loading at host speed")
	}

	b.heading(v, "Control")
	// Pause is one button whose label follows the state: the action toggles, and a
	// user reading the window should not have to work out which way.
	pause := "Pause"
	if s.Run != frontend.StateRun {
		pause = "Resume"
	}
	if b.buttonAction(v, pause, frontend.ActPauseToggle, true) {
		return
	}
	if b.buttonAction(v, "Step one instruction", frontend.ActStepOne, true) {
		return
	}
	b.sameLine(v)
	if b.buttonAction(v, "Reset", frontend.ActReset, true) {
		return
	}
	if b.buttonAction(v, "NMI", frontend.ActNMI, true) {
		return
	}

	b.heading(v, "Tape speed")
	if b.buttonAction(v, "Fast tape (turbo)", frontend.ActTurboToggle, true) {
		return
	}
	if s.FastTape {
		b.textDisabled(v, "on: the throttle is off while a tape plays")
	} else {
		b.textDisabled(v, "off: a tape loads at tape speed")
	}
}

// drawSettings is the settings window: the machine and the CPU behaviours as radio rows, and
// the switches as checkboxes, all of them lists the front end owns (UI_DESIGN.md section 9).
//
// Every row is drawn from data the front end hands over, and a click reports the action that
// option carries. The window therefore knows nothing about models, sound cards or what applies
// live - and adding a model, a switch or a group is a line in `internal/frontend` and nothing
// here, the same arrangement the on-screen keyboard's rows and the Tools menu already use.
//
// A row that needs a relaunch says so, because a checkbox is otherwise indistinguishable from
// one that has already taken effect (D10).
func (b *Backend) drawSettings(v *view, views frontend.Views) {
	s := views.Settings

	for _, g := range s.Radios {
		b.heading(v, g.Label)
		b.radioRow(v, g.Options)
		if g.Note != "" {
			b.textDisabled(v, g.Note)
		}
	}

	for _, g := range s.Toggles {
		b.heading(v, g.Label)
		for _, row := range g.Rows {
			b.checkbox(v, row.Label, row.Action, row.On)
			if row.RestartRequired() {
				b.sameLine(v)
				b.textDisabled(v, "restart required")
			}
		}
		if g.Note != "" {
			b.textDisabled(v, g.Note)
		}
	}

	if s.Restart {
		b.spacing(v)
		b.text(v, "Restart required for the settings above.")
		if b.buttonAction(v, "Relaunch now", frontend.ActRelaunch, true) {
			return
		}
		b.textDisabled(v, "Leaves and starts again, keeping the same command line and media.")
	}
}

// radioRow draws the options of a mutually exclusive row on one line. They are drawn in a row
// because the labels are short by construction - the front end picks them to fit - and because
// a settings window whose every group is four lines tall is a window nobody scrolls to the
// bottom of.
func (b *Backend) radioRow(v *view, options []frontend.Choice) {
	for i, o := range options {
		if i > 0 {
			b.sameLine(v)
		}
		b.radio(v, o.Label, o.Action, o.Chosen)
	}
}

// drawOnScreenKeyboard is the keyboard window: the machine's forty keys, clickable.
//
// A click is reported as an *edge* of the matrix, not as a host key: the key on screen is
// the machine's key, and the front end presses it through the same counting a physical
// key goes through (D9). Nothing here decides what a key means.
//
// The window is created not-focusable (D4), so clicking a key never takes the keyboard
// away from the machine - which is the whole point of it existing beside the host
// keyboard rather than instead of it.
func (b *Backend) drawOnScreenKeyboard(v *view, views frontend.Views) {
	b.textDisabled(v, "Click a key. This window never takes the keyboard.")
	b.spacing(v)

	// What was reported last frame, so the two edges can be told apart: ImGui says
	// whether a key is held now, and a matrix wants the press and the release.
	if b.keyCells == nil {
		b.keyCells = make(map[frontend.Cell]bool, frontend.ZXKeyboardLen)
	}
	now := make(map[frontend.Cell]bool, len(b.keyCells))

	const keyW, keyH = 74.0, 56.0
	for r, row := range frontend.KeyboardRows() {
		if r > 0 {
			b.spacing(v)
		}
		for i, cell := range row {
			if i > 0 {
				b.sameLine(v)
			}
			key, ok := frontend.ZXKeyAt(cell.Row, cell.Col)
			if !ok {
				continue
			}
			keyDown := views.Keyboard.Down[cell]

			label := C.CString(keyLabel(key))
			var held C.int
			C.zx_key(v.ptr, label, C.float(keyW), C.float(keyH),
				boolInt(keyDown), &held)
			C.free(unsafe.Pointer(label))

			if held != 0 {
				now[cell] = true
			}
		}
	}

	// The edges, in a stable order, so a test can read them and a replay gets a
	// deterministic sequence.
	for _, row := range frontend.KeyboardRows() {
		for _, cell := range row {
			was, is := b.keyCells[cell], now[cell]
			if was == is {
				continue
			}
			b.pending = append(b.pending, frontend.Event{
				Kind: frontend.EvCell,
				Tool: frontend.ToolKeyboard,
				Cell: cell,
				Down: is,
			})
		}
	}
	b.keyCells = now
}

// keyLabel spells a key the way it is printed on the machine: the keyword CAPS SHIFT
// gives above it, the key with its SYMBOL SHIFT legend beside it, and the keywords both
// shifts give below. A key with no legends on a line simply does not have that line -
// which is what makes the labels fit on a button at all.
func keyLabel(k frontend.ZXKey) string {
	lines := []string{}
	if k.Caps != "" {
		lines = append(lines, k.Caps)
	}
	main := k.Key
	if k.Symbol != "" {
		main += "  " + k.Symbol
	}
	lines = append(lines, main)
	if k.Extended != "" {
		lines = append(lines, k.Extended)
	}
	return strings.Join(lines, "\n")
}

// fdcTimingLabel spells the floppy-controller timing switch as its two states, the way
// the turbo button does.
func fdcTimingLabel(noTiming bool) string {
	if noTiming {
		return "Disk timing: off"
	}
	return "Disk timing: faithful"
}

// turboLabel spells the fast-tape switch as the two states it has, so the button
// never leaves the user working out which way round it is.
func turboLabel(on bool) string {
	if on {
		return "Turbo: on"
	}
	return "Turbo: off"
}

// drawTape is the tape window: what is loaded, where the head is, and the transport.
//
// The transport buttons are the same actions the Cmd+P shortcut and the Media menu
// use, so there is one place that knows how to drive the tape (D9).
func (b *Backend) drawTape(v *view, views frontend.Views) {
	t := views.Tape

	// The tape speed is drawn whether or not there is a tape in the drive: it is a
	// property of the machine, it is worth setting before a load starts, and a
	// control that only appears once a tape is in would be unreachable at the moment
	// the user is thinking about it.
	// Fast tape is machine state rather than tape state, so it comes from the status
	// view - the same field the menubar's read-out shows, which is what keeps the
	// button and that read-out from disagreeing.
	fast := views.Status.FastTape
	b.heading(v, "Loading")
	if b.buttonAction(v, "Open tape...", frontend.ActOpenTape, true) {
		return
	}
	b.sameLine(v)
	if b.buttonAction(v, turboLabel(fast), frontend.ActTurboToggle, true) {
		return
	}
	if fast {
		b.textDisabled(v, "the throttle is off while a tape plays")
	} else {
		b.textDisabled(v, "a tape loads at tape speed")
	}

	if t.FileName == "" && len(t.Blocks) == 0 {
		b.heading(v, "Tape")
		b.textDisabled(v, "No tape loaded.")
		b.textDisabled(v, "Use Open tape..., or -tap on the command line.")
		return
	}

	b.heading(v, "Tape")
	b.labelValue(v, "File", t.FileName)
	b.labelValue(v, "Format", t.Format)
	b.labelValue(v, "Blocks", fmt.Sprintf("%d", len(t.Blocks)))
	// Percent is already a percentage: Playback.PercentComplete reports 0 to 100, and
	// the view model keeps those units rather than turning them into a fraction that
	// every display has to remember to multiply back.
	b.labelValue(v, "Position", fmt.Sprintf("%d of %d pulses (%.1f%%)",
		t.Pos, t.Pulses, t.Percent))

	switch {
	case t.Ended:
		b.text(v, "Ended")
	case t.Playing:
		b.text(v, "Playing")
	default:
		b.text(v, "Paused")
	}

	b.heading(v, "Transport")
	play := "Play"
	if t.Playing {
		play = "Pause"
	}
	if b.buttonAction(v, play, frontend.ActTapePlayPause, true) {
		return
	}
	b.sameLine(v)
	if b.buttonAction(v, "Rewind", frontend.ActTapeRewind, true) {
		return
	}

	b.heading(v, "Blocks")
	if t.Current >= 0 && t.Current < len(t.Blocks) {
		cur := t.Blocks[t.Current]
		what := cur.Symbol
		if cur.Name != "" {
			what = cur.Symbol + " " + cur.Name
		}
		b.labelValue(v, "Now", fmt.Sprintf("block %d of %d: %s",
			cur.Index+1, len(t.Blocks), what))
	}
	// The list is capped: a long tape has hundreds of blocks, and a window that
	// scrolls for pages is harder to read than one that says what the head is in and
	// how much is left.
	const show = 12
	for i, block := range t.Blocks {
		if i >= show {
			b.textDisabled(v, fmt.Sprintf("... and %d more", len(t.Blocks)-show))
			break
		}
		label := fmt.Sprintf("%3d  %6d bytes  %s  %s",
			block.Index+1, block.Bytes, block.Symbol, block.Name)
		if block.Current {
			b.text(v, "-> "+label)
		} else {
			b.textDisabled(v, "   "+label)
		}
	}
}

// drawDisks is the disks window: what is in the drive, and what the controller is
// doing with it.
//
// The controller's lines are drawn as a list of the flags that can be up rather than
// as the flag byte, because the byte means different things in the WD1793's two
// layouts and a user reading a window should not have to know which one is current
// (section 6.2).
func (b *Backend) drawDisks(v *view, views frontend.Views) {
	d := views.Disks

	b.heading(v, "Drive")
	if b.buttonAction(v, "Mount disk...", frontend.ActOpenDisk, true) {
		return
	}
	b.sameLine(v)
	if b.buttonAction(v, "Eject", frontend.ActEjectDisk, d.Mounted) {
		return
	}

	// Disk timing, next to the media it affects and for the same reason the tape
	// window carries the tape speed: it changes how a load behaves. Off is the
	// faithful setting; on is for a big image that a user does not mind loading
	// approximately.
	b.heading(v, "Loading")
	if b.buttonAction(v, fdcTimingLabel(d.NoTiming), frontend.ActFDCTimingToggle, true) {
		return
	}
	if d.NoTiming {
		b.textDisabled(v, "on: no seek or rotational latency (fast, not for protected disks)")
	} else {
		b.textDisabled(v, "off: the controller models seek and rotation")
	}

	if !d.Mounted {
		b.textDisabled(v, "No disk in the drive.")
		b.textDisabled(v, "Mount one, or use -disk on the command line.")
	} else {
		name := d.Info.Name
		if name == "" {
			name = "(no TR-DOS name)"
		}
		b.labelValue(v, "File", d.Path)
		b.labelValue(v, "Name", name)
		b.labelValue(v, "Type", string(d.Info.Type))
		b.labelValue(v, "Geometry", fmt.Sprintf("%d tracks, %d sides, %d sectors/track",
			d.Info.Tracks, d.Info.Sides, d.Info.SectorsPerTrack))
		b.labelValue(v, "Size", fmt.Sprintf("%d KB", d.Info.SizeKB))
		// Free sectors and the file count come from the TR-DOS information sector, so
		// they mean nothing for a +3 disk.
		if d.Info.FileCount > 0 || d.Info.FreeSectors > 0 {
			b.labelValue(v, "Files", fmt.Sprintf("%d, %d sectors free",
				d.Info.FileCount, d.Info.FreeSectors))
		}
	}

	b.heading(v, "Controller")
	if !d.HasFDC {
		// A machine with no controller is a normal state rather than a failure: the
		// Beta Disk is created when the first disk is mounted in it.
		b.textDisabled(v, "No floppy controller yet: mount a disk to add one.")
		return
	}

	f := d.FDC
	b.labelValue(v, "Chip", f.Controller)
	b.labelValue(v, "Command", f.Command)
	b.labelValue(v, "Position", fmt.Sprintf("drive %d, track %d, sector %d",
		f.Drive, f.Track, f.Sector))
	// The rotation is shown only while the controller is working. The disk turns
	// whenever a disk is in the drive, so the phase sweeps from 0 to 100% about five
	// times a second forever - which is a read-out that says nothing while nothing is
	// happening, and flickers while the user is trying to read the rest of the window.
	// During a command it is the one thing worth watching.
	if f.Busy && f.IndexPhase >= 0 {
		b.labelValue(v, "Rotation", fmt.Sprintf("%.0f%% of the revolution", f.IndexPhase*100))
	}

	// Only the lines that are up: a list of everything with six of them off is harder
	// to read than one that says what is happening.
	lines := 0
	for _, flag := range []struct {
		on    bool
		label string
	}{
		{f.Busy, "BUSY"},
		{f.DRQ, "DRQ (data ready)"},
		{f.INTRQ, "INTRQ (interrupt)"},
		{f.LostData, "LOST DATA"},
		{f.RecordNotFound, "RECORD NOT FOUND"},
		{f.CRCError, "CRC ERROR"},
		{f.TrackZero, "TRACK 00"},
		{f.WriteProtected, "WRITE PROTECTED"},
		{f.NotReady, "NOT READY"},
	} {
		if !flag.on {
			continue
		}
		b.text(v, flag.label)
		lines++
	}
	if lines == 0 {
		b.textDisabled(v, "idle")
	}
}

// ------------------------------------------------------------ drawing helpers

// buttonAction draws a button and turns a press into an event, which is how every
// control in a tool window reaches the front end. It reports whether it was pressed,
// so a caller can stop drawing the rest of a window whose state is about to change.
func (b *Backend) buttonAction(v *view, label string, action frontend.Action, enabled bool) bool {
	text := C.CString(label)
	pressed := C.zx_button_state(v.ptr, text, boolInt(enabled)) != 0
	C.free(unsafe.Pointer(text))
	if pressed {
		b.pending = append(b.pending, frontend.Event{Kind: frontend.EvAction, Action: action})
	}
	return pressed
}

// radio draws one option of a mutually exclusive row and turns a press into an event, the way
// buttonAction does. Which option is selected is the caller's to state: a radio button's dot is
// not something the front end's data can be asked to agree with.
func (b *Backend) radio(v *view, label string, action frontend.Action, selected bool) bool {
	text := C.CString(label)
	pressed := C.zx_radio(v.ptr, text, boolInt(selected)) != 0
	C.free(unsafe.Pointer(text))
	if pressed {
		b.pending = append(b.pending, frontend.Event{Kind: frontend.EvAction, Action: action})
	}
	return pressed
}

// checkbox draws one switch row and turns a press into an event. The box's state is drawn from
// the value the caller hands over, not from what the toolkit remembers, so a row waiting for a
// relaunch shows the file's value rather than the click that has not been applied yet.
func (b *Backend) checkbox(v *view, label string, action frontend.Action, checked bool) bool {
	text := C.CString(label)
	pressed := C.zx_checkbox(v.ptr, text, boolInt(checked)) != 0
	C.free(unsafe.Pointer(text))
	if pressed {
		b.pending = append(b.pending, frontend.Event{Kind: frontend.EvAction, Action: action})
	}
	return pressed
}

func (b *Backend) text(v *view, text string) {
	s := C.CString(text)
	C.zx_text(v.ptr, s)
	C.free(unsafe.Pointer(s))
}

func (b *Backend) textDisabled(v *view, text string) {
	s := C.CString(text)
	C.zx_text_disabled(v.ptr, s)
	C.free(unsafe.Pointer(s))
}

func (b *Backend) labelValue(v *view, label, value string) {
	l := C.CString(label)
	val := C.CString(value)
	C.zx_label_value(v.ptr, l, val)
	C.free(unsafe.Pointer(l))
	C.free(unsafe.Pointer(val))
}

func (b *Backend) heading(v *view, text string) {
	s := C.CString(text)
	C.zx_heading(v.ptr, s)
	C.free(unsafe.Pointer(s))
}

func (b *Backend) sameLine(v *view) { C.zx_same_line(v.ptr) }

func (b *Backend) spacing(v *view) { C.zx_spacing(v.ptr) }

// formatFPS renders a frame rate the way the menubar does. The backend formats it
// rather than reading a string from the view model: it is presentation, and the view
// model stays plain numbers.
func formatFPS(fps float64) string {
	if fps <= 0 {
		return "not measured yet"
	}
	return fmt.Sprintf("%.1f frames/s", fps)
}

// Destroy closes every window and shuts SDL down.
func (b *Backend) Destroy() {
	b.closeGamepad()
	for _, v := range b.views() {
		C.zx_view_free(v.ptr)
	}
	b.main = nil
	b.tools = make(map[frontend.ToolID]*view)
	C.SDL_Quit()
}

// renderFrame presents a view twice and copies the result out, which is what the
// pixel tests compare. Twice because ImGui settles some window layout on the frame
// after the first: a running emulator presents continuously and never shows that frame
// for longer than it takes to draw the next one.
func (b *Backend) renderFrame(view frontend.MainView, w, h int) []byte {
	b.PresentMain(view)
	b.PresentMain(view)
	frame := make([]byte, w*h*4)
	if !b.readPixels(b.main, frame, w, h) {
		return nil
	}
	return frame
}

// readPixels copies what the renderer last drew into dst as ARGB8888. It is
// unexported and exists for the test that checks a 1x present is a straight copy
// of the framebuffer: cgo cannot be used from a _test.go file, and making this
// public would be API for a test's sake.
func (b *Backend) readPixels(v *view, dst []byte, w, h int) bool {
	if v == nil || len(dst) < w*h*4 {
		return false
	}
	return C.zx_read_pixels(v.ptr, unsafe.Pointer(&dst[0]), C.int(w), C.int(h)) != 0
}

// ------------------------------------------------------------------ internals

// openView creates one window with its own renderer and its own ImGui context.
func (b *Backend) openView(tool frontend.ToolID, w, h int, flags frontend.WindowFlags) (*view, error) {
	if w <= 0 || h <= 0 {
		w, h = 768, 576
	}

	// Hidden until the front end says where it goes: a window that appears at the
	// origin and then jumps to its saved rect is a flash the user sees.
	// Every window is resizable, the main one included: the screen scales to fill it
	// (D6), so a user who wants a bigger picture resizes the window they already
	// have. The flags are the front end's additions, and resizable is not one of them
	// because it is not optional.
	sdlFlags := C.ulonglong(C.SDL_WINDOW_HIDDEN)
	if flags.Has(frontend.NotFocusable) {
		sdlFlags |= C.SDL_WINDOW_NOT_FOCUSABLE
	}
	if flags.Has(frontend.AlwaysOnTop) {
		sdlFlags |= C.SDL_WINDOW_ALWAYS_ON_TOP
	}
	if flags.Has(frontend.Utility) {
		sdlFlags |= C.SDL_WINDOW_UTILITY
	}

	title := C.CString(windowTitle(tool))
	ini := C.CString(b.iniPath(tool))
	vsync := 0
	// VSync on the main window is what paces a paused loop; a tool window with it
	// on would add another vblank wait per frame (D7).
	if b.Vsync && tool == frontend.ToolMain {
		vsync = 1
	}
	ptr := C.zx_view_new(title, C.int(w), C.int(h), sdlFlags, ini, C.int(vsync))
	C.free(unsafe.Pointer(title))
	C.free(unsafe.Pointer(ini))
	if ptr == nil {
		return nil, fmt.Errorf("imgui: opening the %s window: %s",
			tool.String(), C.GoString(C.SDL_GetError()))
	}
	return &view{ptr: ptr, tool: tool}, nil
}

func (b *Backend) iniPath(tool frontend.ToolID) string {
	if b.IniDir == "" {
		return ""
	}
	return b.IniDir + "/ui-" + tool.String() + ".ini"
}

// views lists every view, main first.
func (b *Backend) views() []*view {
	out := make([]*view, 0, len(b.tools)+1)
	if b.main != nil {
		out = append(out, b.main)
	}
	for _, v := range b.tools {
		out = append(out, v)
	}
	return out
}

func (b *Backend) viewRect(tool frontend.ToolID) frontend.Rect {
	var v *view
	if tool == frontend.ToolMain {
		v = b.main
	} else {
		v = b.tools[tool]
	}
	if v == nil {
		return frontend.Rect{}
	}
	var x, y, w, h C.int
	C.zx_view_geometry(v.ptr, &x, &y, &w, &h)
	return frontend.Rect{X: int(x), Y: int(y), W: int(w), H: int(h)}
}

func (b *Backend) pixelSize(v *view) (w, h int) {
	var pw, ph C.int
	C.zx_view_pixel_size(v.ptr, &pw, &ph)
	return int(pw), int(ph)
}

// toolForWindowID finds which tool an SDL window id belongs to, which is how a
// window event is attributed.
func (b *Backend) toolForWindowID(id uint32) frontend.ToolID {
	for _, v := range b.views() {
		if uint32(C.zx_view_id(v.ptr)) == id {
			return v.tool
		}
	}
	return frontend.ToolMain
}

func windowTitle(tool frontend.ToolID) string {
	if tool == frontend.ToolMain {
		return "zxgo-v3"
	}
	return "zxgo-v3 - " + frontend.ToolTitle(tool)
}

func boolInt(b bool) C.int {
	if b {
		return 1
	}
	return 0
}
