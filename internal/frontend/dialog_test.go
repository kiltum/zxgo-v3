package frontend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The whole path a tape load takes without a command line: the action asks for a
// file, the loop hands the request to the backend, the backend answers with an
// event, and the machine ends up with the tape.
//
// The load itself is the real emulator's, because the point is that the file the
// user chose is the file the machine got - and a fake would only be asserting that
// the fake was called.
func TestOpenTapeRoundTrip(t *testing.T) {
	app, m := testApp()
	backend := &fakeBackend{}

	// The user asks, from the menu or the tape window's button.
	if err := app.Dispatch(ActOpenTape); err != nil {
		t.Fatalf("Dispatch(ActOpenTape): %v", err)
	}
	if !app.DialogOpen() {
		t.Error("the app is not waiting for an answer after asking for a dialog")
	}

	// The loop takes the request and gives it to the backend in the same iteration.
	req, ok := app.TakeDialogRequest()
	if !ok {
		t.Fatal("no dialog request to hand over")
	}
	if req.Kind != DialogOpenTape {
		t.Errorf("request kind = %v, want a tape dialog", req.Kind)
	}
	if _, again := app.TakeDialogRequest(); again {
		t.Error("the same request was handed over twice")
	}
	backend.OpenFileDialog(req)

	// The user chooses a file, and the answer arrives as an event.
	app.Apply([]Event{{Kind: EvDialogResult, Tool: ToolMain, Path: "/tapes/game.tap", Dialog: DialogOpenTape}})

	if m.tapeName != "/tapes/game.tap" {
		t.Errorf("the machine loaded %q, want the file the user chose", m.tapeName)
	}
	if !m.tapeMounted {
		t.Error("the machine reports no tape after a load")
	}
	if app.DialogOpen() {
		t.Error("the app is still waiting for an answer after one arrived")
	}
	if _, ok := app.TakeDialogRequest(); ok {
		t.Error("the answer asked for another dialog")
	}
}

// A cancelled dialog does nothing at all: a user who closed it did not ask for
// anything, and loading a tape they did not choose would be a surprise.
func TestCancelledDialogDoesNothing(t *testing.T) {
	app, m := testApp()

	app.Dispatch(ActOpenTape)
	app.TakeDialogRequest()
	app.Apply([]Event{{Kind: EvDialogResult, Tool: ToolMain, Dialog: DialogOpenTape}})

	if m.tapeMounted {
		t.Error("a cancelled dialog loaded something")
	}
	if app.DialogOpen() {
		t.Error("the app is still waiting after a cancel")
	}
	if len(app.Settings().LastFile) != 0 {
		t.Errorf("a cancel was remembered as a location: %v", app.Settings().LastFile)
	}
}

// Where the user was is remembered per kind, and the next dialog opens there: a file
// that is still there is handed over as it stands, and one that has gone leaves its
// directory behind.
func TestDialogOpensWhereTheUserWas(t *testing.T) {
	dir := t.TempDir()
	tape := filepath.Join(dir, "game.tap")
	if err := os.WriteFile(tape, []byte("tape"), 0o644); err != nil {
		t.Fatal(err)
	}

	app, _ := testApp()
	app.Dispatch(ActOpenTape)
	app.TakeDialogRequest()
	app.Apply([]Event{{Kind: EvDialogResult, Tool: ToolMain, Path: tape, Dialog: DialogOpenTape}})

	if got := app.Settings().LastFile[DialogOpenTape]; got != tape {
		t.Errorf("remembered %q, want %q", got, tape)
	}

	// The next request to open a tape starts at the file itself.
	app.Dispatch(ActOpenTape)
	req, _ := app.TakeDialogRequest()
	if req.Default != tape {
		t.Errorf("the next dialog opens at %q, want %q", req.Default, tape)
	}
	app.Apply([]Event{{Kind: EvDialogResult, Tool: ToolMain, Dialog: DialogOpenTape}})

	// And when the file has gone, at the directory it was in: a directory that still
	// exists is a better answer than a file that does not.
	if err := os.Remove(tape); err != nil {
		t.Fatal(err)
	}
	app.Dispatch(ActOpenTape)
	req, _ = app.TakeDialogRequest()
	if req.Default != dir {
		t.Errorf("the dialog opens at %q after the file went, want the directory %q", req.Default, dir)
	}
}

