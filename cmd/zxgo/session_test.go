package main

import (
	"path/filepath"
	"testing"

	"github.com/kiltum/zxgo-v3/internal/emulator"
	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/sound"
	"github.com/kiltum/zxgo-v3/pkg/state"
)

// The CLI half of a session: which flag supplies which path, and that the file
// the emulator writes is the file it reads back. The GUI loop is not exercised
// here - the emulator-level tests cover the state itself and the flags are what
// this file can check without a window.

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
			if got := statePathFor(tc.explicit, tc.session); got != tc.want {
				t.Errorf("statePathFor(%q, %q) = %q, want %q", tc.explicit, tc.session, got, tc.want)
			}
		})
	}
}

// TestSessionFileRoundTripThroughTheCLINames: the emulator the CLI builds saves
// and resumes, and the refusal the CLI prints its message from is the one the
// container produces.
func TestSessionFileRoundTripThroughTheCLINames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.zxstate")

	cfg := model.AllModels["48k"]
	original, err := emulator.NewFromModel(cfg, "roms", &sound.NullOutput{})
	if err != nil {
		t.Fatalf("building the machine: %v", err)
	}
	for i := 0; i < 10; i++ {
		original.RunFrame()
	}
	if err := original.SaveSession(path, state.Options{Deflate: true}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	resumed, err := emulator.NewFromModel(cfg, "roms", &sound.NullOutput{})
	if err != nil {
		t.Fatal(err)
	}
	info, err := resumed.LoadSession(path)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if info.Model != cfg.Key {
		t.Errorf("session names %s, want %s", info.Model, cfg.Key)
	}
	if info.Ticks != original.TotalTicks() {
		t.Errorf("resumed at %d ticks, want %d", info.Ticks, original.TotalTicks())
	}

	// The refusal the CLI reports for a -model that does not match.
	other, err := emulator.NewFromModel(model.AllModels["128k"], "roms", &sound.NullOutput{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.LoadSession(path); err == nil {
		t.Fatal("a 128K machine resumed a 48K session")
	} else if got := err.Error(); got == "" {
		t.Error("the refusal carries no reason")
	}
}
