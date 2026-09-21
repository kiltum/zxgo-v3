package frontend

import (
	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/replay"
)

// ApplyRecordedSettings turns on every switch a session ran with, and reports
// whether it ran with fast tape.
//
// Fast tape is returned rather than applied because it is emulator state, not
// part of the machine config -- and that is exactly how it came to be recorded,
// printed and never used: a settings field nobody reads looks the same as one
// that works. Every field here is a switch that only turns something on, so a
// replay can add to a machine but never take away from it.
func ApplyRecordedSettings(cfg *model.Config, s replay.Settings) (fastTape bool) {
	if s.TurboSound {
		cfg.HasTurboSound = true
	}
	if s.TurboSoundFM {
		cfg.HasTurboSoundFM = true
	}
	if s.NoFDCTiming {
		cfg.NoFDCTiming = true
	}
	if s.GeneralSound {
		cfg.HasGS = true
	}
	if s.Snow {
		cfg.HasSnowEffect = true
	}
	return s.FastTape
}

// Choice is one option in a settings row: what the backend draws, the action a click asks
// for, and whether it is the one in force.
//
// The front end owns the list - which models exist, which Z80 variants - and the backend
// only draws it, exactly as it does with Tools() and KeyboardRows(). A row is a list of
// Choices rather than a control the backend knows how to build, so adding a model is a line
// here and nothing in `internal/ui`.
type Choice struct {
	Label  string
	Action Action
	// Chosen marks the option in force. The backend draws it as the selected one, and it
	// is the only thing that tells the user where they are: a row of buttons with nothing
	// selected reads as four things that could be true.
	Chosen bool
}

// Toggle is one switch row of the settings window: what the box says, the action a click asks
// for, whether it is ticked, and what the running machine has.
//
// On and Running differ only for a row that needs a relaunch (D10), and only between the click
// and the relaunch that applies it - which is what earns the row its badge. A row that applies
// at once has no badge to earn: its two values move together, so the window shows one state
// and the group's note says that it applies now.
type Toggle struct {
	Label  string
	Action Action
	// On is what the settings ask for, and what the box is drawn as.
	On bool
	// Running is what the built machine has.
	Running bool
}

// RestartRequired reports whether this row's setting is waiting for a relaunch.
func (t Toggle) RestartRequired() bool { return t.On != t.Running }

// ToggleGroup is a labelled group of switch rows, which is how the window draws them: one
// heading, the rows under it, and a line saying whether they apply now or at the next launch.
type ToggleGroup struct {
	Label string
	Rows  []Toggle
	// Note is the line under the rows, and is the front end's because whether a group is live
	// is: the backend draws a checkbox and cannot know whether ticking it changed the machine
	// or only this file.
	Note string
}

// machineSwitches are the switches the settings window offers, in one struct that the settings
// and the file they are written to share.
//
// It is a struct of its own because two things hold exactly these: the settings and the file.
// One struct in both means the JSON names are written once, and the table below ties a row to a
// field without either of them naming the fields again. It is embedded in both, so the fields
// are reached as s.TurboSound in either.
//
// Six of them, in two groups: the four sound devices, and the two speed switches (fast tape,
// and the floppy controller's seek and rotation latency). All six are three-state: nil is "no
// opinion", which leaves the model's own default - the Pentagon's TurboSound - in force.
//
// The JSON names are pkg/replay's for the switches the two have in common, deliberately: a
// replay's settings section and this file name the same five things, and one spelling means a
// reader who has seen either can read the other.
type machineSwitches struct {
	TurboSound   *bool `json:"turbosound,omitempty"`
	TurboSoundFM *bool `json:"turbosound_fm,omitempty"`
	GeneralSound *bool `json:"general_sound,omitempty"`
	Snow         *bool `json:"snow,omitempty"`
	FastTape     *bool `json:"fast_tape,omitempty"`
	NoFDCTiming  *bool `json:"no_fdc_timing,omitempty"`
}

// The groups the switch rows are drawn under, in the order they appear.
const (
	groupSound = "Sound"
	groupSpeed = "Speed"
)

