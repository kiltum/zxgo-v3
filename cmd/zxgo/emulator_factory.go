package main

import (
	"fmt"

	"github.com/kiltum/zxgo-v3/internal/emulator"
	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/sound"
)

// createEmulatorWithLayout creates an emulator using the new ROM layout system.
func createEmulatorWithLayout(cfg model.Config, audioOut sound.AudioOutput, romsDir string) (*emulator.Emulator, error) {
	// Get the ROM layout for the model (nil -> unknown model).
	layout := model.ROMLayoutFor(cfg.Name)
	if layout == nil {
		return nil, fmt.Errorf("no ROM layout defined for model: %s", cfg.Name)
	}

	// Create emulator with layout
	emu, err := emulator.NewWithLayout(cfg, layout, romsDir, audioOut)
	if err != nil {
		return nil, fmt.Errorf("creating emulator with layout: %w", err)
	}

	return emu, nil
}
