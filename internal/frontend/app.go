package frontend

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/kiltum/zxgo-v3/pkg/replay"
)

// App is the front end's own state and its decisions: what the machine is doing,
// which windows are open and where, what a key means, and what the view models
// hold. It draws nothing and owns no window. The loop pumps the backend and calls
// into App; the backend asks App what to draw (UI_DESIGN.md section 5.4).
//
// Everything here is plain data and pure decision. That is what makes the whole
// front end testable without a window (section 11), and it is why the placement
// rules, the present policy and the keymap live in one file each rather than
// being spread through the drawing code.
type App struct {
	// Machine is the emulator, or a fake in tests.
	Machine Machine

	// ScreenshotDir is where the screenshot action writes. Empty means the
	// working directory.
	ScreenshotDir string

	// PresentInterval is the longest gap between presents while a fast tape load
	// runs. Zero means DefaultPresentInterval.
	PresentInterval time.Duration

	// Notice and Warn are where one-line messages and failures go. Nil means
	// stdout and stderr, which is what the CLI wants.
	Notice func(msg string)
	Warn   func(msg string)

	run       RunState
	keymap    Keymap
	matrix    MatrixMap
	swallowed map[Key]bool // keys whose press a binding ate, so its release is eaten too
	held      map[Cell]int // matrix cells the front end holds, and by how many sources

	capturing  bool // a widget in the focused window wants the keyboard (D5 rule 3)
	dialogOpen bool // a native file dialog is pending (D5 rule 10)

	main        WindowState
	tools       []WindowState
	menuVisible bool
	focused     ToolID
	displays    []Rect

	restored      bool // a saved layout has been applied (see LayoutRestored)
	settings      Settings
	media         MediaPaths
	recorder      *replay.Recorder
	recordingPath string
	raiseWanted   bool // a window to bring forward, once
	raiseID       ToolID
	dialogWanted  bool // a dialog the loop has not been told about yet
	dialogWaiting bool // a dialog the user has asked for and not been answered
	dialogAsked   DialogRequest
	quit          bool
	relaunch      bool   // the user asked for a new process rather than a new machine (ActRelaunch)
	dialogResult  string // the last native dialog's answer, for M4's media windows
	frameReady    bool
	lastPresent   time.Time
	tapeNotified  bool
	fps           fpsMeter
	views         Views
}

// New returns an App driving m: the default keymap and matrix, the default window
// set with the menubar shown and the machine running. Running is the default
// because that is what the CLI does - a front end that opened paused would be a
// surprise the first time someone used it.
func New(m Machine) *App {
	main, tools, menu := DefaultWindows()
	return &App{
		Machine:     m,
		run:         StateRun,
		keymap:      DefaultBindings(),
		matrix:      DefaultMatrixMap(),
		main:        main,
		tools:       tools,
		menuVisible: menu,
		focused:     ToolMain,
	}
}

// DefaultPresentInterval is the longest gap between presents while a fast tape load
// is running. Presenting every emulated frame would hand the loop to the display: it
// blocks in its present when the drawable pool runs dry, so a load would run at the
// refresh rate instead of at host speed. 100 ms keeps the window visibly alive at a
// negligible cost.
const DefaultPresentInterval = 100 * time.Millisecond

// ---------------------------------------------------------------- execution

// RunState reports what the machine is doing.
func (a *App) RunState() RunState { return a.run }

// Tick advances the machine according to the run state, then refreshes the view
// models.
//
// It takes the time rather than reading the clock so that a test can drive it:
// the frame rate in the status view is the one piece of the front end that has
// an opinion about wall-clock time, and a test that had to sleep to check it
// would be a test that fails on a loaded machine.
func (a *App) Tick(now time.Time) {
	a.frameReady = false
	switch a.run {
	case StateRun:
		a.frameReady = a.Machine.RunSlice()
	case StateStep:
		// A step is one instruction and then a pause: the machine is not left
		// running because the user asked for exactly one instruction.
		a.Machine.StepOne()
		a.run = StatePaused
	case StatePaused:
		// Nothing to advance: the loop still presents, and that is what paces it
		// (D7 - the backend blocks in its VSync'd present).
	}
	a.refreshViews(now)
}

