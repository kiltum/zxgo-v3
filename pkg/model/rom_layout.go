package model

// ROMLayoutFor returns the ROM layout for a model name, or nil if the name is
// not a known model. This is the single place that maps a model name to its
// layout (the emulator, the test compatibility wrapper, and cmd/zxgo all
// previously duplicated this switch).
func ROMLayoutFor(name string) *ROMLayout {
	switch name {
	case "ZX Spectrum 48K":
		return Spectrum48K_ROMLayout
	case "ZX Spectrum 128K":
		return Spectrum128K_ROMLayout
	case "ZX Spectrum +2A/+3":
		return Spectrum2A3_ROMLayout
	case "Pentagon 128":
		return Pentagon128_ROMLayout
	case "Pentagon 512":
		return Pentagon128_ROMLayout // same ROMs as the Pentagon 128
	default:
		return nil
	}
}

// ROMDescriptor describes one logical ROM bank and how it's loaded/activated.
type ROMDescriptor struct {
	// Identity
	Name        string // "beta", "pentagon-0", "atm-bios"
	Description string // Human-readable description

	// Physical layout
	BankIndex int // Which physical ROM bank slot (0-63)
	SizeKB    int // Size in KB: 8, 16, or 32

	// File loading
	FilePath   string // Path relative to roms/ directory, or absolute
	FileOffset int    // Offset within file (for multi-bank ROMs in one file)
	Optional   bool   // If true, missing file is not an error

	// Embedded fallback
	EmbeddedName string // Name in embedded ROM registry (e.g., "48k/48.rom")

	// Activation
	Activation ROMActivation // How/when this ROM becomes active
}

// ROMLayout describes the complete ROM configuration for a model.
type ROMLayout struct {
	// All ROM banks for this model
	ROMs []ROMDescriptor

	// Default active ROM bank on reset
	DefaultROM int
}

// MaxBankIndex returns the highest ROM bank index in the layout.
func (r *ROMLayout) MaxBankIndex() int {
	maxIdx := -1
	for _, rom := range r.ROMs {
		if rom.BankIndex > maxIdx {
			maxIdx = rom.BankIndex
		}
	}
	return maxIdx
}

// IsTRDOSBank reports whether the ROM bank at index bank is the TR-DOS (Beta
// Disk) ROM. The TR-DOS ROM is identified by name "trdos" in the layout; the
// memory mapper uses this to drive the Kempston/FDC port arbitration instead of
// hardcoding a bank number.
func (r *ROMLayout) IsTRDOSBank(bank int) bool {
	if r == nil {
		return false
	}
	for _, rom := range r.ROMs {
		if rom.Name == "trdos" && rom.BankIndex == bank {
			return true
		}
	}
	return false
}

// ROMActivation describes when and how a ROM bank becomes active.
// Multiple conditions can be combined (e.g., port selects ROM1, then PC range
// switches to Beta).
type ROMActivation struct {
	// Port-based selection: ROM activated by specific port write value
	PortSelect *PortSelect

	// PC-based activation: ROM activated when PC enters range during M1 cycle
	PCRange *PCRange

	// Trigger conditions: switch FROM one ROM bank TO another
	TriggerFrom int // -1 = any, >=0 = specific ROM bank index
	TriggerTo   int // Target ROM bank index

	// Deactivation: return to previous ROM when PC leaves range
	DeactivatePC *PCRange
}

// PortSelect describes ROM activation via I/O port write.
type PortSelect struct {
	Port  uint16 // Port address (e.g., 0x7FFD, 0x1FFD)
	Mask  uint8  // Bits to check
	Value uint8  // Expected value after masking
}

// PCRange describes a PC address range [Low, High] inclusive.
type PCRange struct {
	Low  uint16
	High uint16
}

// MatchesPort returns true if the port write matches this selector.
func (ps *PortSelect) MatchesPort(port uint16, value uint8) bool {
	if ps == nil {
		return false
	}
	return port == ps.Port && (value&ps.Mask) == ps.Value
}

// ContainsPC returns true if PC is within this range.
func (pr *PCRange) ContainsPC(pc uint16) bool {
	if pr == nil {
		return false
	}
	return pc >= pr.Low && pc <= pr.High
}

// ActivatesOnPort returns true if this activation responds to the given port write.
func (a *ROMActivation) ActivatesOnPort(port uint16, value uint8) bool {
	return a.PortSelect != nil && a.PortSelect.MatchesPort(port, value)
}

// ActivatesOnPC returns true if this activation triggers on the given PC during
// M1. currentROM is the currently active ROM bank index.
func (a *ROMActivation) ActivatesOnPC(pc uint16, currentROM int) bool {
	if a.PCRange == nil {
		return false
	}
	if !a.PCRange.ContainsPC(pc) {
		return false
	}
	// Check trigger condition if specified
	if a.TriggerFrom >= 0 && currentROM != a.TriggerFrom {
		return false
	}
	return true
}

// DeactivatesOnPC returns true if this ROM should deactivate when PC is at the
// given address.
func (a *ROMActivation) DeactivatesOnPC(pc uint16) bool {
	return a.DeactivatePC != nil && a.DeactivatePC.ContainsPC(pc)
}