// A dialog that is up holds the keyboard back (D5 rule 10): the user is typing a
// filename into an OS window, and the machine must not hear it.
func TestKeyboardIsHeldBackWhileADialogIsUp(t *testing.T) {
	app, m := testApp()
	app.Dispatch(ActOpenTape)

	app.Apply([]Event{{Kind: EvKey, Tool: ToolMain, Key: KeyA, Down: true, DialogOpen: true}})
	if len(m.pressed) != 0 {
		t.Error("the machine heard a key typed into a file dialog")
	}

	// The three actions of rule 3 stay live through it, because they are the ones a
	// user may legitimately want while a dialog is in front.
	if act, ok := app.HandleKey(ToolMain, KeyF5, 0, true); !ok || act != ActPauseToggle {
		t.Errorf("pause under a dialog = %v (%v), want it to fire", act, ok)
	}

	// And once the answer arrives, the keyboard is the machine's again.
	app.Apply([]Event{{Kind: EvDialogResult, Tool: ToolMain, Dialog: DialogOpenTape}})
	app.Apply([]Event{{Kind: EvKey, Tool: ToolMain, Key: KeyA, Down: true}})
	if len(m.pressed) != 1 {
		t.Errorf("the machine did not hear a key after the dialog closed: %v", m.pressed)
	}
}

