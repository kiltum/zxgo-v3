package frontend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A rebind survives a restart (M3's first *done when*): a file written by the
// bindings editor is read at startup and the new key does what the user said.
//
// The editor itself is M5. What is tested here is everything it depends on - the
// file, the merge over the defaults, and the dispatch that follows - which is the
// half of the feature that has no window in it.
func TestRebindPersistsAcrossRestarts(t *testing.T) {
	store := Store{Dir: t.TempDir()}

	// What the editor does: take the table in force, change one binding, and write
	// the difference against the defaults.
	app, m := testApp()
	table := app.Keymap()
	delete(table, Binding{Key: KeyF5})          // the user takes pause away from F5
	table[Binding{Key: KeyF7}] = ActPauseToggle // and puts it on F7
	if err := store.SaveKeymap(DiffBindings(DefaultBindings(), table)); err != nil {
		t.Fatalf("SaveKeymap: %v", err)
	}

	// What a restart does.
	restarted, m2 := testApp()
	user, err := store.LoadKeymap()
	if err != nil {
		t.Fatalf("LoadKeymap: %v", err)
	}
	restarted.SetKeymap(Merge(DefaultBindings(), user))

	// F7 pauses now...
	if act, ok := restarted.HandleKey(ToolMain, KeyF7, 0, true); !ok || act != ActPauseToggle {
		t.Fatalf("F7 = %v (%v), want the pause action", act, ok)
	}
	if restarted.RunState() != StatePaused {
		t.Error("the rebound key did not pause the machine")
	}
	// ...and F5 does nothing, which is the half a Merge alone would get wrong: a
	// binding taken away has to stay taken away.
	if act, ok := restarted.HandleKey(ToolMain, KeyF5, 0, true); ok {
		t.Errorf("F5 still fires %v after it was unbound", act)
	}
	if len(m.pressed) != 0 || len(m2.pressed) != 0 {
		t.Error("a host key reached the machine")
	}
}

// The file holds the user's changes, not a copy of the table: an entry that is back
// to its default is left out, so a later version that binds a new action gives the
// user that binding rather than nothing.
func TestKeymapFileHoldsOnlyTheChanges(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	app, _ := testApp()

	// Nothing changed at all: the file is empty of bindings.
	if err := store.SaveKeymap(DiffBindings(DefaultBindings(), app.Keymap())); err != nil {
		t.Fatalf("SaveKeymap: %v", err)
	}
	data, err := os.ReadFile(store.KeymapPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"bindings": {}`) {
		t.Errorf("an unchanged keymap wrote entries:\n%s", data)
	}

	// One change: one entry.
	table := app.Keymap()
	table[Binding{Key: KeyF7}] = ActReset
	if err := store.SaveKeymap(DiffBindings(DefaultBindings(), table)); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(store.KeymapPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"f7": "reset"`) {
		t.Errorf("the change is not in the file:\n%s", data)
	}
	if strings.Contains(string(data), "quit") {
		t.Errorf("an unchanged binding was written down:\n%s", data)
	}
}

// A file written by hand is read, which is what makes the format a format rather
// than an implementation detail: the strings are the ones ParseBinding and
// ParseAction accept.
func TestLoadKeymapFromHandWrittenFile(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir}
	body := `{
	  "version": 1,
	  "bindings": {
	    "cmd+shift+p": "tape-playpause",
	    "f7": "screenshot",
	    "cmd+q": "none"
	  }
	}`
	if err := os.WriteFile(store.KeymapPath(), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	user, err := store.LoadKeymap()
	if err != nil {
		t.Fatalf("LoadKeymap: %v", err)
	}
	app, _ := testApp()
	app.SetKeymap(Merge(DefaultBindings(), user))

	if act, ok := app.HandleKey(ToolMain, KeyP, ModSuper|ModShift, true); !ok || act != ActTapePlayPause {
		t.Errorf("cmd+shift+p = %v (%v), want tape play/pause", act, ok)
	}
	if act, ok := app.HandleKey(ToolMain, KeyF7, 0, true); !ok || act != ActScreenshot {
		t.Errorf("f7 = %v (%v), want screenshot", act, ok)
	}
	// And the default that was explicitly unbound is gone.
	if act, ok := app.HandleKey(ToolMain, KeyQ, ModSuper, true); ok {
		t.Errorf("cmd+q still fires %v after the file unbound it", act)
	}
}

// A partly unreadable file gives up what it could and names the rest: a typo in one
// binding must not take the other twenty with it (D3).
func TestLoadKeymapReportsBadEntriesAndKeepsGoodOnes(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	body := `{
	  "version": 1,
	  "bindings": {
	    "f7": "screenshot",
	    "hyper+q": "quit",
	    "f8": "no-such-action"
	  }
	}`
	if err := os.WriteFile(store.KeymapPath(), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	user, err := store.LoadKeymap()
	if err == nil {
		t.Fatal("a file with entries that cannot be read was accepted silently")
	}
	if !strings.Contains(err.Error(), "hyper+q") || !strings.Contains(err.Error(), "no-such-action") {
		t.Errorf("the report does not name what was skipped: %v", err)
	}
	if len(user) != 1 {
		t.Errorf("read %d entries, want just the one that was understood: %v", len(user), user)
	}
	if act, _ := user[Binding{Key: KeyF7}]; act != ActScreenshot {
		t.Errorf("the good entry is %v, want screenshot", act)
	}
}

// A file that cannot be read at all is reported and ignored, never fatal, and a
// missing one is simply a user who has never rebound anything.
func TestLoadKeymapRejectsRubbish(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir}

	if km, err := store.LoadKeymap(); err != nil || km != nil {
		t.Errorf("a missing keymap file = %v, %v; want nothing and no error", km, err)
	}

	for _, tc := range []struct{ name, body string }{
		{"not json", "bindings: f7"},
		{"from the future", `{"version":99,"bindings":{}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(dir, "keymap.json"), []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := store.LoadKeymap(); err == nil {
				t.Error("a keymap file that cannot be read was accepted")
			}
		})
	}
}

// DiffBindings is what the editor writes: everything that differs from the defaults,
// including a default the user took away.
func TestDiffBindings(t *testing.T) {
	base := Keymap{
		{Key: KeyF5}:                ActPauseToggle,
		{Key: KeyQ, Mods: ModSuper}: ActQuit,
	}
	current := Keymap{
		{Key: KeyF5}:                ActPauseToggle, // unchanged: not in the diff
		{Key: KeyF7}:                ActPauseToggle, // added
		{Key: KeyR, Mods: ModSuper}: ActReset,       // added
	}

	diff := DiffBindings(base, current)

	if _, ok := diff[Binding{Key: KeyF5}]; ok {
		t.Error("an unchanged binding is in the diff")
	}
	if act := diff[Binding{Key: KeyF7}]; act != ActPauseToggle {
		t.Errorf("the added binding is %v", act)
	}
	if act, ok := diff[Binding{Key: KeyQ, Mods: ModSuper}]; !ok || act != ActNone {
		t.Errorf("the removed binding is %v (%v), want an explicit unbinding", act, ok)
	}

	// And applying the diff to the defaults gives back the table it came from.
	//
	// "Gives back" is asked of Lookup rather than of the map: an unbinding is stored
	// as ActNone so that the file records the user's intent, and Lookup is what turns
	// that into "no action", which is the property that matters. A test on map
	// membership would be testing the representation.
	got := Merge(base, diff)
	for binding, want := range current {
		if act, ok := got.Lookup(binding); !ok || act != want {
			t.Errorf("%v = %v (%v), want %v", binding, act, ok, want)
		}
	}
	if act, ok := got.Lookup(Binding{Key: KeyQ, Mods: ModSuper}); ok {
		t.Errorf("a binding the user removed came back through the diff as %v", act)
	}
	// The round trip is exact: diffing the result against the base gives the same
	// file back, which is what makes saving twice the same file.
	if again := DiffBindings(base, got); len(again) != len(diff) {
		t.Errorf("re-diffing gave %d entries, want %d", len(again), len(diff))
	}
}

// A rebind made by editing the file shows up where the user looks for it: the menubar's
// shortcut column, which is read from the table in force. This is what MANUAL.md tells a
// user to check after a restart, so it is the claim that has to hold.
func TestAFileRebindShowsInTheMenu(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	// The example out of MANUAL.md, as a user would type it - including the second entry
	// that makes moving a key a *move*: the file is merged over the defaults, so binding
	// the new key without unbinding the old one would leave both working.
	body := `{
	  "version": 1,
	  "bindings": {
	    "f5": "none",
	    "f7": "reset",
	    "ctrl+p": "pause-toggle",
	    "cmd+q": "none"
	  }
	}`
	if err := os.WriteFile(store.KeymapPath(), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	user, err := store.LoadKeymap()
	if err != nil {
		t.Fatalf("LoadKeymap: %v", err)
	}
	app, _ := testApp()
	app.SetKeymap(Merge(DefaultBindings(), user))

	shown := map[Action]string{}
	for _, item := range menuItems(app.Menu(), "Machine") {
		shown[item.Action] = item.Shortcut
	}
	if got := shown[ActPauseToggle]; got != "ctrl+p" {
		t.Errorf("the menubar shows pause as %q, want ctrl+p", got)
	}
	if got := shown[ActReset]; got != "f7" {
		t.Errorf("the menubar shows reset as %q, want f7", got)
	}

	// And the key does what the menu says.
	if act, ok := app.HandleKey(ToolMain, KeyP, ModCtrl, true); !ok || act != ActPauseToggle {
		t.Errorf("ctrl+p = %v (%v), want the pause action", act, ok)
	}
	if app.RunState() != StatePaused {
		t.Error("the rebound key did not pause the machine")
	}

	// The unbinding the file asked for: cmd+q is gone from the menu as well as the
	// keyboard.
	for _, item := range menuItems(app.Menu(), "File") {
		if item.Action == ActQuit && item.Shortcut != "" {
			t.Errorf("quit still advertises %q after the file unbound it", item.Shortcut)
		}
	}
	if act, ok := app.HandleKey(ToolMain, KeyQ, ModSuper, true); ok {
		t.Errorf("cmd+q still fires %v after the file unbound it", act)
	}

	// The other half of a move, and the half a reader is most likely to get wrong: f5
	// does nothing after the file unbound it, and the menu stops advertising it.
	if act, ok := app.HandleKey(ToolMain, KeyF5, 0, true); ok {
		t.Errorf("f5 still fires %v after the file unbound it", act)
	}
	if got := shown[ActPauseToggle]; got != "ctrl+p" {
		t.Errorf("the menubar shows pause as %q after the move", got)
	}
}