// ShouldPresent reports whether the screen should be uploaded this iteration,
// and records the decision's clock. This is D7's present policy as plain data.
func (a *App) ShouldPresent(now time.Time) bool {
	if !a.Machine.FastTapeActive() {
		// Not fast: the clock is cleared so the next fast load presents at once
		// rather than waiting out an interval left over from this one.
		a.lastPresent = time.Time{}
		// A frame boundary is the interesting moment while running. Paused and
		// stepped machines have no boundary and no new frame, but presenting is
		// what stops the loop spinning, so they present too.
		return a.run != StateRun || a.frameReady
	}

	interval := a.PresentInterval
	if interval <= 0 {
		interval = DefaultPresentInterval
	}
	if a.lastPresent.IsZero() || now.Sub(a.lastPresent) >= interval {
		a.lastPresent = now
		return true
	}
	return false
}

// Quitting reports whether a quit has been asked for, which is how the loop
// learns to stop (ActQuit).
func (a *App) Quitting() bool { return a.quit }

// Pause stops the machine. It exists so that a debugger breakpoint (M8) and the
// pause action take the same transition: one place knows how to stop.
func (a *App) Pause() { a.run = StatePaused }

// SetRunState sets the run state directly. Pause and TogglePause are what an
// action uses; this is for a caller that knows exactly which state it wants.
func (a *App) SetRunState(s RunState) { a.run = s }

// TogglePause flips between running and paused. It does not un-pause a step: a
// step is a pause that is already about to happen.
func (a *App) TogglePause() {
	if a.run == StateRun {
		a.Pause()
		return
	}
	a.run = StateRun
}

// Screen is the machine's current picture.
func (a *App) Screen() Screen { return a.Machine.Screen() }

// Views returns the view models as of the last Tick.
func (a *App) Views() Views { return a.views }

// refreshViews rebuilds the views the open windows read, and only those: the status
// view is drawn in the menubar, so it is always refreshed; a tool window's state is
// refreshed while that window is open (section 5.6).
func (a *App) refreshViews(now time.Time) {
	if a.openIs(ToolTape) {
		a.views.Tape, _ = a.Machine.Tape()
	}
	if a.openIs(ToolKeyboard) {
		down := make(map[Cell]bool, ZXKeyboardLen)
		for _, row := range zxKeyboardRows {
			for _, cell := range row {
				if a.Machine.MatrixKey(cell.Row, cell.Col) {
					down[cell] = true
				}
			}
		}
		a.views.Keyboard = KeyboardView{Down: down}
	}
	if a.openIs(ToolDisks) {
		fdc, has := a.Machine.FDC()
		a.views.Disks = DiskView{
			Mounted:  fdc.Ready,
			Info:     fdc.Disk,
			Path:     a.Machine.DiskPath(),
			FDC:      fdc,
			HasFDC:   has,
			NoTiming: a.Machine.NoFDCTiming(),
		}
	}
	if a.openIs(ToolSettings) {
		a.views.Settings = a.SettingsView()
	}

	cfg := a.Machine.ModelConfig()
	a.views.Status = StatusView{
		Run:      a.run,
		Model:    cfg.Name,
		CPUHz:    a.Machine.CPUHz(),
		Ticks:    a.Machine.TotalTicks(),
		Frames:   a.Machine.FrameCount(),
		FPS:      a.fps.observe(a.Machine.FrameCount(), now),
		FastTape: a.Machine.FastTapeActive(),
	}
}

// TapeNotice reports the "tape has run out" message the first time the tape ends
// and not again until it is restarted, so a tape that stops says so once rather
// than on every frame after it.
func (a *App) TapeNotice() (string, bool) {
	if !a.Machine.TapeMounted() {
		return "", false
	}
	if !a.Machine.TapeEnded() {
		// Playing, paused or rewound: the next end is a new event.
		a.tapeNotified = false
		return "", false
	}
	if a.tapeNotified {
		return "", false
	}
	a.tapeNotified = true
	return "Tape ended - playback stopped", true
}

