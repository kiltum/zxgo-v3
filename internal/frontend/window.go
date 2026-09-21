package frontend

// ToolID names a window. UI_DESIGN.md sections 5.6 and 9.
//
// ToolMain is the main window itself - the screen and the menubar. It appears in
// events and in the layout record like any other window, and it is the one with
// no open flag: closing it means leaving (D4).
type ToolID int

const (
	ToolMain ToolID = iota
	ToolControl
	ToolDisks
	ToolTape
	ToolKeyboard
	ToolBindings
	ToolSettings
	ToolDebugger
)

var toolNames = []string{
	ToolMain:     "main",
	ToolControl:  "control",
	ToolDisks:    "disks",
	ToolTape:     "tape",
	ToolKeyboard: "keyboard",
	ToolBindings: "bindings",
	ToolSettings: "settings",
	ToolDebugger: "debugger",
}

func (t ToolID) String() string {
	if int(t) >= 0 && int(t) < len(toolNames) {
		return toolNames[t]
	}
	return "tool(?)"
}

// ParseToolID reads a tool name as the layout file spells it.
func ParseToolID(name string) (ToolID, bool) {
	for i, n := range toolNames {
		if n == name {
			return ToolID(i), true
		}
	}
	return ToolMain, false
}

// Tools lists the closable windows in a stable order: the order the menubar, the
// layout file and the tests all use, so none of them has to invent one.
func Tools() []ToolID {
	return []ToolID{
		ToolControl, ToolDisks, ToolTape, ToolKeyboard,
		ToolBindings, ToolSettings, ToolDebugger,
	}
}

// Rect is a window's geometry, in the point-based coordinates the window APIs
// report (UI_DESIGN.md section 5.4): never in pixels, so a rect saved on a Retina
// display reopens at the same apparent size on a non-Retina one.
//
// A width or height of zero or less means "not placed": the window has never been
// opened, and the backend chooses where it goes. A negative position is a real
// position - a window dragged half off the left edge has one, and the clamp exists
// to pull it back - so nothing else is encoded in the numbers.
type Rect struct{ X, Y, W, H int }

// Placed reports whether a rect says where the window goes, as opposed to being
// the zero value a window that has never opened carries.
func (r Rect) Placed() bool { return r.W > 0 && r.H > 0 }

// WindowFlags are the per-window flags a tool needs. They are read when the
// window is created: SDL cannot turn NotFocusable on and off without changing
// what the window is, so a change recreates it rather than mutating it
// (UI_DESIGN.md section 5.6).
type WindowFlags uint8

const (
	// NotFocusable takes clicks and never takes keyboard focus, which is what
	// keeps physical keys reaching the machine while the on-screen keyboard is
	// clicked (D4).
	NotFocusable WindowFlags = 1 << iota
	// AlwaysOnTop keeps a window above the others.
	AlwaysOnTop
	// Utility keeps a window out of the task bar and the window list.
	Utility
)

func (f WindowFlags) Has(flag WindowFlags) bool { return f&flag != 0 }

// WindowState is one window: which tool it is, whether it is open, where it is
// and the flags it needs. It is plain data (UI_DESIGN.md section 5.6) - App owns
// the set, the backend makes the OS windows match it, and no view model holds a
// pointer that Reset() replaces.
type WindowState struct {
	ID    ToolID
	Open  bool
	Rect  Rect
	Flags WindowFlags
}

