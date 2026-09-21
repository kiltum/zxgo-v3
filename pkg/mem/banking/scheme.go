// Package banking provides ROM/RAM banking abstractions for ZX Spectrum models.
package banking

// BankType identifies the kind of bank a mapped address resolves to. It is a
// uint8 enum, not a string, so the memory mapper's hot path avoids a string
// comparison on every access; a single type byte is what the hot path compares now.
type BankType uint8

const (
	BankROM BankType = iota
	BankRAM
)

// BankingScheme defines how a model maps memory addresses to ROM/RAM banks.
type BankingScheme interface {
	// Name returns the scheme identifier ("standard128", "pentagon1024", "atm")
	Name() string

	// NumRAMBanks returns total RAM banks (8 for 128K, 64 for Pentagon 1024K)
	NumRAMBanks() int

	// NumROMBanks returns total ROM banks
	NumROMBanks() int

	// MapAddress resolves a physical address to (bankType, bankNum, offset)
	// bankNum: bank index (ROM: 0-63, RAM: 0-127)
	// offset: offset within the 16KB bank (0-16383)
	MapAddress(addr uint16) (bankType BankType, bankNum int, offset uint16)

	// ULAScreenBank returns the RAM bank the ULA reads for the screen area
	// (0x4000-0x7FFF): bank 5, or bank 7 with the shadow-screen bit. This is a
	// hardware path independent of the CPU's special paging mode, so it must not
	// go through MapAddress.
	ULAScreenBank() int

	// WritePort handles paging port writes (0x7FFD, 0x1FFD, Pentagon ports, etc.)
	// Returns true if the port was handled
	WritePort(port uint16, value uint8) bool

	// FetchOpcode is called on M1 cycles for PC-based ROM activation (TR-DOS, etc.)
	// Returns the new active ROM bank index (or -1 for no change)
	FetchOpcode(addr uint16) int

	// IsTRDOSBank reports whether the given ROM bank is the TR-DOS (Beta Disk)
	// ROM. The memory mapper uses this to drive the Kempston/FDC port arbitration.
	IsTRDOSBank(bank int) bool

	// ActiveROM returns the currently active ROM bank index
	ActiveROM() int

	// SetActiveROM sets the active ROM bank (for snapshots, reset)
	SetActiveROM(bank int)

	// Reset resets paging state to defaults
	Reset()

	// Encode returns the scheme's paging state as bytes for the native state
	// format (STATE_DESIGN.md). Name is the discriminator: a blob written by one
	// scheme is refused by another, which is what keeps the 128K's 3-bit page
	// from being read as the Pentagon 512's 5-bit one.
	//
	// This is the only form of the paging state that is serialised. An earlier
	// Snapshot/Restore pair handed an unexported struct to a caller in the same
	// process; it was never called outside its own test, and two serialisations
	// of one state is how the two drift apart, so it is gone.
	Encode() []byte

	// Decode applies a blob from Encode. It parses the whole blob before
	// assigning anything, so a short or corrupt one leaves the scheme as it was.
	Decode(blob []byte) error
}