// The settings round-trip through the store, and the file names a kind by string so
// that adding a dialog does not invalidate it.
func TestSettingsRoundTrip(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	app, _ := testApp()
	app.SetSettings(Settings{LastFile: map[DialogKind]string{DialogOpenTape: "/tapes/game.tap"}})

	if err := store.SaveSettings(app.Settings()); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	data, err := os.ReadFile(store.SettingsPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"tape": "/tapes/game.tap"`) {
		t.Errorf("the settings file does not name the kind by name:\n%s", data)
	}

	loaded, err := store.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	restarted, _ := testApp()
	restarted.SetSettings(loaded)
	if got := restarted.Settings().LastFile[DialogOpenTape]; got != "/tapes/game.tap" {
		t.Errorf("the remembered location came back as %q", got)
	}
}

// A settings file that cannot be read is reported and ignored, and one naming a kind
// this build does not know keeps the rest (D3).
func TestLoadSettingsToleratesUnknownAndRubbish(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir}

	if set, err := store.LoadSettings(); err != nil || len(set.LastFile) != 0 {
		t.Errorf("a missing settings file = %+v, %v; want nothing and no error", set, err)
	}

	// One good kind and one from a version that had a dialog this build has not.
	body := `{"version":1,"last_file":{"tape":"/tapes/game.tap","printer":"/tmp/out"}}`
	if err := os.WriteFile(store.SettingsPath(), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	set, err := store.LoadSettings()
	if err == nil {
		t.Error("an unknown kind was accepted silently")
	}
	if set.LastFile[DialogOpenTape] != "/tapes/game.tap" {
		t.Errorf("the known entry was dropped with the unknown one: %+v", set)
	}
	// The report names the entry and what it was going to be used for: a bare key can
	// be useless to the person reading it.
	if !strings.Contains(err.Error(), "printer: /tmp/out") {
		t.Errorf("the report does not name the entry it ignored: %v", err)
	}

	if err := os.WriteFile(store.SettingsPath(), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadSettings(); err == nil {
		t.Error("a settings file that cannot be read was accepted")
	}
}

// The settings are written when they change and not on every tick, which is the same
// rule the layout follows.
func TestSettingsAreSavedOnChangeOnly(t *testing.T) {
	app, _ := testApp()
	backend := &fakeBackend{
		batches: [][]Event{
			nil,
			{{Kind: EvDialogResult, Tool: ToolMain, Path: "/tapes/game.tap", Dialog: DialogOpenTape}},
			nil,
			nil,
		},
	}

	var saves []Settings
	loop := &BackendLoop{
		App:             app,
		Backend:         backend,
		SettingsChanged: func(s Settings) { saves = append(saves, s) },
	}
	loop.Run()

	if len(saves) != 1 {
		t.Fatalf("saved the settings %d times, want once (the change)", len(saves))
	}
	if saves[0].LastFile[DialogOpenTape] != "/tapes/game.tap" {
		t.Errorf("saved %+v, want the location that changed", saves[0])
	}
}

// The loop is what carries the request to the backend: the front end says what it
// wants, and the backend - which owns the OS's windows - opens the dialog. Without
// this hop the action would set a flag that nothing acted on, which is exactly the
// kind of wiring a unit test on App alone cannot see.
func TestLoopHandsTheDialogRequestToTheBackend(t *testing.T) {
	app, _ := testApp()
	backend := &fakeBackend{
		batches: [][]Event{
			{{Kind: EvAction, Action: ActOpenTape}},
			nil,
		},
	}

	(&BackendLoop{App: app, Backend: backend}).Run()

	if len(backend.dialogs) != 1 {
		t.Fatalf("the backend was asked for %d dialogs, want 1", len(backend.dialogs))
	}
	if backend.dialogs[0].Kind != DialogOpenTape {
		t.Errorf("asked for %v, want a tape dialog", backend.dialogs[0].Kind)
	}
	// An action that only fires once, from the menubar or a button: a request handed
	// over twice would open two dialogs.
	if len(backend.dialogs) != 1 {
		t.Error("the same request was handed over more than once")
	}
}

// A disk is mounted from a path the user chose, by the same machinery as a tape: the
// action asks, the backend answers, and the answer is what mounts it.
func TestOpenDiskRoundTrip(t *testing.T) {
	app, m := testApp()

	app.Dispatch(ActOpenDisk)
	req, ok := app.TakeDialogRequest()
	if !ok {
		t.Fatal("no dialog request to hand over")
	}
	if req.Kind != DialogOpenDisk {
		t.Fatalf("request kind = %v, want a disk dialog", req.Kind)
	}

	app.Apply([]Event{{Kind: EvDialogResult, Tool: ToolMain, Path: "/disks/game.trd", Dialog: DialogOpenDisk}})

	if m.diskPath != "/disks/game.trd" {
		t.Errorf("mounted %q, want the file the user chose", m.diskPath)
	}
	if !m.diskMounted {
		t.Error("the machine reports no disk after a mount")
	}
	if app.DialogOpen() {
		t.Error("the app is still waiting for an answer after one arrived")
	}

	// And eject takes it out again.
	app.Dispatch(ActEjectDisk)
	if m.diskMounted {
		t.Error("eject left the disk in the drive")
	}
	if m.diskPath != "" {
		t.Errorf("disk path = %q after an eject", m.diskPath)
	}
}

// The disks window's state is refreshed while that window is open and not before: a
// controller snapshot is cheap but pointless for a window nobody is looking at.
func TestDiskViewIsRefreshedWhileTheWindowIsOpen(t *testing.T) {
	app, m := testApp()
	m.hasFDC, m.diskMounted, m.diskPath = true, true, "/disks/game.trd"

	app.Tick(time.Now())
	if app.Views().Disks.HasFDC {
		t.Error("the disks view was refreshed with the window closed")
	}

	app.OpenTool(ToolDisks)
	app.Tick(time.Now())

	view := app.Views().Disks
	if !view.HasFDC {
		t.Error("the disks view was not refreshed with the window open")
	}
	if !view.Mounted || view.Path != "/disks/game.trd" {
		t.Errorf("the view has %+v, want the mounted disk and its path", view)
	}
	if view.FDC.Controller == "" {
		t.Error("the view carries no controller state")
	}
}

// The controller timing switch is live and reads back, so the disks window can spell
// its own state (D10 lists the setting as live).
func TestFDCTimingToggle(t *testing.T) {
	app, m := testApp()

	if m.NoFDCTiming() {
		t.Fatal("a fresh machine reports the timing switch on")
	}

	app.Dispatch(ActFDCTimingToggle)
	if !m.NoFDCTiming() {
		t.Error("the timing switch did not turn on")
	}
	app.Dispatch(ActFDCTimingToggle)
	if m.NoFDCTiming() {
		t.Error("the timing switch did not turn off")
	}
}

// A settings file carrying an entry this build cannot name heals itself: the entry is
// reported and ignored on load, and the next save writes the file without it.
//
// This is not hypothetical: a disk location was written under the fallback name
// "unknown" because the enum's String had not learned about the disk dialog, so every
// launch reported it and every save rewrote it. The store now refuses to write an
// unnameable kind, so the first save after the fix drops it.
func TestASettingsFileWithAnUnknownEntryHeals(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	body := `{"version":1,"last_file":{
	  "tape":"/tapes/game.tap",
	  "unknown":"/disks/game.trd"
	}}`
	if err := os.WriteFile(store.SettingsPath(), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.LoadSettings()
	if err == nil {
		t.Fatal("an entry that cannot be named was accepted silently")
	}
	if !strings.Contains(err.Error(), "unknown: /disks/game.trd") {
		t.Errorf("the report does not identify the entry: %v", err)
	}

	// The app takes what it could read, and the next save writes that.
	app, _ := testApp()
	app.SetSettings(loaded)
	app.Dispatch(ActOpenDisk)
	app.TakeDialogRequest()
	app.Apply([]Event{{Kind: EvDialogResult, Tool: ToolMain,
		Path: "/disks/other.trd", Dialog: DialogOpenDisk}})

	if err := store.SaveSettings(app.Settings()); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	data, err := os.ReadFile(store.SettingsPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "unknown") {
		t.Errorf("the unnameable entry was written again:\n%s", data)
	}
	// And both real kinds are there, named by name.
	for _, want := range []string{`"tape": "/tapes/game.tap"`, `"disk": "/disks/other.trd"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("the saved file is missing %s:\n%s", want, data)
		}
	}
}

