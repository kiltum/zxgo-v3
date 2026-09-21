package frontend

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/replay"
)

// TestApplyRecordedSettingsTurnsOnEveryField exists because five of the six
// settings were wired and fast tape was not: it was recorded, printed back as
// "Fast tape: playback runs unthrottled", and never applied, so a replay of a
// fast load ran throttled while claiming otherwise. Applying settings is a
// place where the compiler cannot help -- a field that nobody reads looks
// exactly like a field that works -- so this checks each one has an effect
// rather than trusting the shape of the code.
func TestApplyRecordedSettingsTurnsOnEveryField(t *testing.T) {
	// Every switch off by default on a 48K, so a field that is wired shows up as
	// a change and a field that is forgotten shows up as silence.
	cfg := model.Spectrum48K
	if cfg.HasTurboSound || cfg.HasTurboSoundFM || cfg.HasGS ||
		cfg.HasSnowEffect || cfg.NoFDCTiming {
		t.Fatalf("test precondition: 48K should start with every switch off")
	}

	all := replay.Settings{
		TurboSound:   true,
		TurboSoundFM: true,
		GeneralSound: true,
		Snow:         true,
		NoFDCTiming:  true,
		FastTape:     true,
	}
	fastTape := ApplyRecordedSettings(&cfg, all)

	effects := []struct {
		name string
		got  bool
	}{
		{"TurboSound", cfg.HasTurboSound},
		{"TurboSoundFM", cfg.HasTurboSoundFM},
		{"GeneralSound", cfg.HasGS},
		{"Snow", cfg.HasSnowEffect},
		{"NoFDCTiming", cfg.NoFDCTiming},
		{"FastTape", fastTape},
	}
	for _, e := range effects {
		if !e.got {
			t.Errorf("%s was recorded but not applied", e.name)
		}
	}
}

// Applying a replay must never turn off a switch the model itself enables: the
// Pentagon has TurboSound whatever any file says, and a recording made on it
// does not carry a "turn it on" for a machine that already had it.
func TestApplyRecordedSettingsKeepsModelDefaults(t *testing.T) {
	cfg := model.Pentagon128
	if !cfg.HasTurboSound {
		t.Fatalf("test precondition: the Pentagon config should have TurboSound on")
	}

	fastTape := ApplyRecordedSettings(&cfg, replay.Settings{})

	if !cfg.HasTurboSound {
		t.Error("an empty settings section turned off the model's own TurboSound")
	}
	if fastTape {
		t.Error("fast tape was reported on when nothing asked for it")
	}
}

// The settings window's switches are three-state, and the direction the command line cannot
// express is the one worth testing: a flag only ever turns something on, so "off" is a
// setting's alone - while a switch nobody mentioned leaves the model's own answer in force.
func TestApplyToCanTurnAModelDefaultOff(t *testing.T) {
	off := false
	on := true

	pentagon := model.Pentagon128
	if !pentagon.HasTurboSound {
		t.Fatalf("test precondition: the Pentagon should have TurboSound on")
	}
	(Settings{machineSwitches: machineSwitches{TurboSound: &off}}).ApplyTo(&pentagon)
	if pentagon.HasTurboSound {
		t.Error("a settings file that turns TurboSound off left it on")
	}

	// And the other direction on the machine that has nothing: a 48K gains the card.
	fortyEight := model.Spectrum48K
	(Settings{machineSwitches: machineSwitches{GeneralSound: &on}}).ApplyTo(&fortyEight)
	if !fortyEight.HasGS {
		t.Error("a settings file that turns the General Sound on left it off")
	}
	if fortyEight.HasTurboSound {
		t.Error("turning one device on turned another one on with it")
	}

	// A file that mentions nothing leaves everything as the model has it.
	untouched := model.Pentagon128
	(Settings{}).ApplyTo(&untouched)
	if !untouched.HasTurboSound {
		t.Error("an empty settings value turned off a model default")
	}
}

