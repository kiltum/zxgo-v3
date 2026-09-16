package banking

import (
	"fmt"

	"github.com/kiltum/zxgo-v3/pkg/model"
)

// Standard128 implements the ZX Spectrum 128K banking scheme.
// Physical address space:
//
//	Slot 0 (0x0000-0x3FFF): ROM, selected by port 0x7FFD D4
//	Slot 1 (0x4000-0x7FFF): ALWAYS RAM bank 5 (fixed)
//	Slot 2 (0x8000-0xBFFF): ALWAYS RAM bank 2 (fixed)
//	Slot 3 (0xC000-0xFFFF): RAM bank 0-7, selected by port 0x7FFD D0-D2
//
// Port 0x7FFD:
//
//	D0-D2: RAM bank for slot 3
//	D3:    Shadow screen (bank 5 vs bank 7)
//	D4:    ROM select (0=ROM0, 1=ROM1)
//	D5:    Paging lock
type Standard128 struct {
	activeROM    int  // Currently active ROM bank (0-4)
	ramBankSlot3 int  // RAM bank mapped to 0xC000-0xFFFF (0-7)
	shadowScreen bool // true = ULA uses bank 7, false = bank 5
	pagingLocked bool // true = 0x7FFD writes ignored

	// ROM activation support
	romLayout   *model.ROMLayout // ROM descriptors for activation
	previousROM int              // For TR-DOS deactivation
}

// NewStandard128 creates a 128K banking scheme.
func NewStandard128(layout *model.ROMLayout) *Standard128 {
	defaultROM := 0
	if layout != nil && layout.DefaultROM >= 0 {
		defaultROM = layout.DefaultROM
	}
	return &Standard128{
		activeROM:    defaultROM,
		ramBankSlot3: 0,
		shadowScreen: false,
		pagingLocked: false,
		romLayout:    layout,
		previousROM:  defaultROM,
	}
}

func (s *Standard128) Name() string     { return "standard128" }
func (s *Standard128) NumRAMBanks() int { return 8 }
func (s *Standard128) NumROMBanks() int {
	if s.romLayout != nil {
		return len(s.romLayout.ROMs)
	}
	return 5 // Legacy: ROM0, ROM1, ROM48, ROM_SYS, ROM_DOS
}

func (s *Standard128) MapAddress(addr uint16) (BankType, int, uint16) {
	switch {
	case addr < 0x4000:
		// Slot 0: ROM
		return BankROM, s.activeROM, addr
	case addr < 0x8000:
		// Slot 1: RAM bank 5 (fixed)
		return BankRAM, 5, addr - 0x4000
	case addr < 0xC000:
		// Slot 2: RAM bank 2 (fixed)
		return BankRAM, 2, addr - 0x8000
	default:
		// Slot 3: RAM bank selected by ramBankSlot3
		return BankRAM, s.ramBankSlot3, addr - 0xC000
	}
}

func (s *Standard128) WritePort(port uint16, value uint8) bool {
	// Only handle port 0x7FFD (any port where low byte is 0xFD and bit 1 is set)
	if (port & 0xC002) != 0x4000 {
		return false
	}

	// Ignore if paging is locked
	if s.pagingLocked {
		return true
	}

	// D0-D2: RAM bank for slot 3
	s.ramBankSlot3 = int(value & 0x07)

	// D3: Shadow screen
	s.shadowScreen = (value & 0x08) != 0

	// D4: ROM select
	romBit := int((value >> 4) & 0x01)
	s.activeROM = romBit
	s.previousROM = romBit

	// D5: Paging lock (once set, cannot be cleared until reset)
	if (value & 0x20) != 0 {
		s.pagingLocked = true
	}

	return true
}

func (s *Standard128) FetchOpcode(addr uint16) int {
	if s.romLayout == nil {
		return -1 // No ROM layout, no activation
	}

	// Check current ROM for deactivation FIRST (before checking new activations)
	// Find the ROM descriptor for the currently active ROM by BankIndex
	for _, rom := range s.romLayout.ROMs {
		if rom.BankIndex == s.activeROM && rom.Activation.DeactivatesOnPC(addr) {
			s.activeROM = s.previousROM
			return s.previousROM
		}
	}

	// Check all ROMs for PC-based activation
	for _, rom := range s.romLayout.ROMs {
		if rom.Activation.ActivatesOnPC(addr, s.activeROM) {
			s.previousROM = s.activeROM
			s.activeROM = rom.BankIndex
			return rom.BankIndex
		}
	}

	return -1 // No change
}

func (s *Standard128) ActiveROM() int {
	return s.activeROM
}

func (s *Standard128) IsTRDOSBank(bank int) bool {
	return s.romLayout.IsTRDOSBank(bank)
}

func (s *Standard128) SetActiveROM(bank int) {
	s.activeROM = bank
	s.previousROM = bank
}

func (s *Standard128) Reset() {
	defaultROM := 0
	if s.romLayout != nil && s.romLayout.DefaultROM >= 0 {
		defaultROM = s.romLayout.DefaultROM
	}
	s.activeROM = defaultROM
	s.ramBankSlot3 = 0
	s.shadowScreen = false
	s.pagingLocked = false
	s.previousROM = defaultROM
}

type standard128Snapshot struct {
	ActiveROM    int
	RAMBankSlot3 int
	ShadowScreen bool
	PagingLocked bool
	PreviousROM  int
}

func (s *Standard128) Snapshot() interface{} {
	return &standard128Snapshot{
		ActiveROM:    s.activeROM,
		RAMBankSlot3: s.ramBankSlot3,
		ShadowScreen: s.shadowScreen,
		PagingLocked: s.pagingLocked,
		PreviousROM:  s.previousROM,
	}
}

func (s *Standard128) Restore(state interface{}) error {
	snap, ok := state.(*standard128Snapshot)
	if !ok {
		return fmt.Errorf("invalid snapshot type for Standard128")
	}
	s.activeROM = snap.ActiveROM
	s.ramBankSlot3 = snap.RAMBankSlot3
	s.shadowScreen = snap.ShadowScreen
	s.pagingLocked = snap.PagingLocked
	s.previousROM = snap.PreviousROM
	return nil
}

// ShadowScreen returns true if ULA should use RAM bank 7 instead of bank 5.
func (s *Standard128) ShadowScreen() bool {
	return s.shadowScreen
}

// ULAScreenBank returns the RAM bank the ULA reads for the screen (bank 5, or 7
// with the shadow-screen bit).
func (s *Standard128) ULAScreenBank() int {
	if s.shadowScreen {
		return 7
	}
	return 5
}
