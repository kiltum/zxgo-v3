package disasm

import (
	"fmt"
)

// decodeED decodes ED-prefixed instructions
func (d *Disassembler) decodeED(data []byte) (*Instruction, error) {
	opcode := data[1]

	// Handle ED prefixed instructions
	switch opcode {
	// ED40-ED4F range
	case 0x40:
		return &Instruction{Mnemonic: "IN B, (C)", Length: 2, Address: 0xFFFF}, nil
	case 0x41:
		return &Instruction{Mnemonic: "OUT (C), B", Length: 2, Address: 0xFFFF}, nil
	case 0x42:
		return &Instruction{Mnemonic: "SBC HL, BC", Length: 2, Address: 0xFFFF}, nil
	case 0x43:
		nn := uint16(data[3])<<8 | uint16(data[2])
		return &Instruction{Mnemonic: fmt.Sprintf("LD ($%04X), BC", nn), Length: 4, Address: nn}, nil
	case 0x44:
		return &Instruction{Mnemonic: "NEG", Length: 2, Address: 0xFFFF}, nil
	case 0x45:
		return &Instruction{Mnemonic: "RETN", Length: 2, Address: 0xFFFF}, nil
	case 0x46:
		return &Instruction{Mnemonic: "IM 0", Length: 2, Address: 0xFFFF}, nil
	case 0x47:
		return &Instruction{Mnemonic: "LD I, A", Length: 2, Address: 0xFFFF}, nil
	case 0x48:
		return &Instruction{Mnemonic: "IN C, (C)", Length: 2, Address: 0xFFFF}, nil
	case 0x49:
		return &Instruction{Mnemonic: "OUT (C), C", Length: 2, Address: 0xFFFF}, nil
	case 0x4A:
		return &Instruction{Mnemonic: "ADC HL, BC", Length: 2, Address: 0xFFFF}, nil
	case 0x4B:
		nn := uint16(data[3])<<8 | uint16(data[2])
		return &Instruction{Mnemonic: fmt.Sprintf("LD BC, ($%04X)", nn), Length: 4, Address: nn}, nil
	case 0x4C:
		return &Instruction{Mnemonic: "NEG", Length: 2, Address: 0xFFFF}, nil // Duplicate of 0x44
	case 0x4D:
		return &Instruction{Mnemonic: "RETI", Length: 2, Address: 0xFFFF}, nil
	case 0x4E:
		return &Instruction{Mnemonic: "IM 0", Length: 2, Address: 0xFFFF}, nil // Duplicate of 0x46
	case 0x4F:
		return &Instruction{Mnemonic: "LD R, A", Length: 2, Address: 0xFFFF}, nil

	// ED50-ED5F range
	case 0x50:
		return &Instruction{Mnemonic: "IN D, (C)", Length: 2, Address: 0xFFFF}, nil
	case 0x51:
		return &Instruction{Mnemonic: "OUT (C), D", Length: 2, Address: 0xFFFF}, nil
	case 0x52:
		return &Instruction{Mnemonic: "SBC HL, DE", Length: 2, Address: 0xFFFF}, nil
	case 0x53:
		nn := uint16(data[3])<<8 | uint16(data[2])
		return &Instruction{Mnemonic: fmt.Sprintf("LD ($%04X), DE", nn), Length: 4, Address: nn}, nil
	case 0x54:
		return &Instruction{Mnemonic: "NEG", Length: 2, Address: 0xFFFF}, nil // Duplicate of 0x44
	case 0x55:
		return &Instruction{Mnemonic: "RETN", Length: 2, Address: 0xFFFF}, nil // Duplicate of 0x45
	case 0x56:
		return &Instruction{Mnemonic: "IM 1", Length: 2, Address: 0xFFFF}, nil
	case 0x57:
		return &Instruction{Mnemonic: "LD A, I", Length: 2, Address: 0xFFFF}, nil
	case 0x58:
		return &Instruction{Mnemonic: "IN E, (C)", Length: 2, Address: 0xFFFF}, nil
	case 0x59:
		return &Instruction{Mnemonic: "OUT (C), E", Length: 2, Address: 0xFFFF}, nil
	case 0x5A:
		return &Instruction{Mnemonic: "ADC HL, DE", Length: 2, Address: 0xFFFF}, nil
	case 0x5B:
		nn := uint16(data[3])<<8 | uint16(data[2])
		return &Instruction{Mnemonic: fmt.Sprintf("LD DE, ($%04X)", nn), Length: 4, Address: nn}, nil
	case 0x5C:
		return &Instruction{Mnemonic: "NEG", Length: 2, Address: 0xFFFF}, nil // Duplicate of 0x44
	case 0x5D:
		return &Instruction{Mnemonic: "RETN", Length: 2, Address: 0xFFFF}, nil // Duplicate of 0x45
	case 0x5E:
		return &Instruction{Mnemonic: "IM 2", Length: 2, Address: 0xFFFF}, nil
	case 0x5F:
		return &Instruction{Mnemonic: "LD A, R", Length: 2, Address: 0xFFFF}, nil

	// ED60-ED6F range
	case 0x60:
		return &Instruction{Mnemonic: "IN H, (C)", Length: 2, Address: 0xFFFF}, nil
	case 0x61:
		return &Instruction{Mnemonic: "OUT (C), H", Length: 2, Address: 0xFFFF}, nil
	case 0x62:
		return &Instruction{Mnemonic: "SBC HL, HL", Length: 2, Address: 0xFFFF}, nil
	case 0x63:
		nn := uint16(data[3])<<8 | uint16(data[2])
		return &Instruction{Mnemonic: fmt.Sprintf("LD ($%04X), HL", nn), Length: 4, Address: nn}, nil
	case 0x64:
		return &Instruction{Mnemonic: "NEG", Length: 2, Address: 0xFFFF}, nil // Duplicate of 0x44
	case 0x65:
		return &Instruction{Mnemonic: "RETN", Length: 2, Address: 0xFFFF}, nil // Duplicate of 0x45
	case 0x66:
		return &Instruction{Mnemonic: "IM 0", Length: 2, Address: 0xFFFF}, nil // Duplicate of 0x46
	case 0x67:
		return &Instruction{Mnemonic: "RRD", Length: 2, Address: 0xFFFF}, nil
	case 0x68:
		return &Instruction{Mnemonic: "IN L, (C)", Length: 2, Address: 0xFFFF}, nil
	case 0x69:
		return &Instruction{Mnemonic: "OUT (C), L", Length: 2, Address: 0xFFFF}, nil
	case 0x6A:
		return &Instruction{Mnemonic: "ADC HL, HL", Length: 2, Address: 0xFFFF}, nil
	case 0x6B:
		nn := uint16(data[3])<<8 | uint16(data[2])
		return &Instruction{Mnemonic: fmt.Sprintf("LD HL, ($%04X)", nn), Length: 4, Address: nn}, nil
	case 0x6C:
		return &Instruction{Mnemonic: "NEG", Length: 2, Address: 0xFFFF}, nil // Duplicate of 0x44
	case 0x6D:
		return &Instruction{Mnemonic: "RETN", Length: 2, Address: 0xFFFF}, nil // Duplicate of 0x45
	case 0x6E:
		return &Instruction{Mnemonic: "IM 0", Length: 2, Address: 0xFFFF}, nil // Duplicate of 0x46
	case 0x6F:
		return &Instruction{Mnemonic: "RLD", Length: 2, Address: 0xFFFF}, nil

	// ED70-ED7F range
	case 0x70:
		return &Instruction{Mnemonic: "IN F, (C)", Length: 2, Address: 0xFFFF}, nil
	case 0x71:
		return &Instruction{Mnemonic: "OUT (C), 0", Length: 2, Address: 0xFFFF}, nil
	case 0x72:
		return &Instruction{Mnemonic: "SBC HL, SP", Length: 2, Address: 0xFFFF}, nil
	case 0x73:
		nn := uint16(data[3])<<8 | uint16(data[2])
		return &Instruction{Mnemonic: fmt.Sprintf("LD ($%04X), SP", nn), Length: 4, Address: nn}, nil
	case 0x74:
		return &Instruction{Mnemonic: "NEG", Length: 2, Address: 0xFFFF}, nil // Duplicate of 0x44
	case 0x75:
		return &Instruction{Mnemonic: "RETN", Length: 2, Address: 0xFFFF}, nil // Duplicate of 0x45
	case 0x76:
		return &Instruction{Mnemonic: "IM 1", Length: 2, Address: 0xFFFF}, nil // Duplicate of 0x56
	case 0x77:
		return &Instruction{Mnemonic: "NOP", Length: 2, Address: 0xFFFF}, nil
	case 0x78:
		return &Instruction{Mnemonic: "IN A, (C)", Length: 2, Address: 0xFFFF}, nil
	case 0x79:
		return &Instruction{Mnemonic: "OUT (C), A", Length: 2, Address: 0xFFFF}, nil
	case 0x7A:
		return &Instruction{Mnemonic: "ADC HL, SP", Length: 2, Address: 0xFFFF}, nil
	case 0x7B:
		nn := uint16(data[3])<<8 | uint16(data[2])
		return &Instruction{Mnemonic: fmt.Sprintf("LD SP, ($%04X)", nn), Length: 4, Address: nn}, nil
	case 0x7C:
		return &Instruction{Mnemonic: "NEG", Length: 2, Address: 0xFFFF}, nil // Duplicate of 0x44
	case 0x7D:
		return &Instruction{Mnemonic: "RETN", Length: 2, Address: 0xFFFF}, nil // Duplicate of 0x45
	case 0x7E:
		return &Instruction{Mnemonic: "IM 2", Length: 2, Address: 0xFFFF}, nil // Duplicate of 0x5E
	case 0x7F:
		return &Instruction{Mnemonic: "NOP", Length: 2, Address: 0xFFFF}, nil // Duplicate of 0x77

	// Block instructions
	case 0xA0:
		return &Instruction{Mnemonic: "LDI", Length: 2, Address: 0xFFFF}, nil
	case 0xA1:
		return &Instruction{Mnemonic: "CPI", Length: 2, Address: 0xFFFF}, nil
	case 0xA2:
		return &Instruction{Mnemonic: "INI", Length: 2, Address: 0xFFFF}, nil
	case 0xA3:
		return &Instruction{Mnemonic: "OUTI", Length: 2, Address: 0xFFFF}, nil
	case 0xA8:
		return &Instruction{Mnemonic: "LDD", Length: 2, Address: 0xFFFF}, nil
	case 0xA9:
		return &Instruction{Mnemonic: "CPD", Length: 2, Address: 0xFFFF}, nil
	case 0xAA:
		return &Instruction{Mnemonic: "IND", Length: 2, Address: 0xFFFF}, nil
	case 0xAB:
		return &Instruction{Mnemonic: "OUTD", Length: 2, Address: 0xFFFF}, nil
	case 0xB0:
		return &Instruction{Mnemonic: "LDIR", Length: 2, Address: 0xFFFF}, nil
	case 0xB1:
		return &Instruction{Mnemonic: "CPIR", Length: 2, Address: 0xFFFF}, nil
	case 0xB2:
		return &Instruction{Mnemonic: "INIR", Length: 2, Address: 0xFFFF}, nil
	case 0xB3:
		return &Instruction{Mnemonic: "OTIR", Length: 2, Address: 0xFFFF}, nil
	case 0xB8:
		return &Instruction{Mnemonic: "LDDR", Length: 2, Address: 0xFFFF}, nil
	case 0xB9:
		return &Instruction{Mnemonic: "CPDR", Length: 2, Address: 0xFFFF}, nil
	case 0xBA:
		return &Instruction{Mnemonic: "INDR", Length: 2, Address: 0xFFFF}, nil
	case 0xBB:
		return &Instruction{Mnemonic: "OTDR", Length: 2, Address: 0xFFFF}, nil

	default:
		return &Instruction{Mnemonic: fmt.Sprintf("ED $%02X", opcode), Length: 2, Address: 0xFFFF}, nil
	}
}
