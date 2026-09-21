package main

import (
	"path/filepath"
	"testing"

	"github.com/kiltum/zxgo-v3/internal/emulator"
	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/sound"
	"github.com/kiltum/zxgo-v3/pkg/state"
)

// The CLI half of a session. Which flag supplies which path is no longer CLI
// code: it is frontend.StatePathFor, SavePath and LoadPath, tested there. What
// is left to check here is that a machine built the way the CLI builds one saves
// and resumes, and that the refusal is the one the container produces.

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
