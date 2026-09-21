package frontend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kiltum/zxgo-v3/internal/emulator"
	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/sound"
)

// The session setting is on unless a file says otherwise: absent and false are not the
// same thing, which is why it is a pointer rather than a bool. The same shape, and the
// same reason, as the layout's menu_visible - where getting it wrong hid the menubar.
func TestSessionDefaultsOn(t *testing.T) {
	if !(Settings{}).SessionEnabled() {
		t.Error("a settings value that mentions nothing about the session has it off")
	}

	off := Settings{}
	off.SetSession(false)
	if off.SessionEnabled() {
		t.Error("the session is on after it was turned off")
	}
	on := Settings{}
	on.SetSession(true)
	if !on.SessionEnabled() {
		t.Error("the session is off after it was turned on")
	}

	// The file side: a settings file that does not mention it leaves it on, and one that
	// does says what it says.
	store := Store{Dir: t.TempDir()}
	if err := os.WriteFile(store.SettingsPath(), []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if !loaded.SessionEnabled() {
		t.Error("a settings file that says nothing about the session turned it off")
	}

	off = Settings{}
	off.SetSession(false)
	if err := store.SaveSettings(off); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	loaded, err = store.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if loaded.SessionEnabled() {
		t.Error("the session came back on after being saved off")
	}

	// And a settings file with no session key at all is not a reason to rewrite anything:
	// nothing changed, so nothing is saved.
	app, _ := testApp()
	app.SetSettings(Settings{})
	if !app.Settings().Equal(Settings{}) {
		t.Error("an unmentioned session counts as a change, so a save would fire on startup")
	}
}

// Which files a session run uses, and the one case in which it uses none: a replay run.
//
// This is the rule that was two rules and did not cover the case that bit: a replay was
// played, the session was written on the way out, and the next replay of the same file
// continued from where the first one stopped instead of starting again.
func TestSessionFiles(t *testing.T) {
	store := Store{Dir: "/config/zxgo-v3"}
	on := Settings{}
	on.SetSession(true)
	off := Settings{}
	off.SetSession(false)

	// An ordinary run: the config directory's session when nothing was named, the explicit
	// path when one was.
	plain := SessionRequest{Settings: on, Store: store, Model: "48k"}
	resume, saveTo, err := SessionFiles(plain)
	if err != nil {
		t.Fatalf("SessionFiles: %v", err)
	}
	if resume != store.SessionPath("48k") || saveTo != store.SessionPath("48k") {
		t.Errorf("files = %q / %q, want the 48K session", resume, saveTo)
	}

	named := plain
	named.LoadExplicit, named.SaveExplicit = "/load.zxstate", "/save.zxstate"
	resume, saveTo, err = SessionFiles(named)
	if err != nil {
		t.Fatalf("SessionFiles: %v", err)
	}
	if resume != "/load.zxstate" || saveTo != "/save.zxstate" {
		t.Errorf("files = %q / %q, want the two explicit paths", resume, saveTo)
	}

	// The setting off, with nothing named: no session, and no complaint - the user never
	// asked for one.
	offRun := plain
	offRun.Settings = off
	resume, saveTo, err = SessionFiles(offRun)
	if resume != "" || saveTo != "" || err != nil {
		t.Errorf("files = %q / %q, %v; want nothing and no error", resume, saveTo, err)
	}

	// A replay run has no session, whether the replay is playing or being recorded.
	for _, what := range []string{"playing", "recording"} {
		replayRun := plain
		replayRun.Replay = true
		resume, saveTo, err = SessionFiles(replayRun)
		if resume != "" || saveTo != "" {
			t.Errorf("%s a replay: files = %q / %q, want none", what, resume, saveTo)
		}
		if err != nil {
			t.Errorf("%s a replay: %v - a run that only would have used the default says nothing", what, err)
		}
	}

	// And a user who named one is told why.
	named.Replay = true
	if _, _, err := SessionFiles(named); err != ErrReplayHasNoSession {
		t.Errorf("an explicit session in a replay run = %v, want ErrReplayHasNoSession", err)
	}

	// Each model keeps its own session, which is what stops one machine's run from
	// overwriting another's: a 48K run used to refuse a Pentagon's session (correctly) and
	// then write its own over it, so the Pentagon session was gone when the user went back.
	pentagon := plain
	pentagon.Model = "pentagon"
	other, otherSave, err := SessionFiles(pentagon)
	if err != nil {
		t.Fatalf("SessionFiles: %v", err)
	}
	if other == resume || otherSave == saveTo {
		t.Error("two models resolved to the same session file")
	}
	for _, path := range []string{other, otherSave} {
		if filepath.Base(path) != "session-pentagon"+SessionFileExt {
			t.Errorf("the Pentagon session is %q, want its own file", filepath.Base(path))
		}
	}
}

// The round trip, with a real machine: what is saved is what is restored, down to the
// tick count. This is the whole point of a session, and a fake would only be asserting
// that the fake was called.
func TestSessionRoundTrip(t *testing.T) {
	emu := emulator.New(model.Spectrum48K, &sound.NullOutput{})
	m := NewMachine(emu)
	path := filepath.Join(t.TempDir(), "session-48k"+SessionFileExt)

	for i := 0; i < 10; i++ {
		emu.RunFrame()
	}
	ticks := emu.TotalTicks()

	msg, err := SaveSessionOnExit(m, path)
	if err != nil {
		t.Fatalf("SaveSessionOnExit: %v", err)
	}
	if msg == "" {
		t.Error("the save reported nothing")
	}

	// A second machine resumes it and is at the same tick.
	other := emulator.New(model.Spectrum48K, &sound.NullOutput{})
	msg, err = RestoreSession(NewMachine(other), path)
	if err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}
	if other.TotalTicks() != ticks {
		t.Errorf("resumed at %d ticks, want %d", other.TotalTicks(), ticks)
	}
	if !strings.Contains(msg, fmt.Sprintf("%d ticks", ticks)) {
		t.Errorf("the report does not say what it restored: %q", msg)
	}
}