// switchChoice ties a row, the action that flips it, the field it reads and writes and the
// machine's own answer together, so no two of them can drift apart.
type switchChoice struct {
	label  string
	action Action
	group  string
	// live applies the switch to the running machine, and is nil for a row that needs a
	// relaunch: that is D10's split, in one field. A live row is also the only kind whose
	// "what the settings ask for" and "what the machine has" cannot differ, since the two
	// move together.
	live func(Machine, bool)
	// apply writes the switch into a machine configuration, which is what a row that is not
	// live needs: the device is added or replaced when the machine is built. It is nil for
	// fast tape, which is emulator state rather than machine configuration.
	apply func(*model.Config, bool)
	// get reads the switch, and is nil when the settings have no opinion. set writes the
	// field, which is why the two are closures rather than one: reading a field and
	// replacing it are different operations on a three-state switch.
	get func(machineSwitches) *bool
	set func(*machineSwitches, *bool)
	// running reads the machine's answer, which for a live row is the machine and for a
	// restart-required one is what it was built with.
	running func(Machine) bool
}

// switchChoices is the switch section of the settings window, in the order it draws.
var switchChoices = []switchChoice{
	{
		label:   "TurboSound",
		action:  ActTurboSoundToggle,
		group:   groupSound,
		apply:   func(c *model.Config, on bool) { c.HasTurboSound = on },
		get:     func(s machineSwitches) *bool { return s.TurboSound },
		set:     func(s *machineSwitches, v *bool) { s.TurboSound = v },
		running: func(m Machine) bool { return m.ModelConfig().HasTurboSound },
	},
	{
		label:   "TurboSound FM",
		action:  ActTurboSoundFMToggle,
		group:   groupSound,
		apply:   func(c *model.Config, on bool) { c.HasTurboSoundFM = on },
		get:     func(s machineSwitches) *bool { return s.TurboSoundFM },
		set:     func(s *machineSwitches, v *bool) { s.TurboSoundFM = v },
		running: func(m Machine) bool { return m.ModelConfig().HasTurboSoundFM },
	},
	{
		label:   "General Sound",
		action:  ActGeneralSoundToggle,
		group:   groupSound,
		apply:   func(c *model.Config, on bool) { c.HasGS = on },
		get:     func(s machineSwitches) *bool { return s.GeneralSound },
		set:     func(s *machineSwitches, v *bool) { s.GeneralSound = v },
		running: func(m Machine) bool { return m.ModelConfig().HasGS },
	},
	{
		label:   "Snow",
		action:  ActSnowToggle,
		group:   groupSound,
		apply:   func(c *model.Config, on bool) { c.HasSnowEffect = on },
		get:     func(s machineSwitches) *bool { return s.Snow },
		set:     func(s *machineSwitches, v *bool) { s.Snow = v },
		running: func(m Machine) bool { return m.ModelConfig().HasSnowEffect },
	},
	{
		label:   "Fast tape (turbo)",
		action:  ActTurboToggle,
		group:   groupSpeed,
		live:    func(m Machine, on bool) { m.SetFastTape(on) },
		running: func(m Machine) bool { return m.FastTapeActive() },
		get:     func(s machineSwitches) *bool { return s.FastTape },
		set:     func(s *machineSwitches, v *bool) { s.FastTape = v },
	},
	{
		label:   "Fast disk loads (no seek or rotation timing)",
		action:  ActFDCTimingToggle,
		group:   groupSpeed,
		live:    func(m Machine, on bool) { m.SetNoFDCTiming(on) },
		apply:   func(c *model.Config, on bool) { c.NoFDCTiming = on },
		get:     func(s machineSwitches) *bool { return s.NoFDCTiming },
		set:     func(s *machineSwitches, v *bool) { s.NoFDCTiming = v },
		running: func(m Machine) bool { return m.NoFDCTiming() },
	},
}

// value reads a switch as the settings have it: the answer and whether they have one.
func (c switchChoice) value(s machineSwitches) (on, set bool) {
	if v := c.get(s); v != nil {
		return *v, true
	}
	return false, false
}

