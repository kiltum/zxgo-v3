package frontend

// EventKind tags which of an Event's fields are meaningful: a tagged union, as Go
// spells it.
//
// The normalised event lives here rather than with the backend contract
// (UI_DESIGN.md section 5.4 puts it in `internal/ui`) because App has to consume
// events, and section 4 puts `internal/ui` *below* this package: a backend may
// import the front end, never the other way round. Everything a backend reports is
// therefore front end data - the same reason `Rect`, `ToolID` and `Screen` are
// here.
type EventKind int

const (
	// EvKey is a host key edge. The key is the front end's own (D8), so a backend
	// translates its platform's scancodes and nothing downstream sees them.
	EvKey EventKind = iota
	// EvAction is an action the user chose without a key: a menubar item, a button.
	EvAction
	// EvQuit is the user asking to leave. A backend reports it for a window close
	// button or a platform quit; only the front end knows what leaving means.
	EvQuit
	// EvCell is a matrix cell edge from something that deals in cells rather than in
	// host keys: the on-screen keyboard, whose keys are the machine's. It is the third
	// key space of D8, and it arrives here rather than being pressed by the backend
	// because every route to the matrix goes through the front end (D9).
	EvCell
	// EvFocusChange says which window has keyboard focus now (D5's routing).
	EvFocusChange
	// EvDialogResult carries the file a native dialog returned, empty if cancelled.
	EvDialogResult
	// EvWindowMoved, EvWindowResized and EvWindowClosed report a window the user
	// changed, so App can persist the new rect (D3).
	EvWindowMoved
	EvWindowResized
	EvWindowClosed
)

func (k EventKind) String() string {
	switch k {
	case EvKey:
		return "key"
	case EvCell:
		return "cell"
	case EvAction:
		return "action"
	case EvQuit:
		return "quit"
	case EvFocusChange:
		return "focus"
	case EvDialogResult:
		return "dialog-result"
	case EvWindowMoved:
		return "window-moved"
	case EvWindowResized:
		return "window-resized"
	case EvWindowClosed:
		return "window-closed"
	}
	return "unknown"
}

// Event is one thing a backend saw, in the front end's own terms.
type Event struct {
	Kind EventKind

	// Tool names the window the event came from. ToolMain is the main window, and
	// it is a window like any other here: focus is what D5 turns on, and the main
	// window is one of the answers.
	Tool ToolID

	// Key, Mods, Down and Captured are EvKey.
	Key  Key
	Mods ModMask
	Down bool
	// Captured reports that a widget in the window this event came from wants the
	// keyboard (D5 rule 3). It travels with the event because only the backend's
	// toolkit knows, and the rule about what to do with it - which three actions
	// stay live through it - is the front end's.
	Captured bool

	// Cell and Down are EvCell.
	Cell Cell

	// Action is EvAction.
	Action Action

	// Path is EvDialogResult: the chosen file, empty when the user cancelled or the
	// dialog failed. Dialog is which dialog it was.
	Path   string
	Dialog DialogKind
	// DialogOpen reports that a native file dialog is up in the window this event came
	// from, which is D5 rule 10's gate. It travels with the key for the same reason
	// Captured does: only the backend knows.
	DialogOpen bool

	// Rect is the window's new geometry, for the window events.
	Rect Rect
}