// A session that is not there is not a failure and not worth a line: the first run has
// none. Anything else is reported, because the user asked for a session and did not get
// one.
func TestRestoreSessionReporting(t *testing.T) {
	dir := t.TempDir()

	emu := emulator.New(model.Spectrum48K, &sound.NullOutput{})
	msg, err := RestoreSession(NewMachine(emu), filepath.Join(dir, "nothere.zxstate"))
	if err != nil {
		t.Errorf("a missing session was an error: %v", err)
	}
	if msg != "" {
		t.Errorf("a missing session produced a message: %q", msg)
	}

	// A file that is not a session at all.
	rubbish := filepath.Join(dir, "rubbish.zxstate")
	if err := os.WriteFile(rubbish, []byte("this is not a session"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreSession(NewMachine(emu), rubbish); err == nil {
		t.Error("a file that is not a session was accepted")
	}
	// And the machine was left alone: a refused restore must not half-apply.
	if emu.TotalTicks() != 0 {
		t.Errorf("the machine ran %d ticks during a refused restore", emu.TotalTicks())
	}
}

// A launch with no -model resumes the machine that was used last, which is what "say nothing
// and put me back where I was" means when the user's last machine was not the default one.
func TestLatestSession(t *testing.T) {
	store := Store{Dir: t.TempDir()}

	if _, ok := store.LatestSession(); ok {
		t.Error("a config directory with no sessions reported one")
	}

	// Two machines, saved at different times. Timestamps are set rather than slept for: the
	// answer is the newest file, and a test that sleeps to spell "newer" is a test that can
	// fail on a busy machine.
	pentagon := store.SessionPath("pentagon")
	forty8 := store.SessionPath("48k")
	for _, path := range []string{pentagon, forty8} {
		if err := os.WriteFile(path, []byte("state"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	base := time.Now()
	if err := os.Chtimes(forty8, base, base); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(pentagon, base.Add(time.Minute), base.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	if got, ok := store.LatestSession(); !ok || got != "pentagon" {
		t.Errorf("the latest session = %q (%v), want pentagon", got, ok)
	}

	// The other way round: the answer follows the timestamps, not the names.
	if err := os.Chtimes(pentagon, base, base); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(forty8, base.Add(time.Minute), base.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got, ok := store.LatestSession(); !ok || got != "48k" {
		t.Errorf("the latest session = %q (%v), want 48k", got, ok)
	}

	// A file that is not a session is not picked, and neither is a directory that matches:
	// the picker reads the directory the emulator writes to, but it should not be fooled.
	if err := os.WriteFile(filepath.Join(store.Dir, "session-backup"+SessionFileExt+".bak"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.LatestSession(); got != "48k" {
		t.Errorf("the latest session = %q, want 48k still", got)
	}
}
