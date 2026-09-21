package frontend

import (
	"os"
	"path/filepath"
	"testing"
)

// resolveMedia takes a recorded path as it stands, then beside the replay file,
// which is where a session someone sent you usually keeps its tape.
func TestResolveMedia(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "game.tzx"), []byte("tape"), 0o644); err != nil {
		t.Fatal(err)
	}
	replayPath := filepath.Join(dir, "session.replay")

	// Beside the replay file: found.
	var missing []string
	if got := ResolveMedia("game.tzx", replayPath, &missing); got != filepath.Join(dir, "game.tzx") {
		t.Errorf("got %q, want the copy beside the replay", got)
	}
	if len(missing) != 0 {
		t.Errorf("reported missing: %v", missing)
	}

	// Nowhere: reported, and the caller carries on with no media rather than
	// refusing to play the keys.
	if got := ResolveMedia("absent.tzx", replayPath, &missing); got != "" {
		t.Errorf("got %q, want empty for a missing path", got)
	}
	if len(missing) != 1 || missing[0] != "absent.tzx" {
		t.Errorf("missing = %v, want [absent.tzx]", missing)
	}

	// Nothing recorded is not a missing file.
	if got := ResolveMedia("", replayPath, &missing); got != "" {
		t.Errorf("got %q, want empty for an unrecorded medium", got)
	}
	if len(missing) != 1 {
		t.Errorf("missing = %v, want the earlier entry only", missing)
	}

	// An absolute path that exists is used as given, not looked up beside.
	abs := filepath.Join(dir, "game.tzx")
	if got := ResolveMedia(abs, filepath.Join(t.TempDir(), "other.replay"), &missing); got != abs {
		t.Errorf("got %q, want the absolute path %q", got, abs)
	}
}

// A replay that names no media still plays: the missing-file warning must not
// turn into a failure to start.
func TestLoadReplayRejectsForeignFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notareplay.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadReplay(path); err == nil {
		t.Fatal("LoadReplay accepted a file that is not a replay")
	}
	if _, err := LoadReplay(filepath.Join(t.TempDir(), "nope.replay")); err == nil {
		t.Fatal("LoadReplay accepted a missing file")
	}
}
