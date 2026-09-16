package mem

import (
	"fmt"

	"github.com/kiltum/zxgo-v3/pkg/mem/banking"
	"github.com/kiltum/zxgo-v3/pkg/model"
)

// NewMapperWithLayout creates a memory mapper for any model using the new banking system.
// This is the universal constructor.
func NewMapperWithLayout(cfg model.Config, layout *model.ROMLayout, romsDir string) (MemoryMapper, error) {
	// Determine banking scheme based on model
	var scheme banking.BankingScheme

	switch cfg.PagingModel {
	case "none":
		// 48K model
		scheme = banking.NewStandard128(layout)
	case "128k":
		// 128K model
		scheme = banking.NewStandard128(layout)
	case "2a3":
		// +2A/+3 model
		scheme = banking.NewPlus3(layout)
	case "pentagon512":
		// Pentagon 512K model
		scheme = banking.NewPentagon512(layout)
	default:
		return nil, fmt.Errorf("unsupported paging model: %s", cfg.PagingModel)
	}

	// Create mapper
	mapper := NewGenericMapperWithLayout(cfg, scheme, layout)

	// Load ROMs
	if layout != nil {
		romData, err := LoadROMLayout(layout, romsDir)
		if err != nil {
			return nil, fmt.Errorf("loading ROM layout: %w", err)
		}

		for bankIdx, data := range romData {
			if err := mapper.LoadROM(bankIdx, data); err != nil {
				return nil, fmt.Errorf("loading ROM bank %d: %w", bankIdx, err)
			}
		}
	}

	return mapper, nil
}
