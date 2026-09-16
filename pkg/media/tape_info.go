package media

// TapeInfo represents basic tape information for testing and inspection
type TapeInfo struct {
	Format     string // "TAP" or "TZX"
	FormatType string // Same as Format, for consistency
	BlockCount int    // Number of decoded blocks
	PulseCount int    // Total number of pulses in the tape bitstream
	FileName   string // Name of the tape file
}

// GetTapeInfo returns basic information about a tape
func (t *Tape) GetTapeInfo() TapeInfo {
	return TapeInfo{
		Format:     t.Format,
		FormatType: t.Format,
		BlockCount: len(t.Blocks),
		PulseCount: len(t.Pulses),
		FileName:   t.FileName,
	}
}