// ------------------------------------------------------------------- input

// Keymap returns the action table.
func (a *App) Keymap() Keymap { return a.keymap }

// SetKeymap replaces the action table, which is what loads a user's keymap file
// (D3) and what the rebinding editor writes back.
func (a *App) SetKeymap(k Keymap) { a.keymap = k }

// Matrix returns the PC-to-ZX keyboard mapping.
func (a *App) Matrix() MatrixMap { return a.matrix }

// SetMatrix replaces the PC-to-ZX mapping, which is what the bindings window's
// bind mode edits (section 9).
func (a *App) SetMatrix(m MatrixMap) { a.matrix = m }

// HandleKey routes one host key edge, and reports the action a binding fired
// (ActNone when none) and whether the front end claimed the edge rather than
// letting it reach the machine's matrix.
//
// A press that matches a binding fires its action and is eaten, and its release
// is eaten with it. The swallow is remembered by key rather than recomputed from
// the modifiers at release time, because the order of events makes the
// recomputation wrong: hold P, press Cmd, release P would decide at release time
// that no chord was involved and hand the machine a key-up for a key it never saw
// go down. UI_DESIGN.md D5 rule 5, and the same rule the SDL backend carries
// today.
//
// Anything that is not a binding is a matrix key (D9). While a widget wants the
// keyboard, or a native file dialog is up, neither reaches the machine - except
// the three actions of D5 rule 3, which stay live because they are the ones a
// user may legitimately want while typing into a window that is not the machine.
func (a *App) HandleKey(tool ToolID, key Key, mods ModMask, down bool) (Action, bool) {
	if !down {
		if a.swallowed[key] {
			delete(a.swallowed, key)
			return ActNone, true
		}
		// A release reaches the machine whenever the front end is holding the
		// cell. It is deliberately not gated on focus or capture: a press that
		// was delivered must be releasable, or the machine keeps the key down
		// after the user has let it go.
		if chord, ok := a.matrix.Lookup(key); ok {
			// A release lets go of the whole chord: one host key stands for the cells
			// together, so letting go of it lets go of all of them, and the holder
			// counting releases only the ones nothing else still holds.
			for _, cell := range chord {
				a.releaseCell(cell)
			}
		}
		return ActNone, false
	}

	if action, ok := a.keymap.Lookup(Binding{Key: key, Mods: mods}); ok {
		// The key belongs to the binding on both edges whether or not the action
		// runs, so the swallow is recorded before the gate below can drop it.
		a.swallow(key)
		if a.keysHeldBack() && heldBackByCapture(action) {
			return ActNone, true
		}
		if err := a.Dispatch(action); err != nil {
			a.NotifyWarn(err.Error())
		}
		return action, true
	}

	if a.keysHeldBack() || tool != ToolMain {
		// Rules 1 and 3: the machine hears the keyboard only while the main window
		// has focus and nothing in front of it wants the keys. The window comes from
		// the event rather than from App's Focused(): the two can differ for the one
		// frame it takes a focus change to arrive, and the event is the one that knows
		// which window the key was typed into.
		return ActNone, false
	}
	if chord, ok := a.matrix.Lookup(key); ok {
		for _, cell := range chord {
			a.pressCell(cell)
		}
	}
	return ActNone, false
}

// MatrixCell presses or releases a ZX key that something has already resolved to
// a matrix cell: the SDL backend's scancode switch today, and the on-screen
// keyboard of M5, which knows which key was clicked rather than which host key it
// stands for.
func (a *App) MatrixCell(row, col int, down bool) {
	cell := Cell{Row: row, Col: col}
	if down {
		a.pressCell(cell)
		return
	}
	a.releaseCell(cell)
}

// ReleaseHeld releases every matrix key the front end is holding, which is what
// happens when the window loses focus: SDL does not deliver the releases of keys
// that were held when focus went, and a key left down across a focus change is a
// key the machine cannot clear (D5 rule 4).
func (a *App) ReleaseHeld() {
	for cell := range a.held {
		a.Machine.ReleaseKey(cell.Row, cell.Col)
	}
	a.held = nil
}

