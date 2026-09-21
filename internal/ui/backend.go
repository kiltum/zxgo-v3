// Package ui holds the contract a window backend implements.
//
// There is one interface, `Backend`, and one implementation, internal/ui/imgui. The
// earlier single-window SDL backend that `UI` described is gone: the desktop windows
// replaced it, and what it did is in the git history rather than in the tree. That
// also removed the second copy of the PC-to-ZX keyboard it carried (UI_DESIGN.md
// section 8.2, KNOWN_BUGS.md).
package ui

import "github.com/kiltum/zxgo-v3/internal/frontend"

// Backend is what the loop drives: the OS windows, the drawing, and the input they
// produce. It owns no decisions. Which windows exist, where they go, what a key
// means and what is drawn are all front end data (UI_DESIGN.md section 5.4), which
// is what keeps the placement rules and the keymap testable without a window.
//
// The ImGui backend in internal/ui/imgui implements it.
type Backend interface {
	// Init opens the main window and starts the toolkit. It is separate from the
	// constructor so a failure can be reported before a machine is built around it.
	Init() error

	// Poll pumps every window and appends normalised events. False on quit.
	//
	// A backend renders before it polls next, so an event the drawing produced - a
	// menubar item, a window's close button - is reported here on the following
	// iteration. One frame of latency, and one place events come from.
	Poll(events *[]frontend.Event) bool

	// MainWindowSize is the main window's client area in pixels, which is what the
	// screen is scaled to fill.
	MainWindowSize() (w, h int)

	// Displays returns the usable bounds of every attached display, in the
	// point-based coordinates a saved rect is measured in, so the front end can
	// clamp a restored rect without importing SDL (D4).
	Displays() []frontend.Rect

	// PresentMain draws the main window: the menubar and the machine's screen.
	PresentMain(main frontend.MainView)

	// SyncWindows makes the set of OS windows and their geometry match want: open
	// the missing, hide the extra, move and resize the rest, and raise the one that
	// should be in front. It is idempotent, so it can be called every iteration and
	// a window the user is dragging is not fought over.
	SyncWindows(want frontend.WindowsView)

	// PresentTool draws one tool window's contents. The views travel by value: they
	// are small, and a pointer here bought nothing but the chance of a nil one - which
	// happened, and took a test down with it.
	PresentTool(id frontend.ToolID, views frontend.Views)

	// RaiseWindow brings a window to the front, for the one case that needs it:
	// opening a tool that is already open (D4). It is a request rather than state,
	// because a backend that raised a window on every frame would never let the
	// application be sent behind another one - raising a window also activates it.
	RaiseWindow(id frontend.ToolID)

	// OpenFileDialog asks the user for a file and returns immediately; the answer
	// arrives as an EvDialogResult event on a later Poll, because the platform's
	// dialog is asynchronous and its callback may run on another thread (D2).
	OpenFileDialog(req frontend.DialogRequest)

	// SaveFileDialog asks the user to name a file to write, by the same protocol.
	SaveFileDialog(req frontend.DialogRequest)

	// Destroy closes every window and shuts the toolkit down.
	Destroy()
}
