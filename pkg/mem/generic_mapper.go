package mem

import (
	"fmt"

	"github.com/kiltum/zxgo-v3/pkg/io_ports"
	"github.com/kiltum/zxgo-v3/pkg/mem/banking"
	"github.com/kiltum/zxgo-v3/pkg/model"
)

// genericMapper is a MemoryMapper that uses a BankingScheme for flexibility.
// It replaces the model-specific mappers (model128K, model2A3) with a unified implementation.
type genericMapper struct {
	cfg    model.Config
	scheme banking.BankingScheme

	roms [][]byte // ROM banks (indexed by bank number)
	rams [][]byte // RAM banks (indexed by bank number)

	romWrite bool // true during ROM loading

	// trdos is the shared TR-DOS arbitration state (per emulator), updated on ROM
	// switches so the Kempston joystick stands down while TR-DOS is paged in.
	trdos *io_ports.TRDosState

	// paging7FFD is the last byte written to 0x7FFD, which is what an SNA
	// snapshot stores to describe the paging. It is a record of the write, not a
	// reading of the banking scheme's state: the two agree except when the lock
	// bit (D5) is set and later writes are ignored, and recording the write is
	// both what the format means and what the loader will replay.
	paging7FFD uint8
}

// PagingValue returns the last byte written to the 0x7FFD paging port. The
// emulator uses it when saving a 128K snapshot; a model with no paging port
// (the 48K) never sets it, so 0 is the right answer there too.
func (m *genericMapper) PagingValue() uint8 { return m.paging7FFD }

// SetTRDosState wires the shared TR-DOS arbitration state.
func (m *genericMapper) SetTRDosState(s *io_ports.TRDosState) { m.trdos = s }

// NewGenericMapper creates a memory mapper using the given banking scheme.
func NewGenericMapper(cfg model.Config, scheme banking.BankingScheme) MemoryMapper {
	numROMs := scheme.NumROMBanks()

	// For legacy compatibility, ensure we have at least 5 ROM banks for 128K models
	// (to support ROM banks 0, 1, 2, 3, 4 where TR-DOS is at bank 4)
	if cfg.PagingModel == "128k" && numROMs < 5 {
		numROMs = 5
	}

	m := &genericMapper{
		cfg:    cfg,
		scheme: scheme,
		roms:   make([][]byte, numROMs),
		rams:   make([][]byte, scheme.NumRAMBanks()),
	}

	// Allocate ROM banks
	for i := range m.roms {
		m.roms[i] = make([]byte, 16384)
	}

	// Allocate RAM banks
	for i := range m.rams {
		m.rams[i] = make([]byte, 16384)
	}

	return m
}

// NewGenericMapperWithLayout creates a memory mapper with ROM banks sized to fit the layout.
func NewGenericMapperWithLayout(cfg model.Config, scheme banking.BankingScheme, layout *model.ROMLayout) MemoryMapper {
	numROMs := scheme.NumROMBanks()

	// If we have a layout, size ROM banks to fit the highest bank index
	if layout != nil {
		maxIdx := layout.MaxBankIndex()
		if maxIdx+1 > numROMs {
			numROMs = maxIdx + 1
		}
	}

	m := &genericMapper{
		cfg:    cfg,
		scheme: scheme,
		roms:   make([][]byte, numROMs),
		rams:   make([][]byte, scheme.NumRAMBanks()),
	}

	// Allocate ROM banks
	for i := range m.roms {
		m.roms[i] = make([]byte, 16384)
	}

	// Allocate RAM banks
	for i := range m.rams {
		m.rams[i] = make([]byte, 16384)
	}

	return m
}

// --- MemoryMapper interface ---

func (m *genericMapper) ReadByte(addr uint16) uint8 {
	bankType, bankNum, offset := m.scheme.MapAddress(addr)
	if bankType == banking.BankROM {
		if bankNum >= 0 && bankNum < len(m.roms) {
			return m.roms[bankNum][offset]
		}
		return 0xFF
	}
	// RAM
	if bankNum >= 0 && bankNum < len(m.rams) {
		return m.rams[bankNum][offset]
	}
	return 0xFF
}

func (m *genericMapper) FetchOpcode(addr uint16) uint8 {
	// Let the banking scheme handle PC-based ROM activation (TR-DOS, etc.)
	newROM := m.scheme.FetchOpcode(addr)
	if newROM >= 0 {
		// ROM switched: the Kempston/FDC port arbitration depends on whether the
		// newly-active ROM is TR-DOS (identified by name in the layout, not by a
		// hardcoded bank number).
		if m.trdos != nil {
			m.trdos.SetActive(m.scheme.IsTRDOSBank(newROM))
		}
	}

	// Now read the opcode
	return m.ReadByte(addr)
}

func (m *genericMapper) WriteByte(addr uint16, value uint8) {
	// Dispatch on the actual mapping, not the address: the +3 special paging
	// mode (0x1FFD bit 0) maps RAM over the 0x0000-0x3FFF ROM area, and that
	// RAM must stay writable. Only a real "rom" mapping is guarded by romWrite.
	bankType, bankNum, offset := m.scheme.MapAddress(addr)
	if bankType == banking.BankROM {
		if m.romWrite && bankNum >= 0 && bankNum < len(m.roms) {
			m.roms[bankNum][offset] = value
		}
		return
	}
	if bankNum >= 0 && bankNum < len(m.rams) {
		m.rams[bankNum][offset] = value
	}
}

