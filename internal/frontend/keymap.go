package frontend

import (
	"fmt"
	"strings"
)

// Action is something the user can ask the front end to do.
//
// The zero value is ActNone, which is how a keymap says "nothing": a binding
// mapped to ActNone is an explicit unbinding, so a user can take a default away
// without editing the file's internals.
//
// UI_DESIGN.md section 5.3 lists the eventual set. This is the list a stage
// implements: the actions that only a stage can carry - opening a disk, a
// snapshot, a replay, the NMI, the debugger's log level - arrive with that stage
// rather than being declared here to no purpose.
type Action int

const (
	ActNone Action = iota

	// Execution.
	ActQuit
	ActReset
	ActNMI
	ActPauseToggle
	ActStepOne
	ActTurboToggle

	// Media and output.
	ActOpenTape
	ActOpenDisk
	ActEjectDisk
	ActFDCTimingToggle
	ActTapePlayPause
	ActTapeRewind
	ActOpenSnapshot
	ActSaveSnapshot
	ActRecordReplay
	ActScreenshot
	ActLogMark

	// Tools: one action per window rather than one parameterised action, so a
	// binding stays a (key, mods) pair with no argument to carry.
	ActToggleControl
	ActToggleDisks
	ActToggleTape
	ActToggleKeyboard
	ActToggleBindings
	ActToggleSettings
	ActToggleDebugger

	// Settings: one action per choice for the same reason, and the settings window is
	// the one that draws them (UI_DESIGN.md section 9). Model and the sound devices are
	// restart-required; the CPU behaviour rows, the two speed switches and the session apply
	// at once (D10).
	ActModel48K
	ActModel128K
	ActModel2A3
	ActModelPentagon
	ActModelPentagon512
	ActZ80NMOS
	ActZ80CMOS
	ActMEMPTRReal
	ActMEMPTRDocumented
	ActTurboSoundToggle
	ActTurboSoundFMToggle
	ActGeneralSoundToggle
	ActSnowToggle
	ActSessionToggle
	ActRelaunch
)

// actionNames spells the actions, both ways round: a keymap file is written and
// read with these strings.
var actionNames = map[Action]string{
	ActNone: "none",

	ActQuit:        "quit",
	ActReset:       "reset",
	ActNMI:         "nmi",
	ActPauseToggle: "pause-toggle",
	ActStepOne:     "step-one",
	ActTurboToggle: "turbo-toggle",

	ActOpenTape:        "open-tape",
	ActOpenDisk:        "open-disk",
	ActEjectDisk:       "eject-disk",
	ActFDCTimingToggle: "fdc-timing-toggle",
	ActTapePlayPause:   "tape-playpause",
	ActTapeRewind:      "tape-rewind",
	ActOpenSnapshot:    "open-snapshot",
	ActSaveSnapshot:    "save-snapshot",
	ActRecordReplay:    "record-replay",
	ActScreenshot:      "screenshot",
	ActLogMark:         "log-mark",

	ActToggleControl:  "toggle-control",
	ActToggleDisks:    "toggle-disks",
	ActToggleTape:     "toggle-tape",
	ActToggleKeyboard: "toggle-keyboard",
	ActToggleBindings: "toggle-bindings",
	ActToggleSettings: "toggle-settings",
	ActToggleDebugger: "toggle-debugger",

	ActModel48K:           "model-48k",
	ActModel128K:          "model-128k",
	ActModel2A3:           "model-2a3",
	ActModelPentagon:      "model-pentagon",
	ActModelPentagon512:   "model-pentagon512",
	ActZ80NMOS:            "z80-nmos",
	ActZ80CMOS:            "z80-cmos",
	ActMEMPTRReal:         "memptr-real",
	ActMEMPTRDocumented:   "memptr-documented",
	ActTurboSoundToggle:   "turbosound-toggle",
	ActTurboSoundFMToggle: "turbosoundfm-toggle",
	ActGeneralSoundToggle: "gs-toggle",
	ActSnowToggle:         "snow-toggle",
	ActSessionToggle:      "session-toggle",
	ActRelaunch:           "relaunch",
}

func (a Action) String() string {
	if name, ok := actionNames[a]; ok {
		return name
	}
	return fmt.Sprintf("action(%d)", int(a))
}

// ParseAction reads an action name as a keymap file writes it.
func ParseAction(name string) (Action, bool) {
	s := strings.ToLower(strings.TrimSpace(name))
	for a, n := range actionNames {
		if s == n {
			return a, true
		}
	}
	return ActNone, false
}