// defaultWindow is where a tool opens the first time, before the user has moved
// it. Sizes are what the contents need, not what a 90s window manager would have
// picked: the virtual keyboard is 40 keys in four rows with up to four legends
// each, so it is wide (D4).
func defaultWindow(id ToolID) WindowState {
	w := WindowState{ID: id}
	switch id {
	case ToolMain:
		w.Rect = Rect{W: 768, H: 576}
	case ToolControl:
		w.Rect = Rect{W: 340, H: 240}
	case ToolDisks:
		w.Rect = Rect{W: 480, H: 440}
	case ToolTape:
		w.Rect = Rect{W: 440, H: 320}
	case ToolKeyboard:
		w.Rect = Rect{W: 1600, H: 380}
		w.Flags = NotFocusable
	case ToolBindings:
		w.Rect = Rect{W: 560, H: 440}
	case ToolSettings:
		// Taller than the rest: the settings window is a list of labelled rows - three radio
		// groups, three groups of switches, each with its own line of explanation - and a
		// window shorter than that is a window with a scrollbar in it on the first launch.
		w.Rect = Rect{W: 620, H: 660}
	case ToolDebugger:
		w.Rect = Rect{W: 760, H: 540}
	}
	return w
}

// DefaultWindows is the window set before any layout has been saved: every tool
// closed, the main window placed and its menubar shown (D4: a first launch shows
// the menubar, since a user who has never seen the menus cannot know ESC brings
// them back).
func DefaultWindows() (main WindowState, tools []WindowState, menuVisible bool) {
	main = defaultWindow(ToolMain)
	for _, id := range Tools() {
		tools = append(tools, defaultWindow(id))
	}
	return main, tools, true
}

// ClampRect fits a rect inside the displays that are attached now, so a window
// saved on a monitor that is no longer there does not open offscreen (D4). It
// shrinks as well as moves: a rect saved on a larger display is too big to fit,
// and moving it would leave most of it outside.
//
// The display it keeps is the one the rect overlaps most, so a window on the
// second monitor comes back to the second monitor when the second monitor is
// still there and to the first when it is not.
func ClampRect(r Rect, displays []Rect) Rect {
	if !r.Placed() || len(displays) == 0 {
		return r
	}
	d := bestDisplay(r, displays)
	if r.W > d.W {
		r.W = d.W
	}
	if r.H > d.H {
		r.H = d.H
	}
	r.X = clampInt(r.X, d.X, d.X+d.W-r.W)
	r.Y = clampInt(r.Y, d.Y, d.Y+d.H-r.H)
	return r
}

// bestDisplay picks the display a rect belongs to: the one it overlaps most, and
// the first one when it overlaps none of them. The starting area is below zero
// so that "overlaps nothing" still chooses the first display rather than none.
func bestDisplay(r Rect, displays []Rect) Rect {
	best, bestArea := displays[0], -1
	for _, d := range displays {
		if area := overlapArea(r, d); area > bestArea {
			best, bestArea = d, area
		}
	}
	return best
}

func overlapArea(a, b Rect) int {
	w := minInt(a.X+a.W, b.X+b.W) - maxInt(a.X, b.X)
	h := minInt(a.Y+a.H, b.Y+b.H) - maxInt(a.Y, b.Y)
	if w <= 0 || h <= 0 {
		return 0
	}
	return w * h
}

