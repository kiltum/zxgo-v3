package disasm

import (
	"fmt"
)

// decodeUnprefixed decodes unprefixed Z80 instructions
func (d *Disassembler) decodeUnprefixed(opcode byte, data []byte) (*Instruction, error) {
	switch opcode {
	// 8-bit load group
	case 0x00:
		return &Instruction{Mnemonic: "NOP", Length: 1, Address: 0xFFFF}, nil
	case 0x01:
		nn := uint16(data[2])<<8 | uint16(data[1])
		return &Instruction{Mnemonic: fmt.Sprintf("LD BC, $%04X", nn), Length: 3, Address: nn}, nil
	case 0x02:
		return &Instruction{Mnemonic: "LD (BC), A", Length: 1, Address: 0xFFFF}, nil
	case 0x03:
		return &Instruction{Mnemonic: "INC BC", Length: 1, Address: 0xFFFF}, nil
	case 0x04:
		return &Instruction{Mnemonic: "INC B", Length: 1, Address: 0xFFFF}, nil
	case 0x05:
		return &Instruction{Mnemonic: "DEC B", Length: 1, Address: 0xFFFF}, nil
	case 0x06:
		n := data[1]
		return &Instruction{Mnemonic: fmt.Sprintf("LD B, $%02X", n), Length: 2, Address: 0xFFFF}, nil
	case 0x07:
		return &Instruction{Mnemonic: "RLCA", Length: 1, Address: 0xFFFF}, nil
	case 0x08:
		return &Instruction{Mnemonic: "EX AF, AF'", Length: 1, Address: 0xFFFF}, nil
	case 0x09:
		return &Instruction{Mnemonic: "ADD HL, BC", Length: 1, Address: 0xFFFF}, nil
	case 0x0A:
		return &Instruction{Mnemonic: "LD A, (BC)", Length: 1, Address: 0xFFFF}, nil
	case 0x0B:
		return &Instruction{Mnemonic: "DEC BC", Length: 1, Address: 0xFFFF}, nil
	case 0x0C:
		return &Instruction{Mnemonic: "INC C", Length: 1, Address: 0xFFFF}, nil
	case 0x0D:
		return &Instruction{Mnemonic: "DEC C", Length: 1, Address: 0xFFFF}, nil
	case 0x0E:
		n := data[1]
		return &Instruction{Mnemonic: fmt.Sprintf("LD C, $%02X", n), Length: 2, Address: 0xFFFF}, nil
	case 0x0F:
		return &Instruction{Mnemonic: "RRCA", Length: 1, Address: 0xFFFF}, nil
	case 0x10:
		e := int8(data[1])
		if e >= 0 {
			return &Instruction{Mnemonic: fmt.Sprintf("DJNZ $%02X", e), Length: 2, Address: 0xFFFF}, nil
		} else {
			return &Instruction{Mnemonic: fmt.Sprintf("DJNZ -$%02X", -e), Length: 2, Address: 0xFFFF}, nil
		}
	case 0x11:
		nn := uint16(data[2])<<8 | uint16(data[1])
		return &Instruction{Mnemonic: fmt.Sprintf("LD DE, $%04X", nn), Length: 3, Address: nn}, nil
	case 0x12:
		return &Instruction{Mnemonic: "LD (DE), A", Length: 1, Address: 0xFFFF}, nil
	case 0x13:
		return &Instruction{Mnemonic: "INC DE", Length: 1, Address: 0xFFFF}, nil
	case 0x14:
		return &Instruction{Mnemonic: "INC D", Length: 1, Address: 0xFFFF}, nil
	case 0x15:
		return &Instruction{Mnemonic: "DEC D", Length: 1, Address: 0xFFFF}, nil
	case 0x16:
		n := data[1]
		return &Instruction{Mnemonic: fmt.Sprintf("LD D, $%02X", n), Length: 2, Address: 0xFFFF}, nil
	case 0x17:
		return &Instruction{Mnemonic: "RLA", Length: 1, Address: 0xFFFF}, nil
	case 0x18:
		e := int8(data[1])
		if e >= 0 {
			return &Instruction{Mnemonic: fmt.Sprintf("JR $%02X", e), Length: 2, Address: 0xFFFF}, nil
		} else {
			return &Instruction{Mnemonic: fmt.Sprintf("JR -$%02X", -e), Length: 2, Address: 0xFFFF}, nil
		}
	case 0x19:
		return &Instruction{Mnemonic: "ADD HL, DE", Length: 1, Address: 0xFFFF}, nil
	case 0x1A:
		return &Instruction{Mnemonic: "LD A, (DE)", Length: 1, Address: 0xFFFF}, nil
	case 0x1B:
		return &Instruction{Mnemonic: "DEC DE", Length: 1, Address: 0xFFFF}, nil
	case 0x1C:
		return &Instruction{Mnemonic: "INC E", Length: 1, Address: 0xFFFF}, nil
	case 0x1D:
		return &Instruction{Mnemonic: "DEC E", Length: 1, Address: 0xFFFF}, nil
	case 0x1E:
		n := data[1]
		return &Instruction{Mnemonic: fmt.Sprintf("LD E, $%02X", n), Length: 2, Address: 0xFFFF}, nil
	case 0x1F:
		return &Instruction{Mnemonic: "RRA", Length: 1, Address: 0xFFFF}, nil
	case 0x20:
		e := int8(data[1])
		if e >= 0 {
			return &Instruction{Mnemonic: fmt.Sprintf("JR NZ, $%02X", e), Length: 2, Address: 0xFFFF}, nil
		} else {
			return &Instruction{Mnemonic: fmt.Sprintf("JR NZ, -$%02X", -e), Length: 2, Address: 0xFFFF}, nil
		}
	case 0x21:
		nn := uint16(data[2])<<8 | uint16(data[1])
		return &Instruction{Mnemonic: fmt.Sprintf("LD HL, $%04X", nn), Length: 3, Address: nn}, nil
	case 0x22:
		nn := uint16(data[2])<<8 | uint16(data[1])
		return &Instruction{Mnemonic: fmt.Sprintf("LD ($%04X), HL", nn), Length: 3, Address: nn}, nil
	case 0x23:
		return &Instruction{Mnemonic: "INC HL", Length: 1, Address: 0xFFFF}, nil
	case 0x24:
		return &Instruction{Mnemonic: "INC H", Length: 1, Address: 0xFFFF}, nil
	case 0x25:
		return &Instruction{Mnemonic: "DEC H", Length: 1, Address: 0xFFFF}, nil
	case 0x26:
		n := data[1]
		return &Instruction{Mnemonic: fmt.Sprintf("LD H, $%02X", n), Length: 2, Address: 0xFFFF}, nil
	case 0x27:
		return &Instruction{Mnemonic: "DAA", Length: 1, Address: 0xFFFF}, nil
	case 0x28:
		e := int8(data[1])
		if e >= 0 {
			return &Instruction{Mnemonic: fmt.Sprintf("JR Z, $%02X", e), Length: 2, Address: 0xFFFF}, nil
		} else {
			return &Instruction{Mnemonic: fmt.Sprintf("JR Z, -$%02X", -e), Length: 2, Address: 0xFFFF}, nil
		}
	case 0x29:
		return &Instruction{Mnemonic: "ADD HL, HL", Length: 1, Address: 0xFFFF}, nil
	case 0x2A:
		nn := uint16(data[2])<<8 | uint16(data[1])
		return &Instruction{Mnemonic: fmt.Sprintf("LD HL, ($%04X)", nn), Length: 3, Address: nn}, nil
	case 0x2B:
		return &Instruction{Mnemonic: "DEC HL", Length: 1, Address: 0xFFFF}, nil
	case 0x2C:
		return &Instruction{Mnemonic: "INC L", Length: 1, Address: 0xFFFF}, nil
	case 0x2D:
		return &Instruction{Mnemonic: "DEC L", Length: 1, Address: 0xFFFF}, nil
	case 0x2E:
		n := data[1]
		return &Instruction{Mnemonic: fmt.Sprintf("LD L, $%02X", n), Length: 2, Address: 0xFFFF}, nil
	case 0x2F:
		return &Instruction{Mnemonic: "CPL", Length: 1, Address: 0xFFFF}, nil
	case 0x30:
		e := int8(data[1])
		if e >= 0 {
			return &Instruction{Mnemonic: fmt.Sprintf("JR NC, $%02X", e), Length: 2, Address: 0xFFFF}, nil
		} else {
			return &Instruction{Mnemonic: fmt.Sprintf("JR NC, -$%02X", -e), Length: 2, Address: 0xFFFF}, nil
		}
	case 0x31:
		nn := uint16(data[2])<<8 | uint16(data[1])
		return &Instruction{Mnemonic: fmt.Sprintf("LD SP, $%04X", nn), Length: 3, Address: nn}, nil
	case 0x32:
		nn := uint16(data[2])<<8 | uint16(data[1])
		return &Instruction{Mnemonic: fmt.Sprintf("LD ($%04X), A", nn), Length: 3, Address: nn}, nil
	case 0x33:
		return &Instruction{Mnemonic: "INC SP", Length: 1, Address: 0xFFFF}, nil
	case 0x34:
		return &Instruction{Mnemonic: "INC (HL)", Length: 1, Address: 0xFFFF}, nil
	case 0x35:
		return &Instruction{Mnemonic: "DEC (HL)", Length: 1, Address: 0xFFFF}, nil
	case 0x36:
		n := data[1]
		return &Instruction{Mnemonic: fmt.Sprintf("LD (HL), $%02X", n), Length: 2, Address: 0xFFFF}, nil
	case 0x37:
		return &Instruction{Mnemonic: "SCF", Length: 1, Address: 0xFFFF}, nil
	case 0x38:
		e := int8(data[1])
		if e >= 0 {
			return &Instruction{Mnemonic: fmt.Sprintf("JR C, $%02X", e), Length: 2, Address: 0xFFFF}, nil
		} else {
			return &Instruction{Mnemonic: fmt.Sprintf("JR C, -$%02X", -e), Length: 2, Address: 0xFFFF}, nil
		}
	case 0x39:
		return &Instruction{Mnemonic: "ADD HL, SP", Length: 1, Address: 0xFFFF}, nil
	case 0x3A:
		nn := uint16(data[2])<<8 | uint16(data[1])
		return &Instruction{Mnemonic: fmt.Sprintf("LD A, ($%04X)", nn), Length: 3, Address: nn}, nil
	case 0x3B:
		return &Instruction{Mnemonic: "DEC SP", Length: 1, Address: 0xFFFF}, nil
	case 0x3C:
		return &Instruction{Mnemonic: "INC A", Length: 1, Address: 0xFFFF}, nil
	case 0x3D:
		return &Instruction{Mnemonic: "DEC A", Length: 1, Address: 0xFFFF}, nil
	case 0x3E:
		n := data[1]
		return &Instruction{Mnemonic: fmt.Sprintf("LD A, $%02X", n), Length: 2, Address: 0xFFFF}, nil
	case 0x3F:
		return &Instruction{Mnemonic: "CCF", Length: 1, Address: 0xFFFF}, nil

	// LD r, r' instructions (0x40-0x7F)
	case 0x40: // LD B, B
		return &Instruction{Mnemonic: "LD B, B", Length: 1, Address: 0xFFFF}, nil
	case 0x41: // LD B, C
		return &Instruction{Mnemonic: "LD B, C", Length: 1, Address: 0xFFFF}, nil
	case 0x42: // LD B, D
		return &Instruction{Mnemonic: "LD B, D", Length: 1, Address: 0xFFFF}, nil
	case 0x43: // LD B, E
		return &Instruction{Mnemonic: "LD B, E", Length: 1, Address: 0xFFFF}, nil
	case 0x44: // LD B, H
		return &Instruction{Mnemonic: "LD B, H", Length: 1, Address: 0xFFFF}, nil
	case 0x45: // LD B, L
		return &Instruction{Mnemonic: "LD B, L", Length: 1, Address: 0xFFFF}, nil
	case 0x46: // LD B, (HL)
		return &Instruction{Mnemonic: "LD B, (HL)", Length: 1, Address: 0xFFFF}, nil
	case 0x47: // LD B, A
		return &Instruction{Mnemonic: "LD B, A", Length: 1, Address: 0xFFFF}, nil
	case 0x48: // LD C, B
		return &Instruction{Mnemonic: "LD C, B", Length: 1, Address: 0xFFFF}, nil
	case 0x49: // LD C, C
		return &Instruction{Mnemonic: "LD C, C", Length: 1, Address: 0xFFFF}, nil
	case 0x4A: // LD C, D
		return &Instruction{Mnemonic: "LD C, D", Length: 1, Address: 0xFFFF}, nil
	case 0x4B: // LD C, E
		return &Instruction{Mnemonic: "LD C, E", Length: 1, Address: 0xFFFF}, nil
	case 0x4C: // LD C, H
		return &Instruction{Mnemonic: "LD C, H", Length: 1, Address: 0xFFFF}, nil
	case 0x4D: // LD C, L
		return &Instruction{Mnemonic: "LD C, L", Length: 1, Address: 0xFFFF}, nil
	case 0x4E: // LD C, (HL)
		return &Instruction{Mnemonic: "LD C, (HL)", Length: 1, Address: 0xFFFF}, nil
	case 0x4F: // LD C, A
		return &Instruction{Mnemonic: "LD C, A", Length: 1, Address: 0xFFFF}, nil
	case 0x50: // LD D, B
		return &Instruction{Mnemonic: "LD D, B", Length: 1, Address: 0xFFFF}, nil
	case 0x51: // LD D, C
		return &Instruction{Mnemonic: "LD D, C", Length: 1, Address: 0xFFFF}, nil
	case 0x52: // LD D, D
		return &Instruction{Mnemonic: "LD D, D", Length: 1, Address: 0xFFFF}, nil
	case 0x53: // LD D, E
		return &Instruction{Mnemonic: "LD D, E", Length: 1, Address: 0xFFFF}, nil
	case 0x54: // LD D, H
		return &Instruction{Mnemonic: "LD D, H", Length: 1, Address: 0xFFFF}, nil
	case 0x55: // LD D, L
		return &Instruction{Mnemonic: "LD D, L", Length: 1, Address: 0xFFFF}, nil
	case 0x56: // LD D, (HL)
		return &Instruction{Mnemonic: "LD D, (HL)", Length: 1, Address: 0xFFFF}, nil
	case 0x57: // LD D, A
		return &Instruction{Mnemonic: "LD D, A", Length: 1, Address: 0xFFFF}, nil
	case 0x58: // LD E, B
		return &Instruction{Mnemonic: "LD E, B", Length: 1, Address: 0xFFFF}, nil
	case 0x59: // LD E, C
		return &Instruction{Mnemonic: "LD E, C", Length: 1, Address: 0xFFFF}, nil
	case 0x5A: // LD E, D
		return &Instruction{Mnemonic: "LD E, D", Length: 1, Address: 0xFFFF}, nil
	case 0x5B: // LD E, E
		return &Instruction{Mnemonic: "LD E, E", Length: 1, Address: 0xFFFF}, nil
	case 0x5C: // LD E, H
		return &Instruction{Mnemonic: "LD E, H", Length: 1, Address: 0xFFFF}, nil
	case 0x5D: // LD E, L
		return &Instruction{Mnemonic: "LD E, L", Length: 1, Address: 0xFFFF}, nil
	case 0x5E: // LD E, (HL)
		return &Instruction{Mnemonic: "LD E, (HL)", Length: 1, Address: 0xFFFF}, nil
	case 0x5F: // LD E, A
		return &Instruction{Mnemonic: "LD E, A", Length: 1, Address: 0xFFFF}, nil
	case 0x60: // LD H, B
		return &Instruction{Mnemonic: "LD H, B", Length: 1, Address: 0xFFFF}, nil
	case 0x61: // LD H, C
		return &Instruction{Mnemonic: "LD H, C", Length: 1, Address: 0xFFFF}, nil
	case 0x62: // LD H, D
		return &Instruction{Mnemonic: "LD H, D", Length: 1, Address: 0xFFFF}, nil
	case 0x63: // LD H, E
		return &Instruction{Mnemonic: "LD H, E", Length: 1, Address: 0xFFFF}, nil
	case 0x64: // LD H, H
		return &Instruction{Mnemonic: "LD H, H", Length: 1, Address: 0xFFFF}, nil
	case 0x65: // LD H, L
		return &Instruction{Mnemonic: "LD H, L", Length: 1, Address: 0xFFFF}, nil
	case 0x66: // LD H, (HL)
		return &Instruction{Mnemonic: "LD H, (HL)", Length: 1, Address: 0xFFFF}, nil
	case 0x67: // LD H, A
		return &Instruction{Mnemonic: "LD H, A", Length: 1, Address: 0xFFFF}, nil
	case 0x68: // LD L, B
		return &Instruction{Mnemonic: "LD L, B", Length: 1, Address: 0xFFFF}, nil
	case 0x69: // LD L, C
		return &Instruction{Mnemonic: "LD L, C", Length: 1, Address: 0xFFFF}, nil
	case 0x6A: // LD L, D
		return &Instruction{Mnemonic: "LD L, D", Length: 1, Address: 0xFFFF}, nil
	case 0x6B: // LD L, E
		return &Instruction{Mnemonic: "LD L, E", Length: 1, Address: 0xFFFF}, nil
	case 0x6C: // LD L, H
		return &Instruction{Mnemonic: "LD L, H", Length: 1, Address: 0xFFFF}, nil
	case 0x6D: // LD L, L
		return &Instruction{Mnemonic: "LD L, L", Length: 1, Address: 0xFFFF}, nil
	case 0x6E: // LD L, (HL)
		return &Instruction{Mnemonic: "LD L, (HL)", Length: 1, Address: 0xFFFF}, nil
	case 0x6F: // LD L, A
		return &Instruction{Mnemonic: "LD L, A", Length: 1, Address: 0xFFFF}, nil
	case 0x70: // LD (HL), B
		return &Instruction{Mnemonic: "LD (HL), B", Length: 1, Address: 0xFFFF}, nil
	case 0x71: // LD (HL), C
		return &Instruction{Mnemonic: "LD (HL), C", Length: 1, Address: 0xFFFF}, nil
	case 0x72: // LD (HL), D
		return &Instruction{Mnemonic: "LD (HL), D", Length: 1, Address: 0xFFFF}, nil
	case 0x73: // LD (HL), E
		return &Instruction{Mnemonic: "LD (HL), E", Length: 1, Address: 0xFFFF}, nil
	case 0x74: // LD (HL), H
		return &Instruction{Mnemonic: "LD (HL), H", Length: 1, Address: 0xFFFF}, nil
	case 0x75: // LD (HL), L
		return &Instruction{Mnemonic: "LD (HL), L", Length: 1, Address: 0xFFFF}, nil
	case 0x76: // HALT
		return &Instruction{Mnemonic: "HALT", Length: 1, Address: 0xFFFF}, nil
	case 0x77: // LD (HL), A
		return &Instruction{Mnemonic: "LD (HL), A", Length: 1, Address: 0xFFFF}, nil
	case 0x78: // LD A, B
		return &Instruction{Mnemonic: "LD A, B", Length: 1, Address: 0xFFFF}, nil
	case 0x79: // LD A, C
		return &Instruction{Mnemonic: "LD A, C", Length: 1, Address: 0xFFFF}, nil
	case 0x7A: // LD A, D
		return &Instruction{Mnemonic: "LD A, D", Length: 1, Address: 0xFFFF}, nil
	case 0x7B: // LD A, E
		return &Instruction{Mnemonic: "LD A, E", Length: 1, Address: 0xFFFF}, nil
	case 0x7C: // LD A, H
		return &Instruction{Mnemonic: "LD A, H", Length: 1, Address: 0xFFFF}, nil
	case 0x7D: // LD A, L
		return &Instruction{Mnemonic: "LD A, L", Length: 1, Address: 0xFFFF}, nil
	case 0x7E: // LD A, (HL)
		return &Instruction{Mnemonic: "LD A, (HL)", Length: 1, Address: 0xFFFF}, nil
	case 0x7F: // LD A, A
		return &Instruction{Mnemonic: "LD A, A", Length: 1, Address: 0xFFFF}, nil

	// ALU operations (0x80-0xBF)
	case 0x80: // ADD A, B
		return &Instruction{Mnemonic: "ADD A, B", Length: 1, Address: 0xFFFF}, nil
	case 0x81: // ADD A, C
		return &Instruction{Mnemonic: "ADD A, C", Length: 1, Address: 0xFFFF}, nil
	case 0x82: // ADD A, D
		return &Instruction{Mnemonic: "ADD A, D", Length: 1, Address: 0xFFFF}, nil
	case 0x83: // ADD A, E
		return &Instruction{Mnemonic: "ADD A, E", Length: 1, Address: 0xFFFF}, nil
	case 0x84: // ADD A, H
		return &Instruction{Mnemonic: "ADD A, H", Length: 1, Address: 0xFFFF}, nil
	case 0x85: // ADD A, L
		return &Instruction{Mnemonic: "ADD A, L", Length: 1, Address: 0xFFFF}, nil
	case 0x86: // ADD A, (HL)
		return &Instruction{Mnemonic: "ADD A, (HL)", Length: 1, Address: 0xFFFF}, nil
	case 0x87: // ADD A, A
		return &Instruction{Mnemonic: "ADD A, A", Length: 1, Address: 0xFFFF}, nil
	case 0x88: // ADC A, B
		return &Instruction{Mnemonic: "ADC A, B", Length: 1, Address: 0xFFFF}, nil
	case 0x89: // ADC A, C
		return &Instruction{Mnemonic: "ADC A, C", Length: 1, Address: 0xFFFF}, nil
	case 0x8A: // ADC A, D
		return &Instruction{Mnemonic: "ADC A, D", Length: 1, Address: 0xFFFF}, nil
	case 0x8B: // ADC A, E
		return &Instruction{Mnemonic: "ADC A, E", Length: 1, Address: 0xFFFF}, nil
	case 0x8C: // ADC A, H
		return &Instruction{Mnemonic: "ADC A, H", Length: 1, Address: 0xFFFF}, nil
	case 0x8D: // ADC A, L
		return &Instruction{Mnemonic: "ADC A, L", Length: 1, Address: 0xFFFF}, nil
	case 0x8E: // ADC A, (HL)
		return &Instruction{Mnemonic: "ADC A, (HL)", Length: 1, Address: 0xFFFF}, nil
	case 0x8F: // ADC A, A
		return &Instruction{Mnemonic: "ADC A, A", Length: 1, Address: 0xFFFF}, nil
	case 0x90: // SUB B
		return &Instruction{Mnemonic: "SUB B", Length: 1, Address: 0xFFFF}, nil
	case 0x91: // SUB C
		return &Instruction{Mnemonic: "SUB C", Length: 1, Address: 0xFFFF}, nil
	case 0x92: // SUB D
		return &Instruction{Mnemonic: "SUB D", Length: 1, Address: 0xFFFF}, nil
	case 0x93: // SUB E
		return &Instruction{Mnemonic: "SUB E", Length: 1, Address: 0xFFFF}, nil
	case 0x94: // SUB H
		return &Instruction{Mnemonic: "SUB H", Length: 1, Address: 0xFFFF}, nil
	case 0x95: // SUB L
		return &Instruction{Mnemonic: "SUB L", Length: 1, Address: 0xFFFF}, nil
	case 0x96: // SUB (HL)
		return &Instruction{Mnemonic: "SUB (HL)", Length: 1, Address: 0xFFFF}, nil
	case 0x97: // SUB A
		return &Instruction{Mnemonic: "SUB A", Length: 1, Address: 0xFFFF}, nil
	case 0x98: // SBC A, B
		return &Instruction{Mnemonic: "SBC A, B", Length: 1, Address: 0xFFFF}, nil
	case 0x99: // SBC A, C
		return &Instruction{Mnemonic: "SBC A, C", Length: 1, Address: 0xFFFF}, nil
	case 0x9A: // SBC A, D
		return &Instruction{Mnemonic: "SBC A, D", Length: 1, Address: 0xFFFF}, nil
	case 0x9B: // SBC A, E
		return &Instruction{Mnemonic: "SBC A, E", Length: 1, Address: 0xFFFF}, nil
	case 0x9C: // SBC A, H
		return &Instruction{Mnemonic: "SBC A, H", Length: 1, Address: 0xFFFF}, nil
	case 0x9D: // SBC A, L
		return &Instruction{Mnemonic: "SBC A, L", Length: 1, Address: 0xFFFF}, nil
	case 0x9E: // SBC A, (HL)
		return &Instruction{Mnemonic: "SBC A, (HL)", Length: 1, Address: 0xFFFF}, nil
	case 0x9F: // SBC A, A
		return &Instruction{Mnemonic: "SBC A, A", Length: 1, Address: 0xFFFF}, nil
	case 0xA0: // AND B
		return &Instruction{Mnemonic: "AND B", Length: 1, Address: 0xFFFF}, nil
	case 0xA1: // AND C
		return &Instruction{Mnemonic: "AND C", Length: 1, Address: 0xFFFF}, nil
	case 0xA2: // AND D
		return &Instruction{Mnemonic: "AND D", Length: 1, Address: 0xFFFF}, nil
	case 0xA3: // AND E
		return &Instruction{Mnemonic: "AND E", Length: 1, Address: 0xFFFF}, nil
	case 0xA4: // AND H
		return &Instruction{Mnemonic: "AND H", Length: 1, Address: 0xFFFF}, nil
	case 0xA5: // AND L
		return &Instruction{Mnemonic: "AND L", Length: 1, Address: 0xFFFF}, nil
	case 0xA6: // AND (HL)
		return &Instruction{Mnemonic: "AND (HL)", Length: 1, Address: 0xFFFF}, nil
	case 0xA7: // AND A
		return &Instruction{Mnemonic: "AND A", Length: 1, Address: 0xFFFF}, nil
	case 0xA8: // XOR B
		return &Instruction{Mnemonic: "XOR B", Length: 1, Address: 0xFFFF}, nil
	case 0xA9: // XOR C
		return &Instruction{Mnemonic: "XOR C", Length: 1, Address: 0xFFFF}, nil
	case 0xAA: // XOR D
		return &Instruction{Mnemonic: "XOR D", Length: 1, Address: 0xFFFF}, nil
	case 0xAB: // XOR E
		return &Instruction{Mnemonic: "XOR E", Length: 1, Address: 0xFFFF}, nil
	case 0xAC: // XOR H
		return &Instruction{Mnemonic: "XOR H", Length: 1, Address: 0xFFFF}, nil
	case 0xAD: // XOR L
		return &Instruction{Mnemonic: "XOR L", Length: 1, Address: 0xFFFF}, nil
	case 0xAE: // XOR (HL)
		return &Instruction{Mnemonic: "XOR (HL)", Length: 1, Address: 0xFFFF}, nil
	case 0xAF: // XOR A
		return &Instruction{Mnemonic: "XOR A", Length: 1, Address: 0xFFFF}, nil
	case 0xB0: // OR B
		return &Instruction{Mnemonic: "OR B", Length: 1, Address: 0xFFFF}, nil
	case 0xB1: // OR C
		return &Instruction{Mnemonic: "OR C", Length: 1, Address: 0xFFFF}, nil
	case 0xB2: // OR D
		return &Instruction{Mnemonic: "OR D", Length: 1, Address: 0xFFFF}, nil
	case 0xB3: // OR E
		return &Instruction{Mnemonic: "OR E", Length: 1, Address: 0xFFFF}, nil
	case 0xB4: // OR H
		return &Instruction{Mnemonic: "OR H", Length: 1, Address: 0xFFFF}, nil
	case 0xB5: // OR L
		return &Instruction{Mnemonic: "OR L", Length: 1, Address: 0xFFFF}, nil
	case 0xB6: // OR (HL)
		return &Instruction{Mnemonic: "OR (HL)", Length: 1, Address: 0xFFFF}, nil
	case 0xB7: // OR A
		return &Instruction{Mnemonic: "OR A", Length: 1, Address: 0xFFFF}, nil
	case 0xB8: // CP B
		return &Instruction{Mnemonic: "CP B", Length: 1, Address: 0xFFFF}, nil
	case 0xB9: // CP C
		return &Instruction{Mnemonic: "CP C", Length: 1, Address: 0xFFFF}, nil
	case 0xBA: // CP D
		return &Instruction{Mnemonic: "CP D", Length: 1, Address: 0xFFFF}, nil
	case 0xBB: // CP E
		return &Instruction{Mnemonic: "CP E", Length: 1, Address: 0xFFFF}, nil
	case 0xBC: // CP H
		return &Instruction{Mnemonic: "CP H", Length: 1, Address: 0xFFFF}, nil
	case 0xBD: // CP L
		return &Instruction{Mnemonic: "CP L", Length: 1, Address: 0xFFFF}, nil
	case 0xBE: // CP (HL)
		return &Instruction{Mnemonic: "CP (HL)", Length: 1, Address: 0xFFFF}, nil
	case 0xBF: // CP A
		return &Instruction{Mnemonic: "CP A", Length: 1, Address: 0xFFFF}, nil

	// RET cc instructions (0xC0-0xC7)
	case 0xC0: // RET NZ
		return &Instruction{Mnemonic: "RET NZ", Length: 1, Address: 0xFFFF}, nil
	case 0xC1: // POP BC
		return &Instruction{Mnemonic: "POP BC", Length: 1, Address: 0xFFFF}, nil
	case 0xC2: // JP NZ, nn
		nn := uint16(data[2])<<8 | uint16(data[1])
		return &Instruction{Mnemonic: fmt.Sprintf("JP NZ, $%04X", nn), Length: 3, Address: nn}, nil
	case 0xC3: // JP nn
		nn := uint16(data[2])<<8 | uint16(data[1])
		return &Instruction{Mnemonic: fmt.Sprintf("JP $%04X", nn), Length: 3, Address: nn}, nil
	case 0xC4: // CALL NZ, nn
		nn := uint16(data[2])<<8 | uint16(data[1])
		return &Instruction{Mnemonic: fmt.Sprintf("CALL NZ, $%04X", nn), Length: 3, Address: nn}, nil
	case 0xC5: // PUSH BC
		return &Instruction{Mnemonic: "PUSH BC", Length: 1, Address: 0xFFFF}, nil
	case 0xC6: // ADD A, n
		n := data[1]
		return &Instruction{Mnemonic: fmt.Sprintf("ADD A, $%02X", n), Length: 2, Address: 0xFFFF}, nil
	case 0xC7: // RST 00H
		return &Instruction{Mnemonic: "RST 00H", Length: 1, Address: 0x0000}, nil
	case 0xC8: // RET Z
		return &Instruction{Mnemonic: "RET Z", Length: 1, Address: 0xFFFF}, nil
	case 0xC9: // RET
		return &Instruction{Mnemonic: "RET", Length: 1, Address: 0xFFFF}, nil
	case 0xCA: // JP Z, nn
		nn := uint16(data[2])<<8 | uint16(data[1])
		return &Instruction{Mnemonic: fmt.Sprintf("JP Z, $%04X", nn), Length: 3, Address: nn}, nil
	case 0xCB: // PREFIX CB
		// This should be handled in the main Decode function
		return &Instruction{Mnemonic: "PREFIX CB", Length: 1, Address: 0xFFFF}, nil
	case 0xCC: // CALL Z, nn
		nn := uint16(data[2])<<8 | uint16(data[1])
		return &Instruction{Mnemonic: fmt.Sprintf("CALL Z, $%04X", nn), Length: 3, Address: nn}, nil
	case 0xCD: // CALL nn
		nn := uint16(data[2])<<8 | uint16(data[1])
		return &Instruction{Mnemonic: fmt.Sprintf("CALL $%04X", nn), Length: 3, Address: nn}, nil
	case 0xCE: // ADC A, n
		n := data[1]
		return &Instruction{Mnemonic: fmt.Sprintf("ADC A, $%02X", n), Length: 2, Address: 0xFFFF}, nil
	case 0xCF: // RST 08H
		return &Instruction{Mnemonic: "RST 08H", Length: 1, Address: 0x0008}, nil

	// Conditional operations (0xD0-0xDF)
	case 0xD0: // RET NC
		return &Instruction{Mnemonic: "RET NC", Length: 1, Address: 0xFFFF}, nil
	case 0xD1: // POP DE
		return &Instruction{Mnemonic: "POP DE", Length: 1, Address: 0xFFFF}, nil
	case 0xD2: // JP NC, nn
		nn := uint16(data[2])<<8 | uint16(data[1])
		return &Instruction{Mnemonic: fmt.Sprintf("JP NC, $%04X", nn), Length: 3, Address: nn}, nil
	case 0xD3: // OUT (n), A
		n := data[1]
		return &Instruction{Mnemonic: fmt.Sprintf("OUT ($%02X), A", n), Length: 2, Address: 0xFFFF}, nil
	case 0xD4: // CALL NC, nn
		nn := uint16(data[2])<<8 | uint16(data[1])
		return &Instruction{Mnemonic: fmt.Sprintf("CALL NC, $%04X", nn), Length: 3, Address: nn}, nil
	case 0xD5: // PUSH DE
		return &Instruction{Mnemonic: "PUSH DE", Length: 1, Address: 0xFFFF}, nil
	case 0xD6: // SUB n
		n := data[1]
		return &Instruction{Mnemonic: fmt.Sprintf("SUB $%02X", n), Length: 2, Address: 0xFFFF}, nil
	case 0xD7: // RST 10H
		return &Instruction{Mnemonic: "RST 10H", Length: 1, Address: 0x0010}, nil
	case 0xD8: // RET C
		return &Instruction{Mnemonic: "RET C", Length: 1, Address: 0xFFFF}, nil
	case 0xD9: // EXX
		return &Instruction{Mnemonic: "EXX", Length: 1, Address: 0xFFFF}, nil
	case 0xDA: // JP C, nn
		nn := uint16(data[2])<<8 | uint16(data[1])
		return &Instruction{Mnemonic: fmt.Sprintf("JP C, $%04X", nn), Length: 3, Address: nn}, nil
	case 0xDB: // IN A, (n)
		n := data[1]
		return &Instruction{Mnemonic: fmt.Sprintf("IN A, ($%02X)", n), Length: 2, Address: 0xFFFF}, nil
	case 0xDC: // CALL C, nn
		nn := uint16(data[2])<<8 | uint16(data[1])
		return &Instruction{Mnemonic: fmt.Sprintf("CALL C, $%04X", nn), Length: 3, Address: nn}, nil
	case 0xDD: // PREFIX DD
		// This should be handled in the main Decode function
		return &Instruction{Mnemonic: "PREFIX DD", Length: 1, Address: 0xFFFF}, nil
	case 0xDE: // SBC A, n
		n := data[1]
		return &Instruction{Mnemonic: fmt.Sprintf("SBC A, $%02X", n), Length: 2, Address: 0xFFFF}, nil
	case 0xDF: // RST 18H
		return &Instruction{Mnemonic: "RST 18H", Length: 1, Address: 0x0018}, nil

	// Conditional operations (0xE0-0xEF)
	case 0xE0: // RET PO
		return &Instruction{Mnemonic: "RET PO", Length: 1, Address: 0xFFFF}, nil
	case 0xE1: // POP HL
		return &Instruction{Mnemonic: "POP HL", Length: 1, Address: 0xFFFF}, nil
	case 0xE2: // JP PO, nn
		nn := uint16(data[2])<<8 | uint16(data[1])
		return &Instruction{Mnemonic: fmt.Sprintf("JP PO, $%04X", nn), Length: 3, Address: nn}, nil
	case 0xE3: // EX (SP), HL
		return &Instruction{Mnemonic: "EX (SP), HL", Length: 1, Address: 0xFFFF}, nil
	case 0xE4: // CALL PO, nn
		nn := uint16(data[2])<<8 | uint16(data[1])
		return &Instruction{Mnemonic: fmt.Sprintf("CALL PO, $%04X", nn), Length: 3, Address: nn}, nil
	case 0xE5: // PUSH HL
		return &Instruction{Mnemonic: "PUSH HL", Length: 1, Address: 0xFFFF}, nil
	case 0xE6: // AND n
		n := data[1]
		return &Instruction{Mnemonic: fmt.Sprintf("AND $%02X", n), Length: 2, Address: 0xFFFF}, nil
	case 0xE7: // RST 20H
		return &Instruction{Mnemonic: "RST 20H", Length: 1, Address: 0x0020}, nil
	case 0xE8: // RET PE
		return &Instruction{Mnemonic: "RET PE", Length: 1, Address: 0xFFFF}, nil
	case 0xE9: // JP (HL)
		return &Instruction{Mnemonic: "JP (HL)", Length: 1, Address: 0xFFFF}, nil
	case 0xEA: // JP PE, nn
		nn := uint16(data[2])<<8 | uint16(data[1])
		return &Instruction{Mnemonic: fmt.Sprintf("JP PE, $%04X", nn), Length: 3, Address: nn}, nil
	case 0xEB: // EX DE, HL
		return &Instruction{Mnemonic: "EX DE, HL", Length: 1, Address: 0xFFFF}, nil
	case 0xEC: // CALL PE, nn
		nn := uint16(data[2])<<8 | uint16(data[1])
		return &Instruction{Mnemonic: fmt.Sprintf("CALL PE, $%04X", nn), Length: 3, Address: nn}, nil
	case 0xED: // PREFIX ED
		// This should be handled in the main Decode function
		return &Instruction{Mnemonic: "PREFIX ED", Length: 1, Address: 0xFFFF}, nil
	case 0xEE: // XOR n
		n := data[1]
		return &Instruction{Mnemonic: fmt.Sprintf("XOR $%02X", n), Length: 2, Address: 0xFFFF}, nil
	case 0xEF: // RST 28H
		return &Instruction{Mnemonic: "RST 28H", Length: 1, Address: 0x0028}, nil

	// Conditional operations (0xF0-0xFF)
	case 0xF0: // RET P
		return &Instruction{Mnemonic: "RET P", Length: 1, Address: 0xFFFF}, nil
	case 0xF1: // POP AF
		return &Instruction{Mnemonic: "POP AF", Length: 1, Address: 0xFFFF}, nil
	case 0xF2: // JP P, nn
		nn := uint16(data[2])<<8 | uint16(data[1])
		return &Instruction{Mnemonic: fmt.Sprintf("JP P, $%04X", nn), Length: 3, Address: nn}, nil
	case 0xF3: // DI
		return &Instruction{Mnemonic: "DI", Length: 1, Address: 0xFFFF}, nil
	case 0xF4: // CALL P, nn
		nn := uint16(data[2])<<8 | uint16(data[1])
		return &Instruction{Mnemonic: fmt.Sprintf("CALL P, $%04X", nn), Length: 3, Address: nn}, nil
	case 0xF5: // PUSH AF
		return &Instruction{Mnemonic: "PUSH AF", Length: 1, Address: 0xFFFF}, nil
	case 0xF6: // OR n
		n := data[1]
		return &Instruction{Mnemonic: fmt.Sprintf("OR $%02X", n), Length: 2, Address: 0xFFFF}, nil
	case 0xF7: // RST 30H
		return &Instruction{Mnemonic: "RST 30H", Length: 1, Address: 0x0030}, nil
	case 0xF8: // RET M
		return &Instruction{Mnemonic: "RET M", Length: 1, Address: 0xFFFF}, nil
	case 0xF9: // LD SP, HL
		return &Instruction{Mnemonic: "LD SP, HL", Length: 1, Address: 0xFFFF}, nil
	case 0xFA: // JP M, nn
		nn := uint16(data[2])<<8 | uint16(data[1])
		return &Instruction{Mnemonic: fmt.Sprintf("JP M, $%04X", nn), Length: 3, Address: nn}, nil
	case 0xFB: // EI
		return &Instruction{Mnemonic: "EI", Length: 1, Address: 0xFFFF}, nil
	case 0xFC: // CALL M, nn
		nn := uint16(data[2])<<8 | uint16(data[1])
		return &Instruction{Mnemonic: fmt.Sprintf("CALL M, $%04X", nn), Length: 3, Address: nn}, nil
	case 0xFD: // PREFIX FD
		// This should be handled in the main Decode function
		return &Instruction{Mnemonic: "PREFIX FD", Length: 1, Address: 0xFFFF}, nil
	case 0xFE: // CP n
		n := data[1]
		return &Instruction{Mnemonic: fmt.Sprintf("CP $%02X", n), Length: 2, Address: 0xFFFF}, nil
	case 0xFF: // RST 38H
		return &Instruction{Mnemonic: "RST 38H", Length: 1, Address: 0x0038}, nil

	// Default case for unimplemented opcodes
	default:
		return &Instruction{Mnemonic: fmt.Sprintf("DB $%02X", opcode), Length: 1, Address: 0xFFFF}, nil
	}
}
