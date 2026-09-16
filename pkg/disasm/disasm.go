// Package disasm provides a Z80 disassembler implementation
package disasm

import "fmt"

// Instruction represents a decoded Z80 instruction
type Instruction struct {
	Mnemonic string // Human-readable instruction mnemonic
	Length   int    // Number of bytes the instruction occupies
	Address  uint16 // Address operand for jump/load instructions, 0xFFFF if not applicable
}

// Disassembler represents a Z80 disassembler
type Disassembler struct{}

// New creates a new Z80 disassembler
func New() *Disassembler {
	return &Disassembler{}
}

// Decode decodes a single Z80 instruction from a byte slice.
// It returns the decoded instruction and any error encountered.
func (d *Disassembler) Decode(data []byte) (*Instruction, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}

	// The per-opcode decoders index data[0..3] directly (the longest Z80
	// instruction is 4 bytes). Pad a truncated buffer so a caller that hands in
	// fewer than 4 bytes (e.g. at the end of a memory region) does not panic.
	// The returned Length is derived from the opcode, not the padded input.
	if len(data) < 4 {
		padded := make([]byte, 4)
		copy(padded, data)
		data = padded
	}

	// Get the first opcode byte
	opcode := data[0]

	// Handle prefixed instructions (CB, DD, ED, FD prefixes)
	switch opcode {
	case 0xCB:
		return d.decodeCB(data)
	case 0xDD:
		return d.decodeDD(data)
	case 0xED:
		return d.decodeED(data)
	case 0xFD:
		return d.decodeFD(data)
	default:
		return d.decodeUnprefixed(opcode, data)
	}
}
