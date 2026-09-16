package emulator

import (
	"fmt"

	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/sound"
)

// NewFromModel resolves a model's ROM layout and builds an emulator, loading
// ROMs from romsDir. audioOut is typically sound.NullOutput for a headless
// worker (no SDL3).
func NewFromModel(cfg model.Config, romsDir string, audioOut sound.AudioOutput) (*Emulator, error) {
	layout := model.ROMLayoutFor(cfg.Name)
	if layout == nil {
		return nil, fmt.Errorf("no ROM layout defined for model: %s", cfg.Name)
	}
	return NewWithLayout(cfg, layout, romsDir, audioOut)
}
