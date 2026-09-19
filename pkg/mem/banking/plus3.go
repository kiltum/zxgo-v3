package banking

import (
	"fmt"

	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/state"
)

// Plus3 implements the ZX Spectrum +2A/+3 banking scheme.
//
// It extends the 128K scheme with four ROM banks (0-3) and the 0x1FFD port.
//
// Normal mode (0x1FFD D0=0):
//
//	0x0000-0x3FFF: ROM bank 0-3, selected by 0x7FFD D4 + 0x1FFD D2.
//	0x4000-0x7FFF: RAM bank 5 (or 7 with the shadow-screen bit).
//	0x8000-0xBFFF: RAM bank 2.
//	0xC000-0xFFFF: RAM bank 0x7FFD D0-D2.
//
// Special mode (0x1FFD D0=1): the whole 64K is RAM, page-mapped by 0x1FFD
// D1-D2. 0x1FFD D3 is the floppy motor line, not a paging bit.
type Plus3 struct {
	activeROM    int  // 0-3
	ramBankSlot3 int  // 0xC000 bank (0x7FFD D0-D2)
	shadowScreen bool // 0x7FFD D3
	pagingLocked bool // 0x7FFD D5
	specialMode  bool // 0x1FFD D0
	specialWhich int  // 0x1FFD D1-D2 (special-mode page mapping)
	romBit0      int  // 0x7FFD D4 (low ROM bit)
	romBit1      int  // 0x1FFD D2 (high ROM bit)

	romLayout   *model.ROMLayout
	previousROM int
}

// plus3SpecialMaps maps 0x1FFD D1-D2 (which) to the four 16K RAM pages, in
// order 0x0000, 0x4000, 0x8000, 0xC000.
var plus3SpecialMaps = [4][4]int{
	{0, 1, 2, 3},
	{4, 5, 6, 7},
	{4, 5, 6, 3},
	{4, 7, 6, 3},
}

// NewPlus3 creates a +2A/+3 banking scheme.
func NewPlus3(layout *model.ROMLayout) *Plus3 {
	defaultROM := 0
	if layout != nil && layout.DefaultROM >= 0 {
		defaultROM = layout.DefaultROM
	}
	return &Plus3{
		activeROM:    defaultROM,
		ramBankSlot3: 0,
		romLayout:    layout,
		previousROM:  defaultROM,
	}
}

func (s *Plus3) Name() string     { return "plus3" }
func (s *Plus3) NumRAMBanks() int { return 8 }

func (s *Plus3) NumROMBanks() int {
	if s.romLayout != nil {
		return len(s.romLayout.ROMs)
	}
	return 4
}

// updateROM recomputes the active ROM bank from the two select bits.
func (s *Plus3) updateROM() {
	s.activeROM = (s.romBit1 << 1) | s.romBit0
}

func (s *Plus3) MapAddress(addr uint16) (BankType, int, uint16) {
	if s.specialMode {
		// All four 16K pages are RAM, selected by 0x1FFD D1-D2.
		switch {
		case addr < 0x4000:
			return BankRAM, plus3SpecialMaps[s.specialWhich][0], addr
		case addr < 0x8000:
			return BankRAM, plus3SpecialMaps[s.specialWhich][1], addr - 0x4000
		case addr < 0xC000:
			return BankRAM, plus3SpecialMaps[s.specialWhich][2], addr - 0x8000
		default:
			return BankRAM, plus3SpecialMaps[s.specialWhich][3], addr - 0xC000
		}
	}
	switch {
	case addr < 0x4000:
		return BankROM, s.activeROM, addr
	case addr < 0x8000:
		bank := 5
		if s.shadowScreen {
			bank = 7
		}
		return BankRAM, bank, addr - 0x4000
	case addr < 0xC000:
		return BankRAM, 2, addr - 0x8000
	default:
		return BankRAM, s.ramBankSlot3, addr - 0xC000
	}
}

// ULAScreenBank returns the RAM bank the ULA reads for the screen (bank 5, or 7
// with the shadow-screen bit). This is a hardware path independent of the CPU's
// special paging mode, so the screen stays on bank 5/7 even in special mode.
func (s *Plus3) ULAScreenBank() int {
	if s.shadowScreen {
		return 7
	}
	return 5
}