// Every model the emulator has is offered by the settings window, and the row is built
// from the registry rather than from a copy of it. A model added to pkg/model and not to the
// window would be a machine a user cannot choose, which is the kind of gap a test of the
// registry alone would not see.
func TestModelChoicesCoverTheRegistry(t *testing.T) {
	offered := make(map[string]bool, len(modelChoices))
	for _, m := range modelChoices {
		if offered[m.key] {
			t.Errorf("%s is offered twice", m.key)
		}
		offered[m.key] = true
		if _, ok := model.AllModels[m.key]; !ok {
			t.Errorf("%s is offered but is not in the registry", m.key)
		}
	}
	for key := range model.AllModels {
		if !offered[key] {
			t.Errorf("%s is in the registry but the settings window does not offer it", key)
		}
	}

	// And the actions round-trip: a click on a model's button names that model.
	for _, m := range modelChoices {
		got, ok := modelKeyForAction(m.action)
		if !ok || got != m.key {
			t.Errorf("%v came back as %q, %v; want %q", m.action, got, ok, m.key)
		}
	}
	if _, ok := modelKeyForAction(ActReset); ok {
		t.Error("an action that is not a model named one")
	}
}

// The model row flags what the settings ask for, and the answer with nothing chosen is the
// machine that is running: a window that flagged nothing would read as a machine with no
// model at all.
func TestModelChoicesFlagTheChosenMachine(t *testing.T) {
	flagOf := func(list []Choice) string {
		for _, c := range list {
			if c.Chosen {
				return c.Label
			}
		}
		return ""
	}
	// The registry's full name for a model is not what the row carries: five radio buttons fit
	// one line only because the front end picks their labels, and the full name of the machine
	// in force is drawn under the row instead.
	shortName := func(key string) string {
		for _, m := range modelChoices {
			if m.key == key {
				return m.short
			}
		}
		return ""
	}

	if got := flagOf(ModelChoices(Settings{}, model.Spectrum48K.Key)); got != shortName("48k") {
		t.Errorf("with nothing chosen the row flags %q, want the running machine", got)
	}

	pentagon := Settings{Model: "pentagon"}
	chosen := ModelChoices(pentagon, model.Spectrum48K.Key)
	if got := flagOf(chosen); got != shortName("pentagon") {
		t.Errorf("the row flags %q, want the chosen machine", got)
	}
	if got := ModelName(pentagon, model.Spectrum48K.Key); got != model.Pentagon128.Name {
		t.Errorf("the line under the row says %q, want the full name", got)
	}
	// Every model is still on the row: the chosen one is flagged, not the only one offered,
	// or a user who picked the wrong machine could never pick another.
	if len(chosen) != len(modelChoices) {
		t.Errorf("the row has %d entries, want %d", len(chosen), len(modelChoices))
	}
}

// The Z80 row follows the machine, not the file: it is a live row (D10), so a window that read
// the setting would show a variant the machine is not running the moment a session was resumed
// with the other one. The MEMPTR row is its twin.
func TestCPUChoicesFollowTheMachine(t *testing.T) {
	nmos := CPUChoices(true)
	if len(nmos) != 2 || !nmos[0].Chosen || nmos[1].Chosen {
		t.Errorf("NMOS is not the flagged variant of %+v", nmos)
	}
	cmos := CPUChoices(false)
	if cmos[0].Chosen || !cmos[1].Chosen {
		t.Errorf("CMOS is not the flagged variant of %+v", cmos)
	}

	measured := MEMPTRChoices(true)
	if len(measured) != 2 || !measured[0].Chosen || measured[1].Chosen {
		t.Errorf("the measured behaviour is not the flagged one of %+v", measured)
	}
	documented := MEMPTRChoices(false)
	if documented[0].Chosen || !documented[1].Chosen {
		t.Errorf("the documented behaviour is not the flagged one of %+v", documented)
	}
}

