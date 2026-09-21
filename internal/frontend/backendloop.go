package frontend

import (
	"os"
	"time"
)

// WindowBackend is what the loop below needs from a window backend: the windows,
// their events, the menubar, the tool windows.
//
// It is declared here rather than taking `ui.Backend` directly because section 4
// puts `internal/ui` *below* this package: this package may not import it, so the
// loop declares the port it drives and a backend satisfies it structurally - the
// same arrangement the M0 `Display` port used, and the reason neither direction
// needs an adapter. The two interfaces have to agree; `internal/ui` says so where
// Backend is declared.
type WindowBackend interface {
	Init() error
	Poll(events *[]Event) bool
	MainWindowSize() (w, h int)
	Displays() []Rect
	PresentMain(main MainView)
	SyncWindows(want WindowsView)
	PresentTool(id ToolID, views Views)
	// RaiseWindow brings a window to the front, for the one case that needs it:
	// opening a tool that is already open (D4). It is a request rather than state,
	// because a backend that raised a window every frame would never let the
	// application be sent behind another one.
	RaiseWindow(id ToolID)
	// OpenFileDialog asks the user for a file; the answer comes back as an
	// EvDialogResult event (D2).
	OpenFileDialog(req DialogRequest)
	// SaveFileDialog asks the user to name a file to write, by the same protocol. A
	// second method rather than a flag, because the platform's dialogs are two calls
	// with two contracts: one picks an existing file, the other names a new one.
	SaveFileDialog(req DialogRequest)
	Destroy()
}

// BackendLoop drives an App against a full backend: the section 5.4 loop.
//
// The M0 `Loop` above it drives a `Display` - a screen and two callbacks - and is
// what the plain SDL window still uses. That path is the fallback D1 keeps and
// spends no work on; this one is where the windows live.
type BackendLoop struct {
	App     *App
	Backend WindowBackend

	// Interrupt, when non-nil, is polled once per iteration; a value on it takes
	// the same exit as closing the window, so the caller's deferred saves still run
	// (D11).
	Interrupt <-chan os.Signal

	// LayoutChanged, when non-nil, is called with the new layout whenever the
	// window set changes - a tool opened or closed, a window moved or resized, the
	// menubar hidden. It is how D3's file is written without the loop knowing about
	// files.
	LayoutChanged func(Layout)

	// SettingsChanged does the same for the settings: written when they change, so a
	// window that is merely redrawn does not rewrite the file.
	SettingsChanged func(Settings)

	last         Layout
	lastSettings Settings
	have         bool
}

// Run loops until the app quits or the interrupt channel fires.
func (l *BackendLoop) Run() {
	app, backend := l.App, l.Backend
	if app == nil || backend == nil {
		panic("frontend: BackendLoop needs an App and a Backend")
	}

	// The displays are read once, before anything is placed: the clamp (D4) exists
	// for restoring a rect, and a monitor plugged in later is a next-launch matter
	// rather than something to chase while running.
	app.SetDisplays(backend.Displays())
	if !app.LayoutRestored() {
		// A first launch: now that the displays are known, the main window can be
		// given a size that fits its picture and a place rather than a corner.
		app.PlaceMainDefault(2)
	}
	// Remember where things stand before the loop starts, so the first persist() writes
	// only what actually changes rather than everything the app already had.
	l.last = app.Layout()
	l.lastSettings = app.Settings()
	l.have = true

	for {
		var events []Event
		if !backend.Poll(&events) {
			return
		}
		app.Apply(events)

		// A dialog the app asked for is the backend's to open, and a window it asked to
		// bring forward is the backend's to raise: the front end decides what it wants
		// and the backend owns the OS's windows (section 5.4). A dialog's answer comes
		// back as an event on a later poll.
		if req, ok := app.TakeDialogRequest(); ok {
			if req.Kind.IsSave() {
				backend.SaveFileDialog(req)
			} else {
				backend.OpenFileDialog(req)
			}
		}
		if id, ok := app.TakeRaiseRequest(); ok {
			backend.RaiseWindow(id)
		}

		if app.Quitting() {
			return
		}
		if l.interrupted() {
			app.Notify("Interrupted - saving")
			return
		}

		// The machine advances first, then the windows are made to match, then the
		// picture is drawn: a tool opened by a key this iteration has its window
		// before it has anything to draw in it.
		now := time.Now()
		app.Tick(now)
		backend.SyncWindows(app.WindowsView())

		if app.ShouldPresent(now) {
			backend.PresentMain(app.MainView())
			views := app.Views()
			for _, id := range app.OpenTools() {
				backend.PresentTool(id, views)
			}
		}

		l.persist()
	}
}

// persist hands the layout and the settings to the caller when they have changed,
// which is what D3 means by "written whenever it changes" - as opposed to every
// iteration, which would rewrite the files sixty times a second with the same bytes.
func (l *BackendLoop) persist() {
	now := l.App.Layout()
	if !l.have || !now.Equal(l.last) {
		l.last = now
		if l.LayoutChanged != nil {
			l.LayoutChanged(now)
		}
	}

	settings := l.App.Settings()
	if !l.have || !settings.Equal(l.lastSettings) {
		l.lastSettings = settings
		if l.SettingsChanged != nil {
			l.SettingsChanged(settings)
		}
	}
	l.have = true
}

// interrupted reports whether an interrupt has arrived, without blocking.
func (l *BackendLoop) interrupted() bool {
	if l.Interrupt == nil {
		return false
	}
	select {
	case <-l.Interrupt:
		return true
	default:
		return false
	}
}