func (s *Plus3) WritePort(port uint16, value uint8) bool {
	// Both paging ports decode with A15=0 and A1=0; A14 distinguishes them.
	if (port & 0x8002) != 0 {
		return false
	}
	if (port & 0x4000) != 0 {
		// 0x7FFD
		if !s.pagingLocked {
			s.ramBankSlot3 = int(value & 0x07)
			s.shadowScreen = (value & 0x08) != 0
			s.romBit0 = int((value >> 4) & 0x01)
			s.updateROM()
			if (value & 0x20) != 0 {
				s.pagingLocked = true
			}
		}
		return true
	}

	// 0x1FFD: D0 special mode, D1-D2 special page / ROM bit 1, D3 motor.
	s.specialMode = (value & 0x01) != 0
	s.specialWhich = int((value >> 1) & 0x03)
	s.romBit1 = int((value >> 2) & 0x01)
	s.updateROM()
	return true
}

func (s *Plus3) FetchOpcode(addr uint16) int {
	if s.romLayout == nil {
		return -1
	}
	// Deactivation first (PC leaves a range).
	for _, rom := range s.romLayout.ROMs {
		if rom.BankIndex == s.activeROM && rom.Activation.DeactivatesOnPC(addr) {
			s.activeROM = s.previousROM
			return s.previousROM
		}
	}
	// Activation (PC enters a range).
	for _, rom := range s.romLayout.ROMs {
		if rom.Activation.ActivatesOnPC(addr, s.activeROM) {
			s.previousROM = s.activeROM
			s.activeROM = rom.BankIndex
			return rom.BankIndex
		}
	}
	return -1
}

func (s *Plus3) ActiveROM() int { return s.activeROM }

func (s *Plus3) IsTRDOSBank(bank int) bool {
	return s.romLayout.IsTRDOSBank(bank)
}

func (s *Plus3) SetActiveROM(bank int) {
	s.activeROM = bank
	s.previousROM = bank
}

func (s *Plus3) Reset() {
	defaultROM := 0
	if s.romLayout != nil && s.romLayout.DefaultROM >= 0 {
		defaultROM = s.romLayout.DefaultROM
	}
	s.activeROM = defaultROM
	s.ramBankSlot3 = 0
	s.shadowScreen = false
	s.pagingLocked = false
	s.specialMode = false
	s.specialWhich = 0
	s.romBit0 = 0
	s.romBit1 = 0
	s.previousROM = defaultROM
}

// Encode writes the paging state for the native state format. The +2A/+3 has
// more of it than the 128K: the special paging mode and the two ROM select bits
// are separate fields, and the active ROM is derived from them.
func (s *Plus3) Encode() []byte {
	e := state.NewEncoder()
	e.I64(int64(s.activeROM))
	e.I64(int64(s.ramBankSlot3))
	e.Bool(s.shadowScreen)
	e.Bool(s.pagingLocked)
	e.Bool(s.specialMode)
	e.I64(int64(s.specialWhich))
	e.I64(int64(s.romBit0))
	e.I64(int64(s.romBit1))
	e.I64(int64(s.previousROM))
	return e.Payload()
}

// Decode applies a blob from Encode, parsing it all before assigning.
func (s *Plus3) Decode(blob []byte) error {
	d := state.NewDecoder(blob, 0)
	activeROM := int(d.I64())
	ramBankSlot3 := int(d.I64())
	shadowScreen := d.Bool()
	pagingLocked := d.Bool()
	specialMode := d.Bool()
	specialWhich := int(d.I64())
	romBit0 := int(d.I64())
	romBit1 := int(d.I64())
	previousROM := int(d.I64())
	if err := d.Err(); err != nil {
		return fmt.Errorf("plus3 paging: %w", err)
	}
	s.activeROM = activeROM
	s.ramBankSlot3 = ramBankSlot3
	s.shadowScreen = shadowScreen
	s.pagingLocked = pagingLocked
	s.specialMode = specialMode
	s.specialWhich = specialWhich
	s.romBit0 = romBit0
	s.romBit1 = romBit1
	s.previousROM = previousROM
	return nil
}
