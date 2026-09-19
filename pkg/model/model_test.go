package model

import "testing"

// TestRegistryKeysMatchConfig is the guard on Config.Key: the map key in
// AllModels and the Key field are the same string in two places, and a state
// file or a replay recorded with one has to be recognised by the other. A
// mismatch would show up as a session that refuses to load on the machine it
// was saved from, so it fails here instead.
func TestRegistryKeysMatchConfig(t *testing.T) {
	if len(AllModels) == 0 {
		t.Fatal("AllModels is empty")
	}
	for key, cfg := range AllModels {
		if cfg.Key == "" {
			t.Errorf("model %q has no Key", key)
			continue
		}
		if cfg.Key != key {
			t.Errorf("model %q has Key %q: the registry key and the field must agree", key, cfg.Key)
		}
		if cfg.Name == "" {
			t.Errorf("model %q has no Name", key)
		}
	}
}

// TestModelNamesAreUnique pins the other half of the identity: ROMLayoutFor and
// the mapper are both chosen by Name, so two models sharing one would silently
// get each other's layout.
func TestModelNamesAreUnique(t *testing.T) {
	seen := make(map[string]string, len(AllModels))
	for key, cfg := range AllModels {
		if other, dup := seen[cfg.Name]; dup {
			t.Errorf("models %q and %q share the name %q", other, key, cfg.Name)
		}
		seen[cfg.Name] = key
	}
}

// TestEveryModelHasALayout: an emulator cannot be built without one.
func TestEveryModelHasALayout(t *testing.T) {
	for key, cfg := range AllModels {
		if ROMLayoutFor(cfg.Name) == nil {
			t.Errorf("model %q (%s) has no ROM layout", key, cfg.Name)
		}
	}
}

// TestDerivedTiming checks the geometry the ULA and the models depend on.
func TestDerivedTiming(t *testing.T) {
	for key, cfg := range AllModels {
		cpl := cfg.ClockPerLine()
		if cpl <= 0 {
			t.Errorf("model %q: ClockPerLine = %d", key, cpl)
		}
		if cef := cfg.ClockEndFrame(); cef <= 0 {
			t.Errorf("model %q: ClockEndFrame = %d", key, cef)
		}
		if w, h := cfg.FramebufferWidth(), cfg.FramebufferHeight(); w <= 0 || h <= 0 {
			t.Errorf("model %q: framebuffer %dx%d", key, w, h)
		}
	}
}