// copyBool copies a three-state switch by value, so two settings never share one. A shared
// pointer would make a copy that a caller could change through the original - and the loop
// persists what App.Settings() hands it, so the copy is the one that reaches the file.
func copyBool(p *bool) *bool {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// gets reads a switch as the machine will have it, with the built machine's own value as
// the answer when the settings are silent. It is what the window draws and what a click
// flips.
func (c switchChoice) gets(s machineSwitches, running bool) bool {
	if on, set := c.value(s); set {
		return on
	}
	return running
}

// modelChoice ties a machine to the action that selects it, and to the name the window draws
// on the row. The key is the registry's (pkg/model), so the three cannot disagree about what a
// model is called.
type modelChoice struct {
	key    string
	short  string
	action Action
}

// modelChoices is the machine section, in the order the settings window draws it: the four
// the emulator has always had, then the 512K Pentagon. A test keeps it equal to the registry,
// so a model added to pkg/model cannot be missing from the window.
//
// The short name is what the radio row carries, because five radio buttons with the registry's
// full names ("ZX Spectrum +2A/+3") do not fit one line; the full name of the machine in force
// is drawn under the row, which is where the detail belongs.
var modelChoices = []modelChoice{
	{"48k", "48K", ActModel48K},
	{"128k", "128K", ActModel128K},
	{"2a3", "+2A/+3", ActModel2A3},
	{"pentagon", "Pentagon", ActModelPentagon},
	{"pentagon512", "Pentagon 512", ActModelPentagon512},
}

// ModelChoices is the machines the settings window offers, with the one in force flagged.
// running is the machine that is actually built, which is the answer when the settings have
// never been asked.
func ModelChoices(s Settings, running string) []Choice {
	chosen := s.modelOrDefault(running)
	out := make([]Choice, 0, len(modelChoices))
	for _, m := range modelChoices {
		if _, ok := model.AllModels[m.key]; !ok {
			continue // a registry entry that went away: nothing to offer
		}
		out = append(out, Choice{
			Label:  m.short,
			Action: m.action,
			Chosen: m.key == chosen,
		})
	}
	return out
}

// ModelName is the full name of the machine the settings ask for, which the window draws
// under the row: the row itself carries short names, and "Pentagon 128" is not one of them.
func ModelName(s Settings, running string) string {
	chosen := s.modelOrDefault(running)
	if cfg, ok := model.AllModels[chosen]; ok {
		return cfg.Name
	}
	return chosen
}

// modelKeyForAction reports which model an action selects, which is what Dispatch builds
// when the settings window reports the click.
func modelKeyForAction(act Action) (string, bool) {
	for _, m := range modelChoices {
		if m.action == act {
			return m.key, true
		}
	}
	return "", false
}

// CPUChoices is the Z80 variant row: the two variants, with the one the machine is running
// flagged. It is the row that applies at once (D10), so the flag follows the machine and not
// the file.
func CPUChoices(isNMOS bool) []Choice {
	return []Choice{
		{Label: "NMOS", Action: ActZ80NMOS, Chosen: isNMOS},
		{Label: "CMOS", Action: ActZ80CMOS, Chosen: !isNMOS},
	}
}

// MEMPTRChoices is the MEMPTR row: the two behaviours the repeating block I/O instructions
// have, with the one the machine is running flagged. Both are implemented and neither is
// settled to be what silicon does (KNOWN_BUGS.md), so the row says which is which rather than
// calling one of them correct.
func MEMPTRChoices(real bool) []Choice {
	return []Choice{
		{Label: "Measured (PC+1)", Action: ActMEMPTRReal, Chosen: real},
		{Label: "Documented (BC)", Action: ActMEMPTRDocumented, Chosen: !real},
	}
}

// switchRows builds one row per switch in a group, with what the settings ask for and what
// the machine is running. The two differ only for a row that needs a relaunch (D10).
func switchRows(s Settings, m Machine, group string) []Toggle {
	var out []Toggle
	for _, c := range switchChoices {
		if c.group != group {
			continue
		}
		running := c.running(m)
		out = append(out, Toggle{
			Label:   c.label,
			Action:  c.action,
			On:      c.gets(s.machineSwitches, running),
			Running: running,
		})
	}
	return out
}

// SwitchGroups is the switch section as the window draws it: the groups in the order the
// table names them, each with the rows that belong to it.
func SwitchGroups(s Settings, m Machine) []ToggleGroup {
	groups := []struct {
		label string
		note  string
	}{
		{groupSound, "A sound card is built with the machine, so these apply at the next launch."},
		{groupSpeed, "These two apply now: they change how a load runs, not what the machine is."},
	}
	out := make([]ToggleGroup, 0, len(groups))
	for _, g := range groups {
		rows := switchRows(s, m, g.label)
		if len(rows) == 0 {
			continue
		}
		out = append(out, ToggleGroup{Label: g.label, Note: g.note, Rows: rows})
	}
	return out
}

// setSwitch flips the switch an action names, writing the opposite of what the settings
// currently ask for. The running machine decides the starting point only when the settings
// have no opinion, so clicking twice with a relaunch in between cannot leave a switch stuck.
//
// A live row is applied to the machine here as well as written down, which is what makes the
// same action serve both the settings window's checkbox and the button the tape and disks
// windows already had: one setting, one path to it.
func (a *App) setSwitch(act Action) bool {
	for _, c := range switchChoices {
		if c.action != act {
			continue
		}
		on := !c.gets(a.settings.machineSwitches, c.running(a.Machine))
		c.set(&a.settings.machineSwitches, &on)
		if c.live != nil {
			c.live(a.Machine, on)
		}
		return true
	}
	return false
}

// SetZ80 records the CPU variant and applies it to the machine. It is one of the settings rows
// that applies live (D10): the variant changes the undocumented flags, not the instruction set,
// so the machine can change its mind without being rebuilt.
//
// The reset is what section 6.4 asks for and it is why it happens here rather than inside
// SetCPUType: the registers would otherwise hold flags computed under the other chip. It
// discards the running state, which is the price of changing the chip under a live machine - a
// startup, which has a restored machine to keep, applies the variant without it.
func (a *App) SetZ80(isNMOS bool) {
	if isNMOS {
		a.settings.Z80 = Z80NMOS
	} else {
		a.settings.Z80 = Z80CMOS
	}
	a.Machine.SetCPUType(isNMOS)
	a.Machine.Reset()
}

// SetMEMPTR records the MEMPTR behaviour and applies it to the machine, which is the other
// live row: both behaviours are implemented and neither needs a reset, because the switch
// decides what the *next* block instruction leaves in MEMPTR rather than re-reading anything
// already computed (KNOWN_BUGS.md).
func (a *App) SetMEMPTR(real bool) {
	if real {
		a.settings.MEMPTR = MEMPTRReal
	} else {
		a.settings.MEMPTR = MEMPTRDocumented
	}
	a.Machine.SetMEMPTRReal(real)
}

// RestartRequired reports whether the settings ask for something the built machine does not
// have: the model, or one of the sound devices, which are added or replaced when the machine is
// built (D10). The CPU rows are not among them, because they apply at once.
//
// It is computed rather than remembered, so it cannot get stuck: a flag set when the window is
// clicked would still be set after the relaunch that applied it, and would have to be cleared
// by whoever knew the machine had been built.
func (a *App) RestartRequired() bool {
	cfg := a.Machine.ModelConfig()
	if a.settings.Model != "" && a.settings.Model != cfg.Key {
		return true
	}
	for _, c := range switchChoices {
		running := c.running(a.Machine)
		if c.gets(a.settings.machineSwitches, running) != running {
			return true
		}
	}
	return false
}

// SettingsView returns the settings window's state.
func (a *App) SettingsView() SettingsView {
	cfg := a.Machine.ModelConfig()
	s := a.settings

	toggles := SwitchGroups(s, a.Machine)
	// The session is a switch like the others and is drawn the same way, but it is not one of
	// the machine's: it is the front end's own record of whether the state is carried between
	// runs (D11), so it is appended here rather than living in the switch table.
	toggles = append(toggles, ToggleGroup{
		Label: "Session",
		Rows: []Toggle{{
			Label:   "Save the state on exit",
			Action:  ActSessionToggle,
			On:      s.SessionEnabled(),
			Running: s.SessionEnabled(),
		}},
		Note: "Restored at the next start; turning it off stops this run's being written too.",
	})

	return SettingsView{
		Radios: []RadioGroup{
			{
				Label:   "Machine",
				Options: ModelChoices(s, cfg.Key),
				Note:    ModelName(s, cfg.Key) + " - applies at the next launch.",
			},
			{
				Label:   "Z80",
				Options: CPUChoices(a.Machine.IsNMOS()),
				Note:    "Applies now: the variant changes the undocumented flags, not the instruction set.",
			},
			{
				Label:   "MEMPTR on the repeating block I/O",
				Options: MEMPTRChoices(a.Machine.IsMEMPTRReal()),
				Note:    "Applies now. Both are implemented; see KNOWN_BUGS.md for why the choice exists.",
			},
		},
		Toggles: toggles,
		Restart: a.RestartRequired(),
	}
}

// relaunchNow asks to leave and start again with the settings as they now stand. The
// emulator cannot rebuild its peripheral graph in place (D10), so a model or a sound card
// change is applied by a new process reading the same settings file - and the exit is the
// ordinary one, so the session and any recording are written on the way out.
func (a *App) relaunchNow() error {
	a.relaunch = true
	a.quit = true
	a.Notify("Relaunching with the new settings")
	return nil
}

// RelaunchWanted reports whether the user asked to start again, which is what the caller
// outside the loop turns into a new process (ActRelaunch).
func (a *App) RelaunchWanted() bool { return a.relaunch }
