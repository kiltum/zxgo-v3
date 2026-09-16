// Package mem implements ZX Spectrum memory models.
//
// The MemoryMapper interface handles the address space mapping including
// bank paging. Each model (48K, 128K, +2A/+3) gets its own implementation.
//
// MemoryMapper also implements io_ports.PortHandler to handle paging port
// writes (0x7FFD, 0x1FFD).
package mem

import "github.com/kiltum/zxgo-v3/pkg/model"

// MemoryMapper handles the address space mapping including bank paging.
// The CPU never calls this directly -- it goes through bus.BusReaderWriter.
// The ULA calls ULAReadByte for screen memory reads.
type MemoryMapper interface {
	// ReadByte reads from the physical address space (0x0000-0xFFFF).
	ReadByte(addr uint16) uint8

	// FetchOpcode reads an instruction byte during an M1 cycle.
	// Required for TR-DOS hardware activation via address trapping in 0x3D00-0x3DFF.
	FetchOpcode(addr uint16) uint8

	// WriteByte writes to the physical address space.
	WriteByte(addr uint16, value uint8)

	// ULAReadByte reads screen memory, respecting the shadow screen bit.
	// The ULA uses this instead of ReadByte for screen fetches.
	ULAReadByte(addr uint16) uint8

	// WritePort handles writes to paging ports (0x7FFD, 0x1FFD).
	// Called by PortBus when a matching port write occurs.
	WritePort(port uint16, value uint8)

	// Model returns the model configuration this mapper was built for.
	Model() model.Config

	// Reset clears all RAM banks (ROM banks are preserved).
	Reset()

	// LoadROM loads a ROM bank with embedded ROM data.
	LoadROM(bank int, data []byte) error

	// Snapshot returns all RAM banks for state serialization.
	Snapshot() [][]byte

	// RestoreSnapshot restores all RAM banks from a snapshot.
	RestoreSnapshot(banks [][]byte)

	// SetROMWritable enables/disables writes to ROM area (0x0000-0x3FFF).
	// Used during ROM loading and test setup.
	SetROMWritable(writable bool)

	// IsContended returns true if the given address may be subject to ULA contention.
	// For 48K: only 0x4000-0x7FFF (screen RAM). For 128K: banks 1,3,5,7 -- which
	// means 0x4000-0x7FFF (always bank 5) and 0xC000-0xFFFF only when paged to banks 1,3,5,7.
	IsContended(addr uint16) bool

	// GetBank returns a reference to a RAM bank for direct access (needed for snapshots).
	// For 48K models this returns nil as they use a flat memory model.
	GetBank(bank int) []byte

	// SetBank allows direct writing to a RAM bank (needed for snapshots).
	// For 48K models this is a no-op as they use a flat memory model.
	SetBank(bank int, data []byte)
}