// The settings' own copy of a switch follows the settings, and the running value follows the
// machine: telling those two apart is the whole reason a row carries both.
func TestSwitchGroupsSeparateChosenFromRunning(t *testing.T) {
	off := false
	app, m := testApp()
	m.cfg = model.Pentagon128
	if !m.cfg.HasTurboSound {
		t.Fatalf("test precondition: the Pentagon should have TurboSound on")
	}

	// Nothing said: the model's own answer is both, and no row is waiting for a relaunch.
	groups := SwitchGroups(app.Settings(), m)
	sound := groupNamed(t, groups, groupSound)
	if len(sound.Rows) != 4 {
		t.Fatalf("the sound group has %d rows, want 4", len(sound.Rows))
	}
	if !sound.Rows[0].On || !sound.Rows[0].Running || sound.Rows[0].RestartRequired() {
		t.Errorf("an unmentioned switch on the Pentagon = %+v; want on and not restarting", sound.Rows[0])
	}

	// Turned off: the box is clear, the machine still has it, and the row says so.
	app.settings.TurboSound = &off
	sound = groupNamed(t, SwitchGroups(app.Settings(), m), groupSound)
	if sound.Rows[0].On || !sound.Rows[0].Running {
		t.Errorf("a switch turned off = %+v; want the box clear and the machine still on", sound.Rows[0])
	}
	if !sound.Rows[0].RestartRequired() {
		t.Error("a switch the machine has not picked up does not ask for a relaunch")
	}
}

// groupNamed finds a group, and fails the test rather than returning nothing: a group that
// disappeared would make every assertion about its rows pass vacuously.
func groupNamed(t *testing.T, groups []ToggleGroup, label string) ToggleGroup {
	t.Helper()
	for _, g := range groups {
		if g.Label == label {
			return g
		}
	}
	t.Fatalf("no %q group in %+v", label, groups)
	return ToggleGroup{}
}