// SetCapturing records whether a widget in the focused window wants the keyboard
// (D5 rule 3). The backend asks its own context and reports the answer.
func (a *App) SetCapturing(on bool) { a.capturing = on }

// SetDialogOpen records whether a native file dialog is pending (D5 rule 10).
// Nothing in ImGui reports that: a modal OS window is not a widget.
func (a *App) SetDialogOpen(on bool) { a.dialogOpen = on }

// keysHeldBack reports whether the keyboard currently belongs to the front end
// rather than to the machine.
func (a *App) keysHeldBack() bool { return a.capturing || a.dialogOpen }

// heldBackByCapture reports whether an action waits while a widget has the
// keyboard or a dialog is up. Quit, pause and screenshot do not: D5 rule 3 names
// them as the exceptions, and they are the three a user may reasonably want while
// typing into something that is not the machine.
func heldBackByCapture(act Action) bool {
	switch act {
	case ActQuit, ActPauseToggle, ActScreenshot:
		return false
	}
	return true
}

// pressCell delivers a press and records that the front end holds the cell.
//
// The count is what keeps two sources of the same key honest: a physical key and
// an on-screen click on the same cell are one ZX key, and the release of either
// must not release a cell the other still holds. It is also what makes a
// suppressed press safe - a press that no rule let through leaves no count, so
// its release is not delivered either, and the recorder never sees a key-up for a
// key that was never pressed (D5 rule 4).
func (a *App) pressCell(cell Cell) {
	if a.held == nil {
		a.held = make(map[Cell]int)
	}
	if a.held[cell] > 0 {
		a.held[cell]++
		return
	}
	a.held[cell] = 1
	a.Machine.PressKey(cell.Row, cell.Col)
}

// releaseCell releases a cell the front end is holding, and does nothing for one
// it is not: a release without a press is not a release.
func (a *App) releaseCell(cell Cell) {
	switch n := a.held[cell]; {
	case n == 0:
		return
	case n > 1:
		a.held[cell] = n - 1
	default:
		delete(a.held, cell)
		a.Machine.ReleaseKey(cell.Row, cell.Col)
	}
}

func (a *App) swallow(key Key) {
	if a.swallowed == nil {
		a.swallowed = make(map[Key]bool)
	}
	a.swallowed[key] = true
}

