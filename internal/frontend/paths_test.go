package frontend

import (
	"os"
	"path/filepath"
	"testing"
)

// TestStatePathForPrecedence: an explicit -load-state or -save-state is that
// half's path, and -session supplies both halves at once, so the two flags
// cannot fight over one path.
func TestStatePathForPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		explicit string
		session  string
		want     string
	}{
		{"neither", "", "", ""},
		{"session only", "", "/tmp/s.zxstate", "/tmp/s.zxstate"},
		{"explicit only", "/tmp/e.zxstate", "", "/tmp/e.zxstate"},
		{"explicit wins", "/tmp/e.zxstate", "/tmp/s.zxstate", "/tmp/e.zxstate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := StatePathFor(tc.explicit, tc.session); got != tc.want {
				t.Errorf("StatePathFor(%q, %q) = %q, want %q", tc.explicit, tc.session, got, tc.want)
			}
		})
	}
}

// TestSavePathAddsTheExtension: a path that does not name the format gets the
// extension appended, and one that already carries it (in any case) is left
// alone rather than becoming "game.zxstate.zxstate".
func TestSavePathAddsTheExtension(t *testing.T) {
	for _, tc := range []struct {
		in, ext, want string
	}{
		{"game", ".zxstate", "game.zxstate"},
		{"game.zxstate", ".zxstate", "game.zxstate"},
		{"game.ZXSTATE", ".zxstate", "game.ZXSTATE"},
		{"dir/game", ".zxstate", "dir/game.zxstate"},
		{"out.bin", ".zxstate", "out.bin.zxstate"},
		{"session", ".replay", "session.replay"},
		{"session.replay", ".replay", "session.replay"},
		{"", ".zxstate", ""},
	} {
		if got := SavePath(tc.in, tc.ext); got != tc.want {
			t.Errorf("SavePath(%q, %q) = %q, want %q", tc.in, tc.ext, got, tc.want)
		}
	}
}

// TestLoadPathPrefersTheNameGiven: a load reads the file the user named if it is
// there, whatever it is called, and falls back to the suffixed name - which is
// what a save would have written, so the two flags agree on one file.
func TestLoadPathPrefersTheNameGiven(t *testing.T) {
	dir := t.TempDir()

	// Neither exists: the suffixed name is the one reported, because that is the
	// name a save would have created and the name the user is looking for.
	absent := filepath.Join(dir, "missing")
	if got, want := LoadPath(absent, ".zxstate"), absent+".zxstate"; got != want {
		t.Errorf("LoadPath(%q) = %q, want %q", absent, got, want)
	}
	if got, want := LoadPath(absent+".zxstate", ".zxstate"), absent+".zxstate"; got != want {
		t.Errorf("LoadPath of a suffixed name = %q, want %q", got, want)
	}

	// Only the suffixed file exists: it is found from the bare name.
	suffixed := filepath.Join(dir, "game.zxstate")
	if err := os.WriteFile(suffixed, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LoadPath(filepath.Join(dir, "game"), ".zxstate"); got != suffixed {
		t.Errorf("LoadPath = %q, want %q", got, suffixed)
	}

	// A file that exists under the literal name wins, extension or not.
	literal := filepath.Join(dir, "named")
	if err := os.WriteFile(literal, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(literal+".zxstate", []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LoadPath(literal, ".zxstate"); got != literal {
		t.Errorf("LoadPath = %q, want the file the user named (%q)", got, literal)
	}

	if got := LoadPath("", ".zxstate"); got != "" {
		t.Errorf("LoadPath of an empty path = %q, want empty", got)
	}
}
