package mem

import (
	"github.com/kiltum/zxgo-v3/pkg/model"
)

// NewFlat48K creates a flat 64KB memory mapper for testing.
//
// Test programs like ZEXALL and the Fuse instruction tests run on a bare-metal
// CP/M-style machine: a single contiguous 64KB of RAM with no ROM shadow and no
// paging. This returns a direct-array flat mapper (not the banked generic
// mapper), which keeps the CPU test's per-access overhead minimal.
func NewFlat48K(cfg model.Config) MemoryMapper {
	return newFlat48K(cfg)
}
