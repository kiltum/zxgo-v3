package emulator

import (
	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/sound"
)

// New creates an emulator using the new ROM layout system.
// This is a compatibility wrapper for tests.
func New(cfg model.Config, audioOut sound.AudioOutput) *Emulator {
	// Get the ROM layout for the model; fall back to 48K for test models.
	layout := model.ROMLayoutFor(cfg.Name)
	if layout == nil {
		layout = model.Spectrum48K_ROMLayout
	}

	emu, err := NewWithLayout(cfg, layout, "roms", audioOut)
	if err != nil {
		// For tests, panic is acceptable
		panic(err)
	}
	return emu
}
