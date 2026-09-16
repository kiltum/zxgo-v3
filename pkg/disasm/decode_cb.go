package disasm

import (
	"fmt"
)

// decodeCB decodes CB-prefixed instructions
func (d *Disassembler) decodeCB(data []byte) (*Instruction, error) {
	opcode := data[1]

	// Handle CB prefixed instructions
	switch opcode {
	// RLC r / RLC (HL)
	case 0x00:
		return &Instruction{Mnemonic: "RLC B", Length: 2, Address: 0xFFFF}, nil
	case 0x01:
		return &Instruction{Mnemonic: "RLC C", Length: 2, Address: 0xFFFF}, nil
	case 0x02:
		return &Instruction{Mnemonic: "RLC D", Length: 2, Address: 0xFFFF}, nil
	case 0x03:
		return &Instruction{Mnemonic: "RLC E", Length: 2, Address: 0xFFFF}, nil
	case 0x04:
		return &Instruction{Mnemonic: "RLC H", Length: 2, Address: 0xFFFF}, nil
	case 0x05:
		return &Instruction{Mnemonic: "RLC L", Length: 2, Address: 0xFFFF}, nil
	case 0x06:
		return &Instruction{Mnemonic: "RLC (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0x07:
		return &Instruction{Mnemonic: "RLC A", Length: 2, Address: 0xFFFF}, nil

	// RRC r / RRC (HL)
	case 0x08:
		return &Instruction{Mnemonic: "RRC B", Length: 2, Address: 0xFFFF}, nil
	case 0x09:
		return &Instruction{Mnemonic: "RRC C", Length: 2, Address: 0xFFFF}, nil
	case 0x0A:
		return &Instruction{Mnemonic: "RRC D", Length: 2, Address: 0xFFFF}, nil
	case 0x0B:
		return &Instruction{Mnemonic: "RRC E", Length: 2, Address: 0xFFFF}, nil
	case 0x0C:
		return &Instruction{Mnemonic: "RRC H", Length: 2, Address: 0xFFFF}, nil
	case 0x0D:
		return &Instruction{Mnemonic: "RRC L", Length: 2, Address: 0xFFFF}, nil
	case 0x0E:
		return &Instruction{Mnemonic: "RRC (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0x0F:
		return &Instruction{Mnemonic: "RRC A", Length: 2, Address: 0xFFFF}, nil

	// RL r / RL (HL)
	case 0x10:
		return &Instruction{Mnemonic: "RL B", Length: 2, Address: 0xFFFF}, nil
	case 0x11:
		return &Instruction{Mnemonic: "RL C", Length: 2, Address: 0xFFFF}, nil
	case 0x12:
		return &Instruction{Mnemonic: "RL D", Length: 2, Address: 0xFFFF}, nil
	case 0x13:
		return &Instruction{Mnemonic: "RL E", Length: 2, Address: 0xFFFF}, nil
	case 0x14:
		return &Instruction{Mnemonic: "RL H", Length: 2, Address: 0xFFFF}, nil
	case 0x15:
		return &Instruction{Mnemonic: "RL L", Length: 2, Address: 0xFFFF}, nil
	case 0x16:
		return &Instruction{Mnemonic: "RL (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0x17:
		return &Instruction{Mnemonic: "RL A", Length: 2, Address: 0xFFFF}, nil

	// RR r / RR (HL)
	case 0x18:
		return &Instruction{Mnemonic: "RR B", Length: 2, Address: 0xFFFF}, nil
	case 0x19:
		return &Instruction{Mnemonic: "RR C", Length: 2, Address: 0xFFFF}, nil
	case 0x1A:
		return &Instruction{Mnemonic: "RR D", Length: 2, Address: 0xFFFF}, nil
	case 0x1B:
		return &Instruction{Mnemonic: "RR E", Length: 2, Address: 0xFFFF}, nil
	case 0x1C:
		return &Instruction{Mnemonic: "RR H", Length: 2, Address: 0xFFFF}, nil
	case 0x1D:
		return &Instruction{Mnemonic: "RR L", Length: 2, Address: 0xFFFF}, nil
	case 0x1E:
		return &Instruction{Mnemonic: "RR (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0x1F:
		return &Instruction{Mnemonic: "RR A", Length: 2, Address: 0xFFFF}, nil

	// SLA r / SLA (HL)
	case 0x20:
		return &Instruction{Mnemonic: "SLA B", Length: 2, Address: 0xFFFF}, nil
	case 0x21:
		return &Instruction{Mnemonic: "SLA C", Length: 2, Address: 0xFFFF}, nil
	case 0x22:
		return &Instruction{Mnemonic: "SLA D", Length: 2, Address: 0xFFFF}, nil
	case 0x23:
		return &Instruction{Mnemonic: "SLA E", Length: 2, Address: 0xFFFF}, nil
	case 0x24:
		return &Instruction{Mnemonic: "SLA H", Length: 2, Address: 0xFFFF}, nil
	case 0x25:
		return &Instruction{Mnemonic: "SLA L", Length: 2, Address: 0xFFFF}, nil
	case 0x26:
		return &Instruction{Mnemonic: "SLA (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0x27:
		return &Instruction{Mnemonic: "SLA A", Length: 2, Address: 0xFFFF}, nil

	// SRA r / SRA (HL)
	case 0x28:
		return &Instruction{Mnemonic: "SRA B", Length: 2, Address: 0xFFFF}, nil
	case 0x29:
		return &Instruction{Mnemonic: "SRA C", Length: 2, Address: 0xFFFF}, nil
	case 0x2A:
		return &Instruction{Mnemonic: "SRA D", Length: 2, Address: 0xFFFF}, nil
	case 0x2B:
		return &Instruction{Mnemonic: "SRA E", Length: 2, Address: 0xFFFF}, nil
	case 0x2C:
		return &Instruction{Mnemonic: "SRA H", Length: 2, Address: 0xFFFF}, nil
	case 0x2D:
		return &Instruction{Mnemonic: "SRA L", Length: 2, Address: 0xFFFF}, nil
	case 0x2E:
		return &Instruction{Mnemonic: "SRA (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0x2F:
		return &Instruction{Mnemonic: "SRA A", Length: 2, Address: 0xFFFF}, nil

	// SLL r / SLL (HL) (Undocumented)
	case 0x30:
		return &Instruction{Mnemonic: "SLL B", Length: 2, Address: 0xFFFF}, nil
	case 0x31:
		return &Instruction{Mnemonic: "SLL C", Length: 2, Address: 0xFFFF}, nil
	case 0x32:
		return &Instruction{Mnemonic: "SLL D", Length: 2, Address: 0xFFFF}, nil
	case 0x33:
		return &Instruction{Mnemonic: "SLL E", Length: 2, Address: 0xFFFF}, nil
	case 0x34:
		return &Instruction{Mnemonic: "SLL H", Length: 2, Address: 0xFFFF}, nil
	case 0x35:
		return &Instruction{Mnemonic: "SLL L", Length: 2, Address: 0xFFFF}, nil
	case 0x36:
		return &Instruction{Mnemonic: "SLL (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0x37:
		return &Instruction{Mnemonic: "SLL A", Length: 2, Address: 0xFFFF}, nil

	// SRL r / SRL (HL)
	case 0x38:
		return &Instruction{Mnemonic: "SRL B", Length: 2, Address: 0xFFFF}, nil
	case 0x39:
		return &Instruction{Mnemonic: "SRL C", Length: 2, Address: 0xFFFF}, nil
	case 0x3A:
		return &Instruction{Mnemonic: "SRL D", Length: 2, Address: 0xFFFF}, nil
	case 0x3B:
		return &Instruction{Mnemonic: "SRL E", Length: 2, Address: 0xFFFF}, nil
	case 0x3C:
		return &Instruction{Mnemonic: "SRL H", Length: 2, Address: 0xFFFF}, nil
	case 0x3D:
		return &Instruction{Mnemonic: "SRL L", Length: 2, Address: 0xFFFF}, nil
	case 0x3E:
		return &Instruction{Mnemonic: "SRL (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0x3F:
		return &Instruction{Mnemonic: "SRL A", Length: 2, Address: 0xFFFF}, nil

	// BIT b, r / BIT b, (HL)
	case 0x40: // BIT 0, B
		return &Instruction{Mnemonic: "BIT 0, B", Length: 2, Address: 0xFFFF}, nil
	case 0x41: // BIT 0, C
		return &Instruction{Mnemonic: "BIT 0, C", Length: 2, Address: 0xFFFF}, nil
	case 0x42: // BIT 0, D
		return &Instruction{Mnemonic: "BIT 0, D", Length: 2, Address: 0xFFFF}, nil
	case 0x43: // BIT 0, E
		return &Instruction{Mnemonic: "BIT 0, E", Length: 2, Address: 0xFFFF}, nil
	case 0x44: // BIT 0, H
		return &Instruction{Mnemonic: "BIT 0, H", Length: 2, Address: 0xFFFF}, nil
	case 0x45: // BIT 0, L
		return &Instruction{Mnemonic: "BIT 0, L", Length: 2, Address: 0xFFFF}, nil
	case 0x46: // BIT 0, (HL)
		return &Instruction{Mnemonic: "BIT 0, (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0x47: // BIT 0, A
		return &Instruction{Mnemonic: "BIT 0, A", Length: 2, Address: 0xFFFF}, nil
	case 0x48: // BIT 1, B
		return &Instruction{Mnemonic: "BIT 1, B", Length: 2, Address: 0xFFFF}, nil
	case 0x49: // BIT 1, C
		return &Instruction{Mnemonic: "BIT 1, C", Length: 2, Address: 0xFFFF}, nil
	case 0x4A: // BIT 1, D
		return &Instruction{Mnemonic: "BIT 1, D", Length: 2, Address: 0xFFFF}, nil
	case 0x4B: // BIT 1, E
		return &Instruction{Mnemonic: "BIT 1, E", Length: 2, Address: 0xFFFF}, nil
	case 0x4C: // BIT 1, H
		return &Instruction{Mnemonic: "BIT 1, H", Length: 2, Address: 0xFFFF}, nil
	case 0x4D: // BIT 1, L
		return &Instruction{Mnemonic: "BIT 1, L", Length: 2, Address: 0xFFFF}, nil
	case 0x4E: // BIT 1, (HL)
		return &Instruction{Mnemonic: "BIT 1, (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0x4F: // BIT 1, A
		return &Instruction{Mnemonic: "BIT 1, A", Length: 2, Address: 0xFFFF}, nil
	case 0x50: // BIT 2, B
		return &Instruction{Mnemonic: "BIT 2, B", Length: 2, Address: 0xFFFF}, nil
	case 0x51: // BIT 2, C
		return &Instruction{Mnemonic: "BIT 2, C", Length: 2, Address: 0xFFFF}, nil
	case 0x52: // BIT 2, D
		return &Instruction{Mnemonic: "BIT 2, D", Length: 2, Address: 0xFFFF}, nil
	case 0x53: // BIT 2, E
		return &Instruction{Mnemonic: "BIT 2, E", Length: 2, Address: 0xFFFF}, nil
	case 0x54: // BIT 2, H
		return &Instruction{Mnemonic: "BIT 2, H", Length: 2, Address: 0xFFFF}, nil
	case 0x55: // BIT 2, L
		return &Instruction{Mnemonic: "BIT 2, L", Length: 2, Address: 0xFFFF}, nil
	case 0x56: // BIT 2, (HL)
		return &Instruction{Mnemonic: "BIT 2, (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0x57: // BIT 2, A
		return &Instruction{Mnemonic: "BIT 2, A", Length: 2, Address: 0xFFFF}, nil
	case 0x58: // BIT 3, B
		return &Instruction{Mnemonic: "BIT 3, B", Length: 2, Address: 0xFFFF}, nil
	case 0x59: // BIT 3, C
		return &Instruction{Mnemonic: "BIT 3, C", Length: 2, Address: 0xFFFF}, nil
	case 0x5A: // BIT 3, D
		return &Instruction{Mnemonic: "BIT 3, D", Length: 2, Address: 0xFFFF}, nil
	case 0x5B: // BIT 3, E
		return &Instruction{Mnemonic: "BIT 3, E", Length: 2, Address: 0xFFFF}, nil
	case 0x5C: // BIT 3, H
		return &Instruction{Mnemonic: "BIT 3, H", Length: 2, Address: 0xFFFF}, nil
	case 0x5D: // BIT 3, L
		return &Instruction{Mnemonic: "BIT 3, L", Length: 2, Address: 0xFFFF}, nil
	case 0x5E: // BIT 3, (HL)
		return &Instruction{Mnemonic: "BIT 3, (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0x5F: // BIT 3, A
		return &Instruction{Mnemonic: "BIT 3, A", Length: 2, Address: 0xFFFF}, nil
	case 0x60: // BIT 4, B
		return &Instruction{Mnemonic: "BIT 4, B", Length: 2, Address: 0xFFFF}, nil
	case 0x61: // BIT 4, C
		return &Instruction{Mnemonic: "BIT 4, C", Length: 2, Address: 0xFFFF}, nil
	case 0x62: // BIT 4, D
		return &Instruction{Mnemonic: "BIT 4, D", Length: 2, Address: 0xFFFF}, nil
	case 0x63: // BIT 4, E
		return &Instruction{Mnemonic: "BIT 4, E", Length: 2, Address: 0xFFFF}, nil
	case 0x64: // BIT 4, H
		return &Instruction{Mnemonic: "BIT 4, H", Length: 2, Address: 0xFFFF}, nil
	case 0x65: // BIT 4, L
		return &Instruction{Mnemonic: "BIT 4, L", Length: 2, Address: 0xFFFF}, nil
	case 0x66: // BIT 4, (HL)
		return &Instruction{Mnemonic: "BIT 4, (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0x67: // BIT 4, A
		return &Instruction{Mnemonic: "BIT 4, A", Length: 2, Address: 0xFFFF}, nil
	case 0x68: // BIT 5, B
		return &Instruction{Mnemonic: "BIT 5, B", Length: 2, Address: 0xFFFF}, nil
	case 0x69: // BIT 5, C
		return &Instruction{Mnemonic: "BIT 5, C", Length: 2, Address: 0xFFFF}, nil
	case 0x6A: // BIT 5, D
		return &Instruction{Mnemonic: "BIT 5, D", Length: 2, Address: 0xFFFF}, nil
	case 0x6B: // BIT 5, E
		return &Instruction{Mnemonic: "BIT 5, E", Length: 2, Address: 0xFFFF}, nil
	case 0x6C: // BIT 5, H
		return &Instruction{Mnemonic: "BIT 5, H", Length: 2, Address: 0xFFFF}, nil
	case 0x6D: // BIT 5, L
		return &Instruction{Mnemonic: "BIT 5, L", Length: 2, Address: 0xFFFF}, nil
	case 0x6E: // BIT 5, (HL)
		return &Instruction{Mnemonic: "BIT 5, (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0x6F: // BIT 5, A
		return &Instruction{Mnemonic: "BIT 5, A", Length: 2, Address: 0xFFFF}, nil
	case 0x70: // BIT 6, B
		return &Instruction{Mnemonic: "BIT 6, B", Length: 2, Address: 0xFFFF}, nil
	case 0x71: // BIT 6, C
		return &Instruction{Mnemonic: "BIT 6, C", Length: 2, Address: 0xFFFF}, nil
	case 0x72: // BIT 6, D
		return &Instruction{Mnemonic: "BIT 6, D", Length: 2, Address: 0xFFFF}, nil
	case 0x73: // BIT 6, E
		return &Instruction{Mnemonic: "BIT 6, E", Length: 2, Address: 0xFFFF}, nil
	case 0x74: // BIT 6, H
		return &Instruction{Mnemonic: "BIT 6, H", Length: 2, Address: 0xFFFF}, nil
	case 0x75: // BIT 6, L
		return &Instruction{Mnemonic: "BIT 6, L", Length: 2, Address: 0xFFFF}, nil
	case 0x76: // BIT 6, (HL)
		return &Instruction{Mnemonic: "BIT 6, (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0x77: // BIT 6, A
		return &Instruction{Mnemonic: "BIT 6, A", Length: 2, Address: 0xFFFF}, nil
	case 0x78: // BIT 7, B
		return &Instruction{Mnemonic: "BIT 7, B", Length: 2, Address: 0xFFFF}, nil
	case 0x79: // BIT 7, C
		return &Instruction{Mnemonic: "BIT 7, C", Length: 2, Address: 0xFFFF}, nil
	case 0x7A: // BIT 7, D
		return &Instruction{Mnemonic: "BIT 7, D", Length: 2, Address: 0xFFFF}, nil
	case 0x7B: // BIT 7, E
		return &Instruction{Mnemonic: "BIT 7, E", Length: 2, Address: 0xFFFF}, nil
	case 0x7C: // BIT 7, H
		return &Instruction{Mnemonic: "BIT 7, H", Length: 2, Address: 0xFFFF}, nil
	case 0x7D: // BIT 7, L
		return &Instruction{Mnemonic: "BIT 7, L", Length: 2, Address: 0xFFFF}, nil
	case 0x7E: // BIT 7, (HL)
		return &Instruction{Mnemonic: "BIT 7, (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0x7F: // BIT 7, A
		return &Instruction{Mnemonic: "BIT 7, A", Length: 2, Address: 0xFFFF}, nil

	// RES b, r / RES b, (HL)
	case 0x80: // RES 0, B
		return &Instruction{Mnemonic: "RES 0, B", Length: 2, Address: 0xFFFF}, nil
	case 0x81: // RES 0, C
		return &Instruction{Mnemonic: "RES 0, C", Length: 2, Address: 0xFFFF}, nil
	case 0x82: // RES 0, D
		return &Instruction{Mnemonic: "RES 0, D", Length: 2, Address: 0xFFFF}, nil
	case 0x83: // RES 0, E
		return &Instruction{Mnemonic: "RES 0, E", Length: 2, Address: 0xFFFF}, nil
	case 0x84: // RES 0, H
		return &Instruction{Mnemonic: "RES 0, H", Length: 2, Address: 0xFFFF}, nil
	case 0x85: // RES 0, L
		return &Instruction{Mnemonic: "RES 0, L", Length: 2, Address: 0xFFFF}, nil
	case 0x86: // RES 0, (HL)
		return &Instruction{Mnemonic: "RES 0, (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0x87: // RES 0, A
		return &Instruction{Mnemonic: "RES 0, A", Length: 2, Address: 0xFFFF}, nil
	case 0x88: // RES 1, B
		return &Instruction{Mnemonic: "RES 1, B", Length: 2, Address: 0xFFFF}, nil
	case 0x89: // RES 1, C
		return &Instruction{Mnemonic: "RES 1, C", Length: 2, Address: 0xFFFF}, nil
	case 0x8A: // RES 1, D
		return &Instruction{Mnemonic: "RES 1, D", Length: 2, Address: 0xFFFF}, nil
	case 0x8B: // RES 1, E
		return &Instruction{Mnemonic: "RES 1, E", Length: 2, Address: 0xFFFF}, nil
	case 0x8C: // RES 1, H
		return &Instruction{Mnemonic: "RES 1, H", Length: 2, Address: 0xFFFF}, nil
	case 0x8D: // RES 1, L
		return &Instruction{Mnemonic: "RES 1, L", Length: 2, Address: 0xFFFF}, nil
	case 0x8E: // RES 1, (HL)
		return &Instruction{Mnemonic: "RES 1, (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0x8F: // RES 1, A
		return &Instruction{Mnemonic: "RES 1, A", Length: 2, Address: 0xFFFF}, nil
	case 0x90: // RES 2, B
		return &Instruction{Mnemonic: "RES 2, B", Length: 2, Address: 0xFFFF}, nil
	case 0x91: // RES 2, C
		return &Instruction{Mnemonic: "RES 2, C", Length: 2, Address: 0xFFFF}, nil
	case 0x92: // RES 2, D
		return &Instruction{Mnemonic: "RES 2, D", Length: 2, Address: 0xFFFF}, nil
	case 0x93: // RES 2, E
		return &Instruction{Mnemonic: "RES 2, E", Length: 2, Address: 0xFFFF}, nil
	case 0x94: // RES 2, H
		return &Instruction{Mnemonic: "RES 2, H", Length: 2, Address: 0xFFFF}, nil
	case 0x95: // RES 2, L
		return &Instruction{Mnemonic: "RES 2, L", Length: 2, Address: 0xFFFF}, nil
	case 0x96: // RES 2, (HL)
		return &Instruction{Mnemonic: "RES 2, (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0x97: // RES 2, A
		return &Instruction{Mnemonic: "RES 2, A", Length: 2, Address: 0xFFFF}, nil
	case 0x98: // RES 3, B
		return &Instruction{Mnemonic: "RES 3, B", Length: 2, Address: 0xFFFF}, nil
	case 0x99: // RES 3, C
		return &Instruction{Mnemonic: "RES 3, C", Length: 2, Address: 0xFFFF}, nil
	case 0x9A: // RES 3, D
		return &Instruction{Mnemonic: "RES 3, D", Length: 2, Address: 0xFFFF}, nil
	case 0x9B: // RES 3, E
		return &Instruction{Mnemonic: "RES 3, E", Length: 2, Address: 0xFFFF}, nil
	case 0x9C: // RES 3, H
		return &Instruction{Mnemonic: "RES 3, H", Length: 2, Address: 0xFFFF}, nil
	case 0x9D: // RES 3, L
		return &Instruction{Mnemonic: "RES 3, L", Length: 2, Address: 0xFFFF}, nil
	case 0x9E: // RES 3, (HL)
		return &Instruction{Mnemonic: "RES 3, (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0x9F: // RES 3, A
		return &Instruction{Mnemonic: "RES 3, A", Length: 2, Address: 0xFFFF}, nil
	case 0xA0: // RES 4, B
		return &Instruction{Mnemonic: "RES 4, B", Length: 2, Address: 0xFFFF}, nil
	case 0xA1: // RES 4, C
		return &Instruction{Mnemonic: "RES 4, C", Length: 2, Address: 0xFFFF}, nil
	case 0xA2: // RES 4, D
		return &Instruction{Mnemonic: "RES 4, D", Length: 2, Address: 0xFFFF}, nil
	case 0xA3: // RES 4, E
		return &Instruction{Mnemonic: "RES 4, E", Length: 2, Address: 0xFFFF}, nil
	case 0xA4: // RES 4, H
		return &Instruction{Mnemonic: "RES 4, H", Length: 2, Address: 0xFFFF}, nil
	case 0xA5: // RES 4, L
		return &Instruction{Mnemonic: "RES 4, L", Length: 2, Address: 0xFFFF}, nil
	case 0xA6: // RES 4, (HL)
		return &Instruction{Mnemonic: "RES 4, (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0xA7: // RES 4, A
		return &Instruction{Mnemonic: "RES 4, A", Length: 2, Address: 0xFFFF}, nil
	case 0xA8: // RES 5, B
		return &Instruction{Mnemonic: "RES 5, B", Length: 2, Address: 0xFFFF}, nil
	case 0xA9: // RES 5, C
		return &Instruction{Mnemonic: "RES 5, C", Length: 2, Address: 0xFFFF}, nil
	case 0xAA: // RES 5, D
		return &Instruction{Mnemonic: "RES 5, D", Length: 2, Address: 0xFFFF}, nil
	case 0xAB: // RES 5, E
		return &Instruction{Mnemonic: "RES 5, E", Length: 2, Address: 0xFFFF}, nil
	case 0xAC: // RES 5, H
		return &Instruction{Mnemonic: "RES 5, H", Length: 2, Address: 0xFFFF}, nil
	case 0xAD: // RES 5, L
		return &Instruction{Mnemonic: "RES 5, L", Length: 2, Address: 0xFFFF}, nil
	case 0xAE: // RES 5, (HL)
		return &Instruction{Mnemonic: "RES 5, (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0xAF: // RES 5, A
		return &Instruction{Mnemonic: "RES 5, A", Length: 2, Address: 0xFFFF}, nil
	case 0xB0: // RES 6, B
		return &Instruction{Mnemonic: "RES 6, B", Length: 2, Address: 0xFFFF}, nil
	case 0xB1: // RES 6, C
		return &Instruction{Mnemonic: "RES 6, C", Length: 2, Address: 0xFFFF}, nil
	case 0xB2: // RES 6, D
		return &Instruction{Mnemonic: "RES 6, D", Length: 2, Address: 0xFFFF}, nil
	case 0xB3: // RES 6, E
		return &Instruction{Mnemonic: "RES 6, E", Length: 2, Address: 0xFFFF}, nil
	case 0xB4: // RES 6, H
		return &Instruction{Mnemonic: "RES 6, H", Length: 2, Address: 0xFFFF}, nil
	case 0xB5: // RES 6, L
		return &Instruction{Mnemonic: "RES 6, L", Length: 2, Address: 0xFFFF}, nil
	case 0xB6: // RES 6, (HL)
		return &Instruction{Mnemonic: "RES 6, (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0xB7: // RES 6, A
		return &Instruction{Mnemonic: "RES 6, A", Length: 2, Address: 0xFFFF}, nil
	case 0xB8: // RES 7, B
		return &Instruction{Mnemonic: "RES 7, B", Length: 2, Address: 0xFFFF}, nil
	case 0xB9: // RES 7, C
		return &Instruction{Mnemonic: "RES 7, C", Length: 2, Address: 0xFFFF}, nil
	case 0xBA: // RES 7, D
		return &Instruction{Mnemonic: "RES 7, D", Length: 2, Address: 0xFFFF}, nil
	case 0xBB: // RES 7, E
		return &Instruction{Mnemonic: "RES 7, E", Length: 2, Address: 0xFFFF}, nil
	case 0xBC: // RES 7, H
		return &Instruction{Mnemonic: "RES 7, H", Length: 2, Address: 0xFFFF}, nil
	case 0xBD: // RES 7, L
		return &Instruction{Mnemonic: "RES 7, L", Length: 2, Address: 0xFFFF}, nil
	case 0xBE: // RES 7, (HL)
		return &Instruction{Mnemonic: "RES 7, (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0xBF: // RES 7, A
		return &Instruction{Mnemonic: "RES 7, A", Length: 2, Address: 0xFFFF}, nil

	// SET b, r / SET b, (HL)
	case 0xC0: // SET 0, B
		return &Instruction{Mnemonic: "SET 0, B", Length: 2, Address: 0xFFFF}, nil
	case 0xC1: // SET 0, C
		return &Instruction{Mnemonic: "SET 0, C", Length: 2, Address: 0xFFFF}, nil
	case 0xC2: // SET 0, D
		return &Instruction{Mnemonic: "SET 0, D", Length: 2, Address: 0xFFFF}, nil
	case 0xC3: // SET 0, E
		return &Instruction{Mnemonic: "SET 0, E", Length: 2, Address: 0xFFFF}, nil
	case 0xC4: // SET 0, H
		return &Instruction{Mnemonic: "SET 0, H", Length: 2, Address: 0xFFFF}, nil
	case 0xC5: // SET 0, L
		return &Instruction{Mnemonic: "SET 0, L", Length: 2, Address: 0xFFFF}, nil
	case 0xC6: // SET 0, (HL)
		return &Instruction{Mnemonic: "SET 0, (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0xC7: // SET 0, A
		return &Instruction{Mnemonic: "SET 0, A", Length: 2, Address: 0xFFFF}, nil
	case 0xC8: // SET 1, B
		return &Instruction{Mnemonic: "SET 1, B", Length: 2, Address: 0xFFFF}, nil
	case 0xC9: // SET 1, C
		return &Instruction{Mnemonic: "SET 1, C", Length: 2, Address: 0xFFFF}, nil
	case 0xCA: // SET 1, D
		return &Instruction{Mnemonic: "SET 1, D", Length: 2, Address: 0xFFFF}, nil
	case 0xCB: // SET 1, E
		return &Instruction{Mnemonic: "SET 1, E", Length: 2, Address: 0xFFFF}, nil
	case 0xCC: // SET 1, H
		return &Instruction{Mnemonic: "SET 1, H", Length: 2, Address: 0xFFFF}, nil
	case 0xCD: // SET 1, L
		return &Instruction{Mnemonic: "SET 1, L", Length: 2, Address: 0xFFFF}, nil
	case 0xCE: // SET 1, (HL)
		return &Instruction{Mnemonic: "SET 1, (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0xCF: // SET 1, A
		return &Instruction{Mnemonic: "SET 1, A", Length: 2, Address: 0xFFFF}, nil
	case 0xD0: // SET 2, B
		return &Instruction{Mnemonic: "SET 2, B", Length: 2, Address: 0xFFFF}, nil
	case 0xD1: // SET 2, C
		return &Instruction{Mnemonic: "SET 2, C", Length: 2, Address: 0xFFFF}, nil
	case 0xD2: // SET 2, D
		return &Instruction{Mnemonic: "SET 2, D", Length: 2, Address: 0xFFFF}, nil
	case 0xD3: // SET 2, E
		return &Instruction{Mnemonic: "SET 2, E", Length: 2, Address: 0xFFFF}, nil
	case 0xD4: // SET 2, H
		return &Instruction{Mnemonic: "SET 2, H", Length: 2, Address: 0xFFFF}, nil
	case 0xD5: // SET 2, L
		return &Instruction{Mnemonic: "SET 2, L", Length: 2, Address: 0xFFFF}, nil
	case 0xD6: // SET 2, (HL)
		return &Instruction{Mnemonic: "SET 2, (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0xD7: // SET 2, A
		return &Instruction{Mnemonic: "SET 2, A", Length: 2, Address: 0xFFFF}, nil
	case 0xD8: // SET 3, B
		return &Instruction{Mnemonic: "SET 3, B", Length: 2, Address: 0xFFFF}, nil
	case 0xD9: // SET 3, C
		return &Instruction{Mnemonic: "SET 3, C", Length: 2, Address: 0xFFFF}, nil
	case 0xDA: // SET 3, D
		return &Instruction{Mnemonic: "SET 3, D", Length: 2, Address: 0xFFFF}, nil
	case 0xDB: // SET 3, E
		return &Instruction{Mnemonic: "SET 3, E", Length: 2, Address: 0xFFFF}, nil
	case 0xDC: // SET 3, H
		return &Instruction{Mnemonic: "SET 3, H", Length: 2, Address: 0xFFFF}, nil
	case 0xDD: // SET 3, L
		return &Instruction{Mnemonic: "SET 3, L", Length: 2, Address: 0xFFFF}, nil
	case 0xDE: // SET 3, (HL)
		return &Instruction{Mnemonic: "SET 3, (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0xDF: // SET 3, A
		return &Instruction{Mnemonic: "SET 3, A", Length: 2, Address: 0xFFFF}, nil
	case 0xE0: // SET 4, B
		return &Instruction{Mnemonic: "SET 4, B", Length: 2, Address: 0xFFFF}, nil
	case 0xE1: // SET 4, C
		return &Instruction{Mnemonic: "SET 4, C", Length: 2, Address: 0xFFFF}, nil
	case 0xE2: // SET 4, D
		return &Instruction{Mnemonic: "SET 4, D", Length: 2, Address: 0xFFFF}, nil
	case 0xE3: // SET 4, E
		return &Instruction{Mnemonic: "SET 4, E", Length: 2, Address: 0xFFFF}, nil
	case 0xE4: // SET 4, H
		return &Instruction{Mnemonic: "SET 4, H", Length: 2, Address: 0xFFFF}, nil
	case 0xE5: // SET 4, L
		return &Instruction{Mnemonic: "SET 4, L", Length: 2, Address: 0xFFFF}, nil
	case 0xE6: // SET 4, (HL)
		return &Instruction{Mnemonic: "SET 4, (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0xE7: // SET 4, A
		return &Instruction{Mnemonic: "SET 4, A", Length: 2, Address: 0xFFFF}, nil
	case 0xE8: // SET 5, B
		return &Instruction{Mnemonic: "SET 5, B", Length: 2, Address: 0xFFFF}, nil
	case 0xE9: // SET 5, C
		return &Instruction{Mnemonic: "SET 5, C", Length: 2, Address: 0xFFFF}, nil
	case 0xEA: // SET 5, D
		return &Instruction{Mnemonic: "SET 5, D", Length: 2, Address: 0xFFFF}, nil
	case 0xEB: // SET 5, E
		return &Instruction{Mnemonic: "SET 5, E", Length: 2, Address: 0xFFFF}, nil
	case 0xEC: // SET 5, H
		return &Instruction{Mnemonic: "SET 5, H", Length: 2, Address: 0xFFFF}, nil
	case 0xED: // SET 5, L
		return &Instruction{Mnemonic: "SET 5, L", Length: 2, Address: 0xFFFF}, nil
	case 0xEE: // SET 5, (HL)
		return &Instruction{Mnemonic: "SET 5, (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0xEF: // SET 5, A
		return &Instruction{Mnemonic: "SET 5, A", Length: 2, Address: 0xFFFF}, nil
	case 0xF0: // SET 6, B
		return &Instruction{Mnemonic: "SET 6, B", Length: 2, Address: 0xFFFF}, nil
	case 0xF1: // SET 6, C
		return &Instruction{Mnemonic: "SET 6, C", Length: 2, Address: 0xFFFF}, nil
	case 0xF2: // SET 6, D
		return &Instruction{Mnemonic: "SET 6, D", Length: 2, Address: 0xFFFF}, nil
	case 0xF3: // SET 6, E
		return &Instruction{Mnemonic: "SET 6, E", Length: 2, Address: 0xFFFF}, nil
	case 0xF4: // SET 6, H
		return &Instruction{Mnemonic: "SET 6, H", Length: 2, Address: 0xFFFF}, nil
	case 0xF5: // SET 6, L
		return &Instruction{Mnemonic: "SET 6, L", Length: 2, Address: 0xFFFF}, nil
	case 0xF6: // SET 6, (HL)
		return &Instruction{Mnemonic: "SET 6, (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0xF7: // SET 6, A
		return &Instruction{Mnemonic: "SET 6, A", Length: 2, Address: 0xFFFF}, nil
	case 0xF8: // SET 7, B
		return &Instruction{Mnemonic: "SET 7, B", Length: 2, Address: 0xFFFF}, nil
	case 0xF9: // SET 7, C
		return &Instruction{Mnemonic: "SET 7, C", Length: 2, Address: 0xFFFF}, nil
	case 0xFA: // SET 7, D
		return &Instruction{Mnemonic: "SET 7, D", Length: 2, Address: 0xFFFF}, nil
	case 0xFB: // SET 7, E
		return &Instruction{Mnemonic: "SET 7, E", Length: 2, Address: 0xFFFF}, nil
	case 0xFC: // SET 7, H
		return &Instruction{Mnemonic: "SET 7, H", Length: 2, Address: 0xFFFF}, nil
	case 0xFD: // SET 7, L
		return &Instruction{Mnemonic: "SET 7, L", Length: 2, Address: 0xFFFF}, nil
	case 0xFE: // SET 7, (HL)
		return &Instruction{Mnemonic: "SET 7, (HL)", Length: 2, Address: 0xFFFF}, nil
	case 0xFF: // SET 7, A
		return &Instruction{Mnemonic: "SET 7, A", Length: 2, Address: 0xFFFF}, nil

	default:
		return &Instruction{Mnemonic: fmt.Sprintf("CB $%02X", opcode), Length: 2, Address: 0xFFFF}, nil
	}
}