func (m *genericMapper) ULAReadByte(addr uint16) uint8 {
	// The ULA reads the screen area (0x4000-0x7FFF) from bank 5 (or 7 with the
	// shadow-screen bit) directly. This is a hardware path independent of the
	// CPU's special paging mode, so it must not go through MapAddress (which in
	// +3 special mode remaps 0x4000 to a different bank and would corrupt the
	// screen).
	if addr >= 0x4000 && addr < 0x8000 {
		bank := m.scheme.ULAScreenBank()
		if bank >= 0 && bank < len(m.rams) {
			return m.rams[bank][addr-0x4000]
		}
		return 0xFF
	}
	return m.ReadByte(addr)
}

func (m *genericMapper) WritePort(port uint16, value uint8) {
	if (port & 0xFFFF) == 0x7FFD {
		m.paging7FFD = value
	}
	// Delegate to the banking scheme
	m.scheme.WritePort(port, value)
}

func (m *genericMapper) Model() model.Config {
	return m.cfg
}

func (m *genericMapper) Reset() {
	// Clear all RAM
	for i := range m.rams {
		for j := range m.rams[i] {
			m.rams[i][j] = 0
		}
	}
	// Reset banking scheme
	m.scheme.Reset()
	m.paging7FFD = 0
	// TR-DOS is not paged after a reset (the scheme returns to the default ROM);
	// clear the arbitration state so Kempston stands up again.
	if m.trdos != nil {
		m.trdos.SetActive(false)
	}
}

func (m *genericMapper) LoadROM(bank int, data []byte) error {
	if bank < 0 || bank >= len(m.roms) {
		return fmt.Errorf("ROM bank %d out of range (have %d banks)", bank, len(m.roms))
	}
	if len(data) != 16384 {
		return fmt.Errorf("ROM data must be 16384 bytes, got %d", len(data))
	}
	copy(m.roms[bank], data)
	return nil
}

func (m *genericMapper) Snapshot() [][]byte {
	// Return all RAM banks
	snapshot := make([][]byte, len(m.rams))
	for i := range m.rams {
		snapshot[i] = make([]byte, len(m.rams[i]))
		copy(snapshot[i], m.rams[i])
	}
	return snapshot
}

func (m *genericMapper) RestoreSnapshot(banks [][]byte) {
	for i, bank := range banks {
		if i < len(m.rams) && len(bank) == len(m.rams[i]) {
			copy(m.rams[i], bank)
		}
	}
}

func (m *genericMapper) SetROMWritable(writable bool) {
	m.romWrite = writable
}

// IsContended returns true if the given address is in a bank the ULA contends.
//
// Contention is a property of the *bank*, not the address: the ULA stalls the
// CPU while it fetches screen bytes from a contended bank, wherever that bank
// happens to be paged. The contended-bank sets match Fuse:
//
//	48K        (spec48.c):  bank 5 only (the screen at 0x4000-0x7FFF)
//	128K       (spec128.c): odd banks 1,3,5,7
//	+2A/+3     (specplus3.c): banks 4,5,6,7
func (m *genericMapper) IsContended(addr uint16) bool {
	if m.cfg.ContentionModel == "" || m.cfg.ContentionModel == "none" {
		return false
	}
	bankType, bankNum, _ := m.scheme.MapAddress(addr)
	if bankType != banking.BankRAM {
		return false // ROM is never contended
	}
	switch m.cfg.ContentionModel {
	case "48k":
		return bankNum == 5
	case "128k":
		return bankNum&1 == 1
	case "2a3":
		return bankNum >= 4
	}
	return false
}

// GetBank returns a reference to a RAM bank for direct access (snapshots).
func (m *genericMapper) GetBank(bank int) []byte {
	if bank < 0 || bank >= len(m.rams) {
		return nil
	}
	return m.rams[bank]
}

// SetBank allows direct writing to a RAM bank (snapshots).
func (m *genericMapper) SetBank(bank int, data []byte) {
	if bank >= 0 && bank < len(m.rams) {
		copy(m.rams[bank], data)
	}
}

// HandlesPort implements io_ports.PortHandler for port routing.
func (m *genericMapper) HandlesPort(port uint16) bool {
	// Generic mapper handles paging ports
	// For Standard128: 0x7FFD (port & 0xC002 == 0x4000)
	// For +2A/+3: 0x7FFD and 0x1FFD
	// Let the scheme decide by trying to handle it
	return (port&0xC002) == 0x4000 || (port&0xF002) == 0x1000
}

func (m *genericMapper) Read(port uint16) uint8 {
	// Paging ports are write-only
	return 0xFF
}

func (m *genericMapper) Write(port uint16, value uint8) {
	m.WritePort(port, value)
}

// Ensure genericMapper implements both MemoryMapper and io_ports.PortHandler
var _ MemoryMapper = (*genericMapper)(nil)
var _ io_ports.PortHandler = (*genericMapper)(nil)