// Dispatch runs one action. It is the single place an action becomes an effect,
// so a shortcut, a menubar item and a tool window's button all take one path -
// which is the reason the registry exists.
func (a *App) Dispatch(act Action) error {
	switch act {
	case ActNone:
		return nil

	case ActQuit:
		a.quit = true
		return nil
	case ActReset:
		a.Machine.Reset()
		a.fps.reset()
		return nil
	case ActNMI:
		// The machine takes it at its next instruction boundary, so a paused
		// machine takes it when it next runs. Nothing here waits for that.
		a.Machine.NMI()
		return nil
	case ActPauseToggle:
		a.TogglePause()
		return nil
	case ActStepOne:
		a.run = StateStep
		return nil
	// Fast tape and the floppy controller's timing are not here: they are two of the settings
	// window's switch rows, and the table that builds those rows is what flips them, records
	// them and applies them to the machine (ActTurboToggle, ActFDCTimingToggle). The tape and
	// disks windows' buttons are the same actions, so both windows are one setting.

	case ActOpenTape:
		// Not a load: a request for the user to choose a file. The load happens when
		// the dialog answers, which may be many frames later, or never.
		a.requestDialog(DialogOpenTape)
		return nil
	case ActOpenDisk:
		a.requestDialog(DialogOpenDisk)
		return nil
	case ActOpenSnapshot:
		a.requestDialog(DialogOpenSnapshot)
		return nil
	case ActRecordReplay:
		a.requestDialog(DialogSaveReplay)
		return nil
	case ActSaveSnapshot:
		a.requestDialog(DialogSaveSnapshot)
		return nil
	case ActEjectDisk:
		a.Machine.EjectDisk()
		return nil
	case ActTapePlayPause:
		return a.tapePlayPause()
	case ActTapeRewind:
		a.Machine.RewindTape()
		return nil
	case ActScreenshot:
		return a.screenshot()
	case ActLogMark:
		a.mark()
		return nil

	case ActToggleControl:
		a.ToggleTool(ToolControl)
		return nil
	case ActToggleDisks:
		a.ToggleTool(ToolDisks)
		return nil
	case ActToggleTape:
		a.ToggleTool(ToolTape)
		return nil
	case ActToggleKeyboard:
		a.ToggleTool(ToolKeyboard)
		return nil
	case ActToggleBindings:
		a.ToggleTool(ToolBindings)
		return nil
	case ActToggleSettings:
		a.ToggleTool(ToolSettings)
		return nil
	case ActToggleDebugger:
		a.ToggleTool(ToolDebugger)
		return nil

	// Settings that apply at once. The rest of the settings window's rows are the choices
	// below it, which are recorded and take effect when the machine is next built.
	case ActZ80NMOS:
		a.SetZ80(true)
		return nil
	case ActZ80CMOS:
		a.SetZ80(false)
		return nil
	case ActMEMPTRReal:
		a.SetMEMPTR(true)
		return nil
	case ActMEMPTRDocumented:
		a.SetMEMPTR(false)
		return nil
	case ActSessionToggle:
		a.settings.SetSession(!a.settings.SessionEnabled())
		return nil
	case ActRelaunch:
		return a.relaunchNow()
	}

	// The settings window's rows are actions like any other control's: the window reports
	// which choice was clicked and the front end decides what that means (section 9). The
	// two lookups are here rather than in the switch above because their tables are the
	// settings' own - which models exist, and which switch a row flips. The switch table is
	// also where the tape and disks windows' own buttons arrive: they are the same settings.
	if key, ok := modelKeyForAction(act); ok {
		a.settings.Model = key
		return nil
	}
	if a.setSwitch(act) {
		return nil
	}

	// Not reachable from the default keymap: every action above is handled, and
	// an action a later stage adds is declared with its handler.
	return fmt.Errorf("frontend: no handler for action %v", act)
}

// tapePlayPause drives the tape through the emulator's transport methods, which
// is what makes Cmd+P visible to a recording (D9). A machine with no tape does
// nothing and says nothing: the command is not an error, it simply has nothing
// to act on.
func (a *App) tapePlayPause() error {
	playing, ok := ToggleTapePlayback(a.Machine)
	if !ok {
		return nil
	}
	if playing {
		a.Notify("Tape playing (Cmd+P)")
	} else {
		a.Notify("Tape paused (Cmd+P)")
	}
	return nil
}

func (a *App) screenshot() error {
	dir := a.ScreenshotDir
	if dir == "" {
		dir = "."
	}
	s := a.Machine.Screen()
	path, err := SavePNG(dir, s.Pix, s.W, s.H)
	if err != nil {
		return err
	}
	a.Notify("Screenshot: " + path)
	return nil
}

// mark writes a marker into the log, which is how a user points at the moment
// something went wrong in a log they are about to send.
func (a *App) mark() {
	if appLog == nil {
		return
	}
	appLog.Info("MARK", "type", "debug")
}

// ------------------------------------------------------------------ output

// Notify reports a one-line message through Notice, or stdout when no sink is
// wired. The loop sends its own messages this way too, so a caller configures one
// output rather than two.
func (a *App) Notify(msg string) { a.notice(msg) }

// NotifyWarn reports a failure through Warn, or stderr.
func (a *App) NotifyWarn(msg string) { a.warn(msg) }

func (a *App) notice(msg string) {
	if a.Notice != nil {
		a.Notice(msg)
		return
	}
	fmt.Println(msg)
}

func (a *App) warn(msg string) {
	if a.Warn != nil {
		a.Warn(msg)
		return
	}
	fmt.Fprintln(os.Stderr, msg)
}

// appLog is the logger the front end writes to. Nil means the caller has not
// wired one and its output is dropped, the same arrangement the other subsystems
// use.
var appLog *slog.Logger

// SetLogger stores the logger the front end uses.
func SetLogger(log *slog.Logger) { appLog = log }