func clampInt(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ----------------------------------------------------------------- the set

// PlaceMainDefault sizes the main window to scale times the machine's screen and
// centres it on the first display. A first launch then opens a window whose picture
// fits it exactly, with nothing reserved for anything else: the menubar floats over
// the picture rather than taking room from it, so the scale does not depend on whether
// the bar is shown (D4).
//
// It is a default and not a placement rule: SetLayout overrides it, which is what
// "restored" means, and the first time the user moves the window the layout file
// remembers where it went.
func (a *App) PlaceMainDefault(scale int) {
	if scale < 1 {
		return
	}
	screen := a.Machine.Screen()
	if screen.W <= 0 || screen.H <= 0 {
		return
	}
	a.main.Rect.W = screen.W * scale
	a.main.Rect.H = screen.H * scale

	if len(a.displays) == 0 {
		return
	}
	d := a.displays[0]
	a.main.Rect.X = d.X + (d.W-a.main.Rect.W)/2
	a.main.Rect.Y = d.Y + (d.H-a.main.Rect.H)/2
	a.main.Rect = ClampRect(a.main.Rect, a.displays)
}

// LayoutRestored reports whether a saved layout has been applied. Until it has, the
// window set is a default rather than the user's, which is what lets the loop place
// the main window once it knows how big the screen is.
func (a *App) LayoutRestored() bool { return a.restored }

// Layout is the window set as it is persisted: the main window's rect, the tools
// with their open flags and rects, and whether the menubar is shown (D3, D4). It
// is plain data, so writing and reading it needs no window either.
type Layout struct {
	Main        WindowState
	Tools       []WindowState
	MenuVisible bool
}

// WindowsView is everything a backend needs to make the OS windows match the front
// end: the main window and the tools. One value rather than two arguments, and one
// that can grow.
//
// UI_DESIGN.md section 5.4 sketched `SyncWindows(want []WindowState)`, which could
// not express where the main window goes.
//
// It deliberately does *not* carry which window is focused. It did, and the backend
// turned that into a raise on every frame - which kept the application in front of
// everything else, because raising a window activates it. Which window is in front is
// an action for the backend to take when it is asked (RaiseWindow), not a piece of
// state to be re-derived on each tick.
type WindowsView struct {
	Main  WindowState
	Tools []WindowState
}

// WindowsView returns the set the backend must match.
func (a *App) WindowsView() WindowsView {
	return WindowsView{
		Main:  a.main,
		Tools: a.Windows(),
	}
}

// Equal reports whether two layouts are the same. It is what lets the loop tell
// "the user moved a window" from "nothing happened", so the file is written when it
// changes rather than every iteration (D3).
func (l Layout) Equal(other Layout) bool {
	if l.MenuVisible != other.MenuVisible || l.Main != other.Main {
		return false
	}
	if len(l.Tools) != len(other.Tools) {
		return false
	}
	for i := range l.Tools {
		if l.Tools[i] != other.Tools[i] {
			return false
		}
	}
	return true
}

// Layout returns the record to persist.
func (a *App) Layout() Layout {
	return Layout{
		Main:        a.main,
		Tools:       a.Windows(),
		MenuVisible: a.menuVisible,
	}
}

// SetLayout applies a layout that was read from disk, clamping every rect to the
// displays App knows about. Anything the layout does not mention keeps its
// default, and a layout with no tools leaves the defaults in place rather than
// emptying the set.
//
// Clamping is here rather than in the reader so that there is one rule for both
// paths: a rect saved on a monitor that is no longer attached does not come back
// offscreen (D4), whether it came from a file or from a backend event.
func (a *App) SetLayout(l Layout) {
	a.main = a.resetWindow(ToolMain, l.Main)
	a.menuVisible = l.MenuVisible
	a.restored = true

	for _, want := range l.Tools {
		if !isTool(want.ID) {
			continue // ToolMain has no entry in the tool list
		}
		for i := range a.tools {
			if a.tools[i].ID != want.ID {
				continue
			}
			a.tools[i].Open = want.Open
			a.tools[i].Rect = ClampRect(want.Rect, a.displays)
		}
	}
}

// resetWindow folds a restored entry over a default, keeping the default's flags
// and size when the entry says nothing or says something impossible. A flag
// cannot be trusted from a file: which flags a window needs is a property of the
// tool, not a preference.
func (a *App) resetWindow(id ToolID, want WindowState) WindowState {
	w := defaultWindow(id)
	if want.Rect.Placed() {
		w.Rect = ClampRect(want.Rect, a.displays)
	}
	return w
}

// SetDisplays tells App which displays are attached, which is what a restored
// rect is clamped against (D4). The backend knows; the front end decides.
func (a *App) SetDisplays(displays []Rect) {
	a.displays = append([]Rect(nil), displays...)
	// Clamp what is already placed: a monitor that has just been unplugged is
	// exactly the case the clamp exists for.
	a.main.Rect = ClampRect(a.main.Rect, a.displays)
	for i := range a.tools {
		a.tools[i].Rect = ClampRect(a.tools[i].Rect, a.displays)
	}
}

// Windows returns the tool windows in Tools() order, as a copy: the backend must
// not be able to change the set by holding on to the slice.
func (a *App) Windows() []WindowState {
	out := make([]WindowState, len(a.tools))
	copy(out, a.tools)
	return out
}

// MainWindow returns the main window's state. It has no open flag: it is the one
// window that cannot be closed without leaving.
func (a *App) MainWindow() WindowState { return a.main }

// MenuVisible reports whether the menubar is shown. ESC toggles it while the main
// window has focus (D4).
func (a *App) MenuVisible() bool { return a.menuVisible }

// SetMenuVisible shows or hides the menubar.
func (a *App) SetMenuVisible(on bool) { a.menuVisible = on }

// ToggleMenu is what ESC does in the main window.
func (a *App) ToggleMenu() { a.menuVisible = !a.menuVisible }

// Focused names the window with keyboard focus, which is what input routing turns
// on: matrix keys reach the machine only while ToolMain has it (D5 rule 1).
func (a *App) Focused() ToolID { return a.focused }

// SetFocus records which window has focus.
func (a *App) SetFocus(id ToolID) { a.focused = id }

// OpenTools lists the open tools, which is what the backend presents each tick.
func (a *App) OpenTools() []ToolID {
	var open []ToolID
	for _, w := range a.tools {
		if w.Open {
			open = append(open, w.ID)
		}
	}
	return open
}

// OpenTool opens a tool, or brings it forward when it is already open: the
// menubar item is not a toggle, so clicking it twice does not close the window
// the user is looking at (D4). ToggleTool is the shortcut's behaviour.
//
// "Bring it forward" is a request to the backend rather than a piece of state the
// backend reads every frame, and that distinction is not cosmetic: a backend that
// raised the focused window on every tick could never be put behind another
// program, because raising a window also activates the application. It was, and it
// could not.
func (a *App) OpenTool(id ToolID) {
	if id == ToolMain {
		a.focused = ToolMain
		a.requestRaise(ToolMain)
		return
	}
	if w := a.window(id); w != nil {
		w.Open = true
		a.focused = id
		a.requestRaise(id)
	}
}

// requestRaise asks the backend to bring a window to the front, once.
func (a *App) requestRaise(id ToolID) {
	a.raiseWanted, a.raiseID = true, id
}

// TakeRaiseRequest returns the window to bring forward, once. The loop hands it to
// the backend, which owns the OS's windows.
func (a *App) TakeRaiseRequest() (ToolID, bool) {
	if !a.raiseWanted {
		return ToolMain, false
	}
	a.raiseWanted = false
	return a.raiseID, true
}

// CloseTool closes a tool. The main window cannot be closed this way: closing it
// means leaving, which is ActQuit.
func (a *App) CloseTool(id ToolID) {
	if w := a.window(id); w != nil {
		w.Open = false
	}
	if a.focused == id {
		a.focused = ToolMain
	}
}

// ToggleTool opens a closed tool and closes an open one. It is the shortcut's
// behaviour rather than the menubar's (D4).
func (a *App) ToggleTool(id ToolID) {
	if w := a.window(id); w != nil {
		if w.Open {
			a.CloseTool(id)
			return
		}
		a.OpenTool(id)
	}
}

// SetRect records a window's new geometry, clamped to the displays App knows
// about. The backend reports a user's drag or resize through this and then
// persists the result (D3).
func (a *App) SetRect(id ToolID, r Rect) {
	if w := a.window(id); w != nil {
		w.Rect = ClampRect(r, a.displays)
	}
}

// window finds a window's state, or nil when the id names no window.
func (a *App) window(id ToolID) *WindowState {
	if id == ToolMain {
		return &a.main
	}
	for i := range a.tools {
		if a.tools[i].ID == id {
			return &a.tools[i]
		}
	}
	return nil
}

// isTool reports whether an id names a tool rather than the main window.
func isTool(id ToolID) bool {
	for _, t := range Tools() {
		if t == id {
			return true
		}
	}
	return false
}