// The whole path a snapshot save takes: the action asks for a name, the backend's save
// dialog answers with one, and the machine is written to it.
//
// The save is the real one - the fake builds a plausible SNA and the front end writes
// it - because what is being checked is that the file the user named is the file that
// appears, with the extension a save would have given it.
func TestSaveSnapshotRoundTrip(t *testing.T) {
	dir := t.TempDir()
	app, m := testApp()

	app.Dispatch(ActSaveSnapshot)
	req, ok := app.TakeDialogRequest()
	if !ok {
		t.Fatal("no dialog request to hand over")
	}
	if req.Kind != DialogSaveSnapshot {
		t.Fatalf("request kind = %v, want a save dialog", req.Kind)
	}
	if !req.Kind.IsSave() {
		t.Error("the save kind does not say it is a save")
	}

	// The user names a file without the extension, and gets one: a file whose name does
	// not say what is in it is worse than one that does.
	app.Apply([]Event{{Kind: EvDialogResult, Tool: ToolMain,
		Path: filepath.Join(dir, "game"), Dialog: DialogSaveSnapshot}})

	if m.madeSnapshots != 1 {
		t.Errorf("the machine was asked for %d snapshots, want 1", m.madeSnapshots)
	}
	path := filepath.Join(dir, "game"+SnapshotExt)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the snapshot was not written where it was named: %v", err)
	}
	// And the location is remembered as the file that was written.
	if got := app.Settings().LastFile[DialogSaveSnapshot]; got != path {
		t.Errorf("remembered %q, want the file that was written (%q)", got, path)
	}
}

// A snapshot loads through the same dialog as the media, and a cancelled one does
// nothing.
func TestOpenSnapshotRoundTrip(t *testing.T) {
	app, m := testApp()

	app.Dispatch(ActOpenSnapshot)
	req, _ := app.TakeDialogRequest()
	if req.Kind != DialogOpenSnapshot {
		t.Fatalf("request kind = %v, want a snapshot dialog", req.Kind)
	}
	if req.Kind.IsSave() {
		t.Error("the open kind says it is a save")
	}

	app.Apply([]Event{{Kind: EvDialogResult, Tool: ToolMain,
		Path: "/snaps/game.sna", Dialog: DialogOpenSnapshot}})
	if m.loadedSnapshot != "/snaps/game.sna" {
		t.Errorf("loaded %q, want the file the user chose", m.loadedSnapshot)
	}

	app.Dispatch(ActOpenSnapshot)
	app.TakeDialogRequest()
	app.Apply([]Event{{Kind: EvDialogResult, Tool: ToolMain, Dialog: DialogOpenSnapshot}})
	if m.loadedSnapshot != "/snaps/game.sna" {
		t.Error("a cancelled open loaded something")
	}
}

// The machine class is the message's own words, and every kind of snapshot names
// itself: a Pentagon is not "128k".
func TestSnapshotClass(t *testing.T) {
	for _, tc := range []struct {
		info SnapshotInfo
		want string
	}{
		{SnapshotInfo{}, "48k"},
		{SnapshotInfo{Is128K: true, HardwareMode: 3}, "128k"},
		{SnapshotInfo{Is128K: true, HardwareMode: 7}, "+3"},
		{SnapshotInfo{Is128K: true, HardwareMode: 9}, "pentagon"},
	} {
		if got := SnapshotClass(tc.info); got != tc.want {
			t.Errorf("SnapshotClass(%+v) = %q, want %q", tc.info, got, tc.want)
		}
	}
}

// The loop picks the backing call from the kind: the open dialog for the kinds that
// pick an existing file, the save dialog for the one that names a new one.
func TestLoopPicksTheRightDialog(t *testing.T) {
	app, _ := testApp()
	backend := &fakeBackend{
		batches: [][]Event{
			{{Kind: EvAction, Action: ActOpenSnapshot}},
			{{Kind: EvAction, Action: ActSaveSnapshot}},
			nil,
		},
	}

	(&BackendLoop{App: app, Backend: backend}).Run()

	if len(backend.dialogs) != 1 || backend.dialogs[0].Kind != DialogOpenSnapshot {
		t.Errorf("open dialogs = %+v, want one snapshot open", backend.dialogs)
	}
	if len(backend.saveDialogs) != 1 || backend.saveDialogs[0].Kind != DialogSaveSnapshot {
		t.Errorf("save dialogs = %+v, want one snapshot save", backend.saveDialogs)
	}
}
