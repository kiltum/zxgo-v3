package frontend

// MenuKind says what an entry is, which is all a backend needs to draw it.
type MenuKind int

const (
	// MenuItemEntry is a plain entry. It is the zero value so that a model built by
	// hand, as a test does, does not have to say so.
	MenuItemEntry MenuKind = iota
	// MenuItemCheck draws a checkmark: tool windows use it for "this tool is open".
	MenuItemCheck
	// MenuItemSeparator draws a line and carries no action.
	MenuItemSeparator
)

// MenuItem is one entry of a menu.
//
// The action is the same value the keymap holds, and the backend reports it back in
// an EvAction event rather than acting on it: the menubar is another way to ask for
// an action, not another place that decides what an action does.
type MenuItem struct {
	Label    string
	Action   Action
	Kind     MenuKind
	Shortcut string
	// Checked is meaningful for MenuItemCheck.
	Checked bool
	// Enabled false draws the entry greyed out. A binding is still reported for a
	// disabled entry, so the front end can say why it is unavailable.
	Enabled bool
}

// Menu is one top-level menu of the menubar.
type Menu struct {
	Label string
	Items []MenuItem
}

// MainView is what the main window draws: the machine's picture, the menubar, and
// the status read-outs that sit in it (D4). Plain data, so the backend can draw it
// and a test can check it without a window.
type MainView struct {
	Screen Screen
	Menu   []Menu
	// Status is the right-aligned menubar read-out: frame rate, run state, the
	// machine and its mounted media.
	Status string
	// MenuVisible is false when the user has hidden the menubar with ESC (D4), in
	// which case the screen has the whole client area.
	MenuVisible bool
}

// Menu returns the menubar as data. It is built from the same registry the keymap
// uses, so an entry and its shortcut cannot disagree, and every tool appears in it
// (D4: the menubar and the shortcuts are two ways to the same window).
func (a *App) Menu() []Menu {
	// The File menu holds the machine's own state, and leaving.
	file := Menu{Label: "File"}
	for _, act := range []Action{ActOpenSnapshot, ActSaveSnapshot, ActRecordReplay} {
		file.Items = append(file.Items, a.actionItem(act))
	}
	file.Items = append(file.Items, MenuItem{Kind: MenuItemSeparator}, a.actionItem(ActQuit))

	var machineItems []MenuItem
	// Fast tape is not here: it is the tape window's control now, next to the
	// transport it affects, and the menubar's status read-out still says when it is on.
	for _, act := range []Action{ActPauseToggle, ActStepOne, ActReset, ActNMI} {
		machineItems = append(machineItems, a.actionItem(act))
	}
	machine := Menu{Label: "Machine", Items: machineItems}

	var mediaItems []MenuItem
	for _, act := range []Action{
		ActOpenTape, ActTapePlayPause, ActTapeRewind,
		ActOpenDisk, ActEjectDisk,
		ActScreenshot,
	} {
		mediaItems = append(mediaItems, a.actionItem(act))
	}
	media := Menu{Label: "Media", Items: mediaItems}

	tools := Menu{Label: "Tools"}
	for i, id := range Tools() {
		if i > 0 {
			tools.Items = append(tools.Items, MenuItem{Kind: MenuItemSeparator})
		}
		tools.Items = append(tools.Items, MenuItem{
			Label:   toolTitle(id),
			Action:  toggleActionFor(id),
			Kind:    MenuItemCheck,
			Checked: a.openIs(id),
			Enabled: true,
		})
	}

	return []Menu{file, machine, media, tools}
}

// actionItem builds one entry: the action, and the shortcut it is bound to, read
// out of the keymap. Deriving the shortcut rather than storing it is what stops the
// menubar from advertising a key the registry does not have.
func (a *App) actionItem(act Action) MenuItem {
	return MenuItem{
		Label:    a.menuLabel(act),
		Action:   act,
		Kind:     MenuItemEntry,
		Enabled:  a.canDispatch(act),
		Shortcut: a.primaryBinding(act),
	}
}

// primaryBinding is the binding a menu shows for an action: the fewest modifiers
// first, then the lowest key, so the answer does not change between calls. A plain
// map iteration would pick a different one each frame and the label would flicker.
func (a *App) primaryBinding(act Action) string {
	best := Binding{}
	found := false
	for binding, bound := range a.keymap {
		if bound != act {
			continue
		}
		if !found || lessBinding(binding, best) {
			best, found = binding, true
		}
	}
	if !found {
		return ""
	}
	return best.String()
}

// lessBinding orders bindings by modifier count and then by key, which is a total
// order over the keymap's keys, so a "best" is unique.
func lessBinding(a, b Binding) bool {
	if countMods(a.Mods) != countMods(b.Mods) {
		return countMods(a.Mods) < countMods(b.Mods)
	}
	return a.Key < b.Key
}

func countMods(m ModMask) int {
	n := 0
	for _, e := range modNames {
		if m&e.mask != 0 {
			n++
		}
	}
	return n
}

// menuLabel is the label an action gets in a menu.
func (a *App) menuLabel(act Action) string {
	if label, ok := menuLabels[act]; ok {
		return label
	}
	return act.String()
}

// canDispatch reports whether an action can run now. It is where a menu entry goes
// grey: at M2 everything in the registry can run.
func (a *App) canDispatch(Action) bool { return true }

// toggleActionFor names the action that toggles a tool's window.
func toggleActionFor(id ToolID) Action {
	switch id {
	case ToolControl:
		return ActToggleControl
	case ToolDisks:
		return ActToggleDisks
	case ToolTape:
		return ActToggleTape
	case ToolKeyboard:
		return ActToggleKeyboard
	case ToolBindings:
		return ActToggleBindings
	case ToolSettings:
		return ActToggleSettings
	case ToolDebugger:
		return ActToggleDebugger
	}
	return ActNone
}

// toolTitle is the title a tool's window and menu entry carry.
func toolTitle(id ToolID) string {
	switch id {
	case ToolMain:
		return "Machine"
	case ToolControl:
		return "Machine control"
	case ToolDisks:
		return "Disks / FDC"
	case ToolTape:
		return "Tape"
	case ToolKeyboard:
		return "Keyboard"
	case ToolBindings:
		return "Key bindings"
	case ToolSettings:
		return "Settings"
	case ToolDebugger:
		return "Debugger"
	}
	return id.String()
}

// ToolTitle is the window title for a tool, which the backend draws and the tests
// check.
func ToolTitle(id ToolID) string { return toolTitle(id) }

// menuLabels are the labels the menubar shows, which are not the action names: a
// menu reads "Pause", a keymap file says "pause-toggle".
var menuLabels = map[Action]string{
	ActQuit:        "Quit",
	ActReset:       "Reset",
	ActNMI:         "NMI",
	ActPauseToggle: "Pause",
	ActStepOne:     "Step one instruction",

	ActOpenTape:      "Open tape...",
	ActOpenDisk:      "Mount disk...",
	ActEjectDisk:     "Eject disk",
	ActTapePlayPause: "Tape play / pause",
	ActTapeRewind:    "Tape rewind",
	ActOpenSnapshot:  "Open snapshot...",
	ActSaveSnapshot:  "Save snapshot as...",
	ActRecordReplay:  "Record replay...",
	ActScreenshot:    "Screenshot",
}

// openIs reports whether a tool is open.
func (a *App) openIs(id ToolID) bool {
	for _, w := range a.tools {
		if w.ID == id {
			return w.Open
		}
	}
	return false
}
