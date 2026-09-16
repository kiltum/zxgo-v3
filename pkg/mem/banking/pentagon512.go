package banking

import "github.com/kiltum/zxgo-v3/pkg/model"

// Pentagon512 is the Pentagon 512K banking scheme. It shares the 128K memory
// layout and the 0x7FFD port, but the RAM page is 5 bits (0-31): bits 0-2 plus
// bits 6-7 shifted down to bits 3-4 (Fuse pentagon512.c pentagon_memory_map).
// This lets 512K demos page through all 32 banks via the same port.
type Pentagon512 struct {
	*Standard128
}

// NewPentagon512 creates a Pentagon 512 banking scheme.
func NewPentagon512(layout *model.ROMLayout) *Pentagon512 {
	return &Pentagon512{Standard128: NewStandard128(layout)}
}

// Name returns the scheme identifier.
func (p *Pentagon512) Name() string { return "pentagon512" }

// NumRAMBanks returns 32 (512K of RAM in 16K banks).
func (p *Pentagon512) NumRAMBanks() int { return 32 }

// WritePort handles the 0x7FFD paging port with the 5-bit page decode.
func (p *Pentagon512) WritePort(port uint16, value uint8) bool {
	if (port & 0xC002) != 0x4000 {
		return false
	}
	if p.pagingLocked {
		return true
	}

	// 5-bit RAM page for 0xC000: bits 0-2 + bits 6-7 shifted to bits 3-4.
	p.ramBankSlot3 = int(value&0x07) | int((value&0xC0)>>3)
	p.shadowScreen = (value & 0x08) != 0
	romBit := int((value >> 4) & 0x01)
	p.activeROM = romBit
	p.previousROM = romBit
	if (value & 0x20) != 0 {
		p.pagingLocked = true
	}
	return true
}