// A settings file carries the machine and the switches, and one that does not mention a
// switch leaves the model's own default in force rather than turning it off.
func TestSettingsRoundTripCarriesTheMachine(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	app, _ := testApp()

	off := false
	app.settings.Model = "pentagon512"
	app.settings.Z80 = Z80CMOS
	app.settings.MEMPTR = MEMPTRDocumented
	app.settings.Snow = &off

	if err := store.SaveSettings(app.Settings()); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	data, err := os.ReadFile(store.SettingsPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"model": "pentagon512"`) ||
		!strings.Contains(string(data), `"z80": "cmos"`) ||
		!strings.Contains(string(data), `"memptr": "documented"`) ||
		!strings.Contains(string(data), `"snow": false`) {
		t.Errorf("the settings file does not carry the machine:\n%s", data)
	}
	if strings.Contains(string(data), "turbosound") {
		t.Errorf("a switch nobody set was written:\n%s", data)
	}

	loaded, err := store.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	restarted, _ := testApp()
	restarted.SetSettings(loaded)
	got := restarted.Settings()
	if got.Model != "pentagon512" || got.Z80 != Z80CMOS || got.MEMPTR != MEMPTRDocumented {
		t.Errorf("the machine came back as %q/%q/%q", got.Model, got.Z80, got.MEMPTR)
	}
	if got.Snow == nil || *got.Snow {
		t.Errorf("the snow switch came back as %v; want off", got.Snow)
	}
	if got.TurboSound != nil {
		t.Errorf("a switch the file does not mention came back as %v", *got.TurboSound)
	}
	if isNMOS, ok := got.Z80Choice(); !ok || isNMOS {
		t.Errorf("the Z80 choice came back as %v, %v; want CMOS", isNMOS, ok)
	}
	if _, ok := (Settings{}).Z80Choice(); ok {
		t.Error("a settings value with no Z80 in it claimed to have one")
	}
	if realMem, ok := got.MEMPTRRealChoice(); !ok || realMem {
		t.Errorf("the MEMPTR choice came back as %v, %v; want documented", realMem, ok)
	}
	if _, ok := (Settings{}).MEMPTRRealChoice(); ok {
		t.Error("a settings value with no MEMPTR in it claimed to have one")
	}
}

// A model or a Z80 variant this build does not know is reported rather than kept: a model
// that is not in the registry would be a startup failure at the next launch, which is a
// worse place to find a typo than the line that reports it.
func TestLoadSettingsRejectsUnknownMachine(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	body := `{"version":1,"model":"spectrum3","z80":"cmos","snow":true}`
	if err := os.WriteFile(store.SettingsPath(), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	set, err := store.LoadSettings()
	if err == nil {
		t.Error("an unknown model was accepted silently")
	}
	if set.Model != "" {
		t.Errorf("the unknown model %q was kept", set.Model)
	}
	if !strings.Contains(err.Error(), "model: spectrum3") {
		t.Errorf("the report does not name the entry it ignored: %v", err)
	}
	// The rest of the file is still read: one bad entry does not throw away the good ones.
	if set.Z80 != Z80CMOS {
		t.Errorf("the Z80 variant was dropped with the model: %+v", set)
	}
	if set.Snow == nil || !*set.Snow {
		t.Errorf("the switch was dropped with the model: %+v", set.Snow)
	}

	if err := os.WriteFile(store.SettingsPath(), []byte(`{"version":1,"model":"48k","z80":"nmos-ish"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	set, err = store.LoadSettings()
	if err == nil || !strings.Contains(err.Error(), "z80: nmos-ish") {
		t.Errorf("an unknown Z80 variant = %+v, %v", set, err)
	}
	if set.Z80 != "" {
		t.Errorf("the unknown variant %q was kept", set.Z80)
	}
	if set.Model != "48k" {
		t.Errorf("the known model was dropped with the unknown variant: %+v", set)
	}

	// The MEMPTR behaviour is a third name in the same file and gets the same treatment.
	if err := os.WriteFile(store.SettingsPath(), []byte(`{"version":1,"memptr":"real-ish"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	set, err = store.LoadSettings()
	if err == nil || !strings.Contains(err.Error(), "memptr: real-ish") {
		t.Errorf("an unknown MEMPTR behaviour = %+v, %v", set, err)
	}
	if set.MEMPTR != "" {
		t.Errorf("the unknown behaviour %q was kept", set.MEMPTR)
	}
}

// Two settings are compared by meaning, so the loop does not rewrite the file because a
// pointer moved: the switches are the fields most likely to be copied wrongly.
func TestSettingsEqualIgnoresPointerIdentity(t *testing.T) {
	on1, on2 := true, true
	base := Settings{machineSwitches: machineSwitches{Snow: &on1}}
	if !base.Equal(Settings{machineSwitches: machineSwitches{Snow: &on2}}) {
		t.Error("two settings that both say the same thing are not equal")
	}
	if base.Equal(Settings{}) {
		t.Error("a switch that was set is equal to one that was not")
	}
	if !base.Equal(Settings{Session: &on1, machineSwitches: machineSwitches{Snow: &on2}}) {
		t.Error("a session left on is not equal to no opinion about it")
	}

	if (Settings{Model: "48k"}).Equal(Settings{}) {
		t.Error("a chosen model is equal to no choice")
	}
	if (Settings{Model: "48k"}).Equal(Settings{Model: "128k"}) {
		t.Error("two different models are equal")
	}
	if (Settings{Z80: Z80NMOS}).Equal(Settings{Z80: Z80CMOS}) {
		t.Error("two different Z80 variants are equal")
	}
}

// The Z80 variant is the row that applies at once (D10): the action reaches the machine, and
// the setting is recorded so the choice survives the run. It is not a restart-required row,
// which is what the view says.
func TestZ80ChoiceAppliesLive(t *testing.T) {
	app, m := testApp()
	m.isNMOS = true

	if err := app.Dispatch(ActZ80CMOS); err != nil {
		t.Fatalf("ActZ80CMOS: %v", err)
	}
	if m.isNMOS {
		t.Error("the machine is still NMOS after CMOS was chosen")
	}
	if got := app.Settings().Z80; got != Z80CMOS {
		t.Errorf("the choice was not recorded: z80 = %q", got)
	}
	// The reset is section 6.4's: the registers were holding flags computed under the other
	// chip, so the machine starts again rather than carrying them into the new one.
	if m.resets == 0 {
		t.Error("changing the chip under a running machine did not reset it")
	}
	if app.RestartRequired() {
		t.Error("a live setting asked for a relaunch")
	}
	if row := radioNamed(t, app.SettingsView(), "Z80"); !row[1].Chosen || row[0].Chosen {
		t.Errorf("the row does not flag CMOS: %+v", row)
	}

	if err := app.Dispatch(ActZ80NMOS); err != nil {
		t.Fatalf("ActZ80NMOS: %v", err)
	}
	if !m.isNMOS {
		t.Error("the machine is still CMOS after NMOS was chosen")
	}
}

// MEMPTR is the Z80 row's twin: a second CPU behaviour, applied live, and recorded so the
// choice outlives the run. It needs no reset, unlike the chip variant - nothing was computed
// under the old switch that the new one invalidates (KNOWN_BUGS.md).
func TestMEMPTRChoiceAppliesLive(t *testing.T) {
	app, m := testApp()
	if !m.memptr {
		t.Fatal("test precondition: a machine should start on the measured behaviour")
	}
	resets := m.resets

	if err := app.Dispatch(ActMEMPTRDocumented); err != nil {
		t.Fatalf("ActMEMPTRDocumented: %v", err)
	}
	if m.memptr {
		t.Error("the machine is still on the measured behaviour after the documented one was chosen")
	}
	if got := app.Settings().MEMPTR; got != MEMPTRDocumented {
		t.Errorf("the choice was not recorded: memptr = %q", got)
	}
	if m.resets != resets {
		t.Error("changing the MEMPTR behaviour reset the machine, which it has no reason to")
	}
	if app.RestartRequired() {
		t.Error("a live setting asked for a relaunch")
	}
	row := radioNamed(t, app.SettingsView(), "MEMPTR on the repeating block I/O")
	if !row[1].Chosen || row[0].Chosen {
		t.Errorf("the row does not flag the documented behaviour: %+v", row)
	}

	if err := app.Dispatch(ActMEMPTRReal); err != nil {
		t.Fatalf("ActMEMPTRReal: %v", err)
	}
	if !m.memptr {
		t.Error("the machine did not go back to the measured behaviour")
	}
}

// The model and the sound devices need a relaunch (D10), and the view says which rows are
// waiting for one: a click that looked like it had taken effect would be a lie about the
// machine the user is running.
func TestModelAndSoundNeedARelaunch(t *testing.T) {
	app, m := testApp() // a 48K, so every switch starts off

	if err := app.Dispatch(ActModelPentagon); err != nil {
		t.Fatalf("ActModelPentagon: %v", err)
	}
	if got := app.Settings().Model; got != "pentagon" {
		t.Errorf("the chosen model was recorded as %q", got)
	}
	if m.cfg.Key != "48k" {
		t.Error("the running machine was rebuilt in place, which D10 says it cannot be")
	}
	if !app.RestartRequired() {
		t.Error("a chosen model the machine is not running does not ask for a relaunch")
	}

	view := app.SettingsView()
	models := radioNamed(t, view, "Machine")
	if !models[3].Chosen {
		t.Errorf("the model row does not flag the chosen machine: %+v", models)
	}
	if !view.Restart {
		t.Error("the view does not report the pending relaunch")
	}

	// A sound switch is the same shape, and turning it off is a choice too: the Pentagon's
	// TurboSound is on by default, and a user who turns it off means it.
	if err := app.Dispatch(ActTurboSoundToggle); err != nil {
		t.Fatalf("ActTurboSoundToggle: %v", err)
	}
	s := app.Settings()
	if s.TurboSound == nil || !*s.TurboSound {
		t.Errorf("the switch was not turned on: %v", s.TurboSound)
	}
	if err := app.Dispatch(ActTurboSoundToggle); err != nil {
		t.Fatalf("ActTurboSoundToggle: %v", err)
	}
	s = app.Settings()
	if s.TurboSound == nil || *s.TurboSound {
		t.Errorf("the switch did not turn back off: %v", s.TurboSound)
	}
	row := toggleNamed(t, app.SettingsView(), groupSound, "TurboSound")
	if row.On || row.Running {
		t.Errorf("the row = %+v; want the switch off on a machine that never had it", row)
	}
	if row.RestartRequired() {
		t.Error("a switch that agrees with the machine asks for a relaunch")
	}
}

// The two speed switches are live: the same action serves the settings window's checkbox and
// the button the tape and disks windows already had, so ticking either one changes the machine
// and is remembered for the next run.
func TestSpeedSwitchesApplyLiveAndPersist(t *testing.T) {
	app, m := testApp()

	if err := app.Dispatch(ActTurboToggle); err != nil {
		t.Fatalf("ActTurboToggle: %v", err)
	}
	if !m.fastTape {
		t.Error("the fast tape row did not reach the machine")
	}
	if s := app.Settings(); s.FastTape == nil || !*s.FastTape {
		t.Errorf("the fast tape setting was not recorded: %v", s.FastTape)
	}
	row := toggleNamed(t, app.SettingsView(), groupSpeed, "Fast tape (turbo)")
	if !row.On || !row.Running || row.RestartRequired() {
		t.Errorf("the row = %+v; want it ticked and running, with nothing to relaunch for", row)
	}
	if app.RestartRequired() {
		t.Error("a live switch asked for a relaunch")
	}

	// The disk timing switch is the same shape, and its stored value is the "off" one - the
	// faithful setting is the box being clear - so the row says what it means rather than
	// what the field is called.
	if err := app.Dispatch(ActFDCTimingToggle); err != nil {
		t.Fatalf("ActFDCTimingToggle: %v", err)
	}
	if !m.noFDCTiming {
		t.Error("the disk timing row did not reach the machine")
	}
	row = toggleNamed(t, app.SettingsView(), groupSpeed, "Fast disk loads (no seek or rotation timing)")
	if !row.On || !row.Running {
		t.Errorf("the row = %+v; want it ticked and running", row)
	}

	// And the other way: the same action from a tape or disks window, and back off again.
	if err := app.Dispatch(ActTurboToggle); err != nil {
		t.Fatalf("ActTurboToggle: %v", err)
	}
	if m.fastTape {
		t.Error("the switch did not turn back off")
	}
	if s := app.Settings(); s.FastTape == nil || *s.FastTape {
		t.Errorf("turning it off was not recorded: %v", s.FastTape)
	}
}

// radioNamed and toggleNamed find a row, and fail rather than returning nothing: a row that
// disappeared would make every assertion about it pass vacuously.
func radioNamed(t *testing.T, view SettingsView, label string) []Choice {
	t.Helper()
	for _, g := range view.Radios {
		if g.Label == label {
			return g.Options
		}
	}
	t.Fatalf("no %q radio group in %+v", label, view.Radios)
	return nil
}

func toggleNamed(t *testing.T, view SettingsView, group, label string) Toggle {
	t.Helper()
	rows := groupNamed(t, view.Toggles, group).Rows
	for _, r := range rows {
		if r.Label == label {
			return r
		}
	}
	t.Fatalf("no %q row in the %q group: %+v", label, group, rows)
	return Toggle{}
}

// A relaunch is asked for, not assumed: the window's button is what sets it, and nothing
// else in the settings does.
func TestRelaunchIsAskedFor(t *testing.T) {
	app, _ := testApp()
	app.SetSettings(Settings{Model: "pentagon"})

	if app.RelaunchWanted() {
		t.Error("a pending change relaunched the emulator by itself")
	}
	if app.Quitting() {
		t.Error("the app is quitting before anyone asked")
	}

	if err := app.Dispatch(ActRelaunch); err != nil {
		t.Fatalf("ActRelaunch: %v", err)
	}
	if !app.RelaunchWanted() {
		t.Error("the relaunch was not recorded")
	}
	if !app.Quitting() {
		t.Error("a relaunch does not leave the loop, so nothing would start the new process")
	}
}

// The session row is the D11 setting, and the window is where it is turned off: the setting
// reaches SessionFiles through the same value the tests there use.
func TestSessionToggle(t *testing.T) {
	app, _ := testApp()
	if !app.Settings().SessionEnabled() {
		t.Fatal("test precondition: the session starts on")
	}
	if err := app.Dispatch(ActSessionToggle); err != nil {
		t.Fatalf("ActSessionToggle: %v", err)
	}
	if app.Settings().SessionEnabled() {
		t.Error("the session is still on after the row was clicked")
	}
	if row := toggleNamed(t, app.SettingsView(), "Session", "Save the state on exit"); row.On {
		t.Errorf("the view still says the session is on: %+v", row)
	}
}

// The settings window's state is refreshed only while it is open, like every other tool
// window (section 5.6): this is what makes the window show the machine rather than whatever
// was true when it was last looked at.
func TestSettingsViewIsRefreshedWhileOpen(t *testing.T) {
	app, m := testApp()
	app.Tick(time.Now())
	if app.Views().Settings.Radios != nil {
		t.Error("the settings view was built for a window that is not open")
	}

	app.OpenTool(ToolSettings)
	m.cfg = model.Pentagon128
	app.Tick(time.Now())

	row := toggleNamed(t, app.Views().Settings, groupSound, "TurboSound")
	if !row.Running {
		t.Errorf("the open window does not show the machine that is running: %+v", row)
	}
}