// Binding is a key and the modifiers held with it. UI_DESIGN.md D8: the key is
// the front end's own Key, not a platform scancode, so a binding means the same
// thing everywhere.
type Binding struct {
	Key  Key
	Mods ModMask
}

// String spells the binding the way a keymap file writes it: "cmd+shift+q",
// "f5", "cmd+,".
func (b Binding) String() string {
	if mods := b.Mods.Name(); mods != "" {
		return mods + "+" + b.Key.Name()
	}
	return b.Key.Name()
}

// ParseBinding reads a binding such as "cmd+shift+q". A bare key name is a
// binding with no modifiers; an empty string, or one with a part this build does
// not know, is an error rather than a silent half-binding.
func ParseBinding(text string) (Binding, error) {
	parts := strings.Split(strings.TrimSpace(text), "+")
	if len(parts) == 0 || strings.TrimSpace(parts[len(parts)-1]) == "" {
		return Binding{}, fmt.Errorf("frontend: %q names no key", text)
	}

	// The key is the last part: a modifier name can never be the key, and "cmd+"
	// plus a key is how every binding is written.
	key, ok := ParseKey(parts[len(parts)-1])
	if !ok {
		return Binding{}, fmt.Errorf("frontend: %q is not a key name", parts[len(parts)-1])
	}

	var mods ModMask
	for _, part := range parts[:len(parts)-1] {
		m, ok := ParseMod(part)
		if !ok {
			return Binding{}, fmt.Errorf("frontend: %q is not a modifier", part)
		}
		mods |= m
	}
	return Binding{Key: key, Mods: mods}, nil
}

// Keymap maps bindings to actions. It is the table behind both the shortcut
// dispatch and the rebinding editor (UI_DESIGN.md section 5.3).
type Keymap map[Binding]Action

// Lookup reports the action a binding is bound to, and whether it is bound at
// all. A binding mapped to ActNone is reported as unbound: that is what an
// unbinding means, and a caller that had to distinguish the two would be the
// caller making a mistake.
func (k Keymap) Lookup(b Binding) (Action, bool) {
	action, ok := k[b]
	if !ok || action == ActNone {
		return ActNone, false
	}
	return action, true
}

// Merge lays a user's keymap over the default one. The user's entries win,
// including an entry that unbinds a default; everything the user did not mention
// keeps its default. That is what makes a keymap file survive a new version
// binding a new action (D3: the file holds the user's changes, not a frozen copy
// of the table).
func Merge(base, over Keymap) Keymap {
	merged := make(Keymap, len(base)+len(over))
	for b, a := range base {
		merged[b] = a
	}
	for b, a := range over {
		merged[b] = a
	}
	return merged
}

// DefaultBindings is the embedded table. Every window has a binding (D4: the
// menubar must not be the only way to reach a tool, since a tool window can
// cover it), the four chords the CLI already had keep their keys, and the
// function keys carry the actions that want one press - none of them is a ZX
// matrix key, so none collides with the machine.
func DefaultBindings() Keymap {
	return Keymap{
		{Key: KeyQ, Mods: ModSuper}: ActQuit,
		{Key: KeyR, Mods: ModSuper}: ActReset,

		{Key: KeyF5}:  ActPauseToggle,
		{Key: KeyF10}: ActNMI,
		{Key: KeyF8}:  ActStepOne,
		{Key: KeyF9}:  ActTurboToggle,

		{Key: KeyP, Mods: ModSuper}:  ActTapePlayPause,
		{Key: KeyF6}:                 ActTapeRewind,
		{Key: KeyS, Mods: ModSuper}:  ActScreenshot,
		{Key: KeyF1, Mods: ModSuper}: ActLogMark,

		{Key: KeyM, Mods: ModSuper}:     ActToggleControl,
		{Key: KeyD, Mods: ModSuper}:     ActToggleDisks,
		{Key: KeyT, Mods: ModSuper}:     ActToggleTape,
		{Key: KeyK, Mods: ModSuper}:     ActToggleKeyboard,
		{Key: KeyB, Mods: ModSuper}:     ActToggleBindings,
		{Key: KeyComma, Mods: ModSuper}: ActToggleSettings,
		{Key: KeyG, Mods: ModSuper}:     ActToggleDebugger,
	}
}
