package cpu

import "fmt"

// executeOpcode executes a base (unprefixed) opcode. Returns T-states used.
func (z *Z80) executeOpcode(opcode uint8) int {
	switch opcode {
	// 8-bit load group
	case 0x00: // NOP
		return 4
	case 0x01: // LD BC, nn
		z.setBC(z.readImmediateWord())
		return 10
	case 0x02: // LD (BC), A
		z.writeByte(z.getBC(), z.A)
		z.MEMPTR = (uint16(z.A) << 8) | uint16((z.getBC()+1)&0xFF)
		return 7
	case 0x03: // INC BC
		z.setBC(z.getBC() + 1)
		return 6
	case 0x04: // INC B
		z.B = z.inc8(z.B)
		return 4
	case 0x05: // DEC B
		z.B = z.dec8(z.B)
		return 4
	case 0x06: // LD B, n
		z.B = z.readImmediateByte()
		return 7
	case 0x07: // RLCA
		z.rlca()
		return 4
	case 0x08: // EX AF, AF'
		temp := z.getAF()
		z.setAF(z.getAF_())
		z.setAF_(temp)
		return 4
	case 0x09: // ADD HL, BC
		oldHL := z.getHL()
		z.setHL(z.add16(oldHL, z.getBC()))
		z.MEMPTR = oldHL + 1
		return 11
	case 0x0A: // LD A, (BC)
		z.A = z.readByte(z.getBC())
		z.MEMPTR = z.getBC() + 1
		return 7
	case 0x0B: // DEC BC
		z.setBC(z.getBC() - 1)
		return 6
	case 0x0C: // INC C
		z.C = z.inc8(z.C)
		return 4
	case 0x0D: // DEC C
		z.C = z.dec8(z.C)
		return 4
	case 0x0E: // LD C, n
		z.C = z.readImmediateByte()
		return 7
	case 0x0F: // RRCA
		z.rrca()
		return 4
	case 0x10: // DJNZ e
		z.B--
		if z.B != 0 {
			offset := z.readDisplacement()
			// MEMPTR = target, like JR/JR cc, but only when the branch is taken
			z.MEMPTR = uint16(int32(z.PC) + int32(offset))
			z.PC = uint16(int32(z.PC) + int32(offset))
			return 13
		}
		z.PC++ // Skip the offset byte
		return 8
	case 0x11: // LD DE, nn
		z.setDE(z.readImmediateWord())
		return 10
	case 0x12: // LD (DE), A
		z.writeByte(z.getDE(), z.A)
		z.MEMPTR = (uint16(z.A) << 8) | uint16((z.getDE()+1)&0xFF)
		return 7
	case 0x13: // INC DE
		z.setDE(z.getDE() + 1)
		return 6
	case 0x14: // INC D
		z.D = z.inc8(z.D)
		return 4
	case 0x15: // DEC D
		z.D = z.dec8(z.D)
		return 4
	case 0x16: // LD D, n
		z.D = z.readImmediateByte()
		return 7
	case 0x17: // RLA
		z.rla()
		return 4
	case 0x18: // JR e
		offset := z.readDisplacement()
		z.MEMPTR = uint16(int32(z.PC) + int32(offset))
		z.PC = uint16(int32(z.PC) + int32(offset))
		return 12
	case 0x19: // ADD HL, DE
		oldHL := z.getHL()
		z.setHL(z.add16(oldHL, z.getDE()))
		z.MEMPTR = oldHL + 1
		return 11
	case 0x1A: // LD A, (DE)
		z.A = z.readByte(z.getDE())
		z.MEMPTR = z.getDE() + 1
		return 7
	case 0x1B: // DEC DE
		z.setDE(z.getDE() - 1)
		return 6
	case 0x1C: // INC E
		z.E = z.inc8(z.E)
		return 4
	case 0x1D: // DEC E
		z.E = z.dec8(z.E)
		return 4
	case 0x1E: // LD E, n
		z.E = z.readImmediateByte()
		return 7
	case 0x1F: // RRA
		z.rra()
		return 4

	// Conditional jumps
	case 0x20: // JR NZ, e
		if !z.getFlag(FLAG_Z) {
			offset := z.readDisplacement()
			z.MEMPTR = uint16(int32(z.PC) + int32(offset))
			z.PC = uint16(int32(z.PC) + int32(offset))
			return 12
		}
		z.PC++ // Skip the offset byte
		return 7
	case 0x21: // LD HL, nn
		z.setHL(z.readImmediateWord())
		return 10
	case 0x22: // LD (nn), HL
		addr := z.readImmediateWord()
		z.writeWord(addr, z.getHL())
		z.MEMPTR = addr + 1
		return 16
	case 0x23: // INC HL
		z.setHL(z.getHL() + 1)
		return 6
	case 0x24: // INC H
		z.H = z.inc8(z.H)
		return 4
	case 0x25: // DEC H
		z.H = z.dec8(z.H)
		return 4
	case 0x26: // LD H, n
		z.H = z.readImmediateByte()
		return 7
	case 0x27: // DAA
		z.daa()
		return 4
	case 0x28: // JR Z, e
		if z.getFlag(FLAG_Z) {
			offset := z.readDisplacement()
			z.MEMPTR = uint16(int32(z.PC) + int32(offset))
			z.PC = uint16(int32(z.PC) + int32(offset))
			return 12
		}
		z.PC++
		return 7
	case 0x29: // ADD HL, HL
		oldHL := z.getHL()
		z.setHL(z.add16(oldHL, oldHL))
		z.MEMPTR = oldHL + 1
		return 11
	case 0x2A: // LD HL, (nn)
		addr := z.readImmediateWord()
		z.setHL(z.readWord(addr))
		z.MEMPTR = addr + 1
		return 16
	case 0x2B: // DEC HL
		z.setHL(z.getHL() - 1)
		return 6
	case 0x2C: // INC L
		z.L = z.inc8(z.L)
		return 4
	case 0x2D: // DEC L
		z.L = z.dec8(z.L)
		return 4
	case 0x2E: // LD L, n
		z.L = z.readImmediateByte()
		return 7
	case 0x2F: // CPL
		z.cpl()
		return 4

	case 0x30: // JR NC, e
		if !z.getFlag(FLAG_C) {
			offset := z.readDisplacement()
			z.MEMPTR = uint16(int32(z.PC) + int32(offset))
			z.PC = uint16(int32(z.PC) + int32(offset))
			return 12
		}
		z.PC++
		return 7
	case 0x31: // LD SP, nn
		z.SP = z.readImmediateWord()
		return 10
	case 0x32: // LD (nn), A
		addr := z.readImmediateWord()
		z.writeByte(addr, z.A)
		z.MEMPTR = (uint16(z.A) << 8) | ((addr + 1) & 0xFF)
		return 13
	case 0x33: // INC SP
		z.SP++
		return 6
	case 0x34: // INC (HL)
		value := z.readByte(z.getHL())
		result := z.inc8(value)
		z.writeByte(z.getHL(), result)
		return 11
	case 0x35: // DEC (HL)
		value := z.readByte(z.getHL())
		result := z.dec8(value)
		z.writeByte(z.getHL(), result)
		return 11
	case 0x36: // LD (HL), n
		value := z.readImmediateByte()
		z.writeByte(z.getHL(), value)
		return 10
	case 0x37: // SCF
		z.scf()
		return 4
	case 0x38: // JR C, e
		if z.getFlag(FLAG_C) {
			offset := z.readDisplacement()
			z.MEMPTR = uint16(int32(z.PC) + int32(offset))
			z.PC = uint16(int32(z.PC) + int32(offset))
			return 12
		}
		z.PC++
		return 7
	case 0x39: // ADD HL, SP
		oldHL := z.getHL()
		z.setHL(z.add16(oldHL, z.SP))
		z.MEMPTR = oldHL + 1
		return 11
	case 0x3A: // LD A, (nn)
		addr := z.readImmediateWord()
		z.A = z.readByte(addr)
		z.MEMPTR = addr + 1
		return 13
	case 0x3B: // DEC SP
		z.SP--
		return 6
	case 0x3C: // INC A
		z.A = z.inc8(z.A)
		return 4
	case 0x3D: // DEC A
		z.A = z.dec8(z.A)
		return 4
	case 0x3E: // LD A, n
		z.A = z.readImmediateByte()
		return 7
	case 0x3F: // CCF
		z.ccf()
		return 4

	// LD r, r' instructions (0x40-0x7F)
	case 0x40: // LD B, B - NOP-like
		return 4
	case 0x41: // LD B, C
		z.B = z.C; return 4
	case 0x42: // LD B, D
		z.B = z.D; return 4
	case 0x43: // LD B, E
		z.B = z.E; return 4
	case 0x44: // LD B, H
		z.B = z.H; return 4
	case 0x45: // LD B, L
		z.B = z.L; return 4
	case 0x46: // LD B, (HL)
		z.B = z.readByte(z.getHL()); return 7
	case 0x47: // LD B, A
		z.B = z.A; return 4
	case 0x48: // LD C, B
		z.C = z.B; return 4
	case 0x49: // LD C, C - NOP-like
		return 4
	case 0x4A:
		z.C = z.D; return 4
	case 0x4B:
		z.C = z.E; return 4
	case 0x4C:
		z.C = z.H; return 4
	case 0x4D:
		z.C = z.L; return 4
	case 0x4E: // LD C, (HL)
		z.C = z.readByte(z.getHL()); return 7
	case 0x4F:
		z.C = z.A; return 4
	case 0x50:
		z.D = z.B; return 4
	case 0x51:
		z.D = z.C; return 4
	case 0x52: // LD D, D - NOP-like
		return 4
	case 0x53:
		z.D = z.E; return 4
	case 0x54:
		z.D = z.H; return 4
	case 0x55:
		z.D = z.L; return 4
	case 0x56: // LD D, (HL)
		z.D = z.readByte(z.getHL()); return 7
	case 0x57:
		z.D = z.A; return 4
	case 0x58:
		z.E = z.B; return 4
	case 0x59:
		z.E = z.C; return 4
	case 0x5A:
		z.E = z.D; return 4
	case 0x5B: // LD E, E - NOP-like
		return 4
	case 0x5C:
		z.E = z.H; return 4
	case 0x5D:
		z.E = z.L; return 4
	case 0x5E: // LD E, (HL)
		z.E = z.readByte(z.getHL()); return 7
	case 0x5F:
		z.E = z.A; return 4
	case 0x60:
		z.H = z.B; return 4
	case 0x61:
		z.H = z.C; return 4
	case 0x62:
		z.H = z.D; return 4
	case 0x63:
		z.H = z.E; return 4
	case 0x64: // LD H, H - NOP-like
		return 4
	case 0x65:
		z.H = z.L; return 4
	case 0x66: // LD H, (HL)
		z.H = z.readByte(z.getHL()); return 7
	case 0x67:
		z.H = z.A; return 4
	case 0x68:
		z.L = z.B; return 4
	case 0x69:
		z.L = z.C; return 4
	case 0x6A:
		z.L = z.D; return 4
	case 0x6B:
		z.L = z.E; return 4
	case 0x6C:
		z.L = z.H; return 4
	case 0x6D: // LD L, L - NOP-like
		return 4
	case 0x6E: // LD L, (HL)
		z.L = z.readByte(z.getHL()); return 7
	case 0x6F:
		z.L = z.A; return 4
	case 0x70: // LD (HL), B
		z.writeByte(z.getHL(), z.B); return 7
	case 0x71: // LD (HL), C
		z.writeByte(z.getHL(), z.C); return 7
	case 0x72: // LD (HL), D
		z.writeByte(z.getHL(), z.D); return 7
	case 0x73: // LD (HL), E
		z.writeByte(z.getHL(), z.E); return 7
	case 0x74: // LD (HL), H
		z.writeByte(z.getHL(), z.H); return 7
	case 0x75: // LD (HL), L
		z.writeByte(z.getHL(), z.L); return 7
	case 0x76: // HALT
		z.HALT = true
		z.PC--
		return 4
	case 0x77: // LD (HL), A
		z.writeByte(z.getHL(), z.A); return 7
	case 0x78:
		z.A = z.B; return 4
	case 0x79:
		z.A = z.C; return 4
	case 0x7A:
		z.A = z.D; return 4
	case 0x7B:
		z.A = z.E; return 4
	case 0x7C:
		z.A = z.H; return 4
	case 0x7D:
		z.A = z.L; return 4
	case 0x7E: // LD A, (HL)
		z.A = z.readByte(z.getHL()); return 7
	case 0x7F: // LD A, A - NOP-like
		return 4

	// ALU group (0x80-0xBF)
	case 0x80:
		z.add8(z.B); return 4
	case 0x81:
		z.add8(z.C); return 4
	case 0x82:
		z.add8(z.D); return 4
	case 0x83:
		z.add8(z.E); return 4
	case 0x84:
		z.add8(z.H); return 4
	case 0x85:
		z.add8(z.L); return 4
	case 0x86: // ADD A, (HL)
		z.add8(z.readByte(z.getHL())); return 7
	case 0x87:
		z.add8(z.A); return 4
	case 0x88:
		z.adc8(z.B); return 4
	case 0x89:
		z.adc8(z.C); return 4
	case 0x8A:
		z.adc8(z.D); return 4
	case 0x8B:
		z.adc8(z.E); return 4
	case 0x8C:
		z.adc8(z.H); return 4
	case 0x8D:
		z.adc8(z.L); return 4
	case 0x8E: // ADC A, (HL)
		z.adc8(z.readByte(z.getHL())); return 7
	case 0x8F:
		z.adc8(z.A); return 4
	case 0x90:
		z.sub8(z.B); return 4
	case 0x91:
		z.sub8(z.C); return 4
	case 0x92:
		z.sub8(z.D); return 4
	case 0x93:
		z.sub8(z.E); return 4
	case 0x94:
		z.sub8(z.H); return 4
	case 0x95:
		z.sub8(z.L); return 4
	case 0x96: // SUB (HL)
		z.sub8(z.readByte(z.getHL())); return 7
	case 0x97:
		z.sub8(z.A); return 4
	case 0x98:
		z.sbc8(z.B); return 4
	case 0x99:
		z.sbc8(z.C); return 4
	case 0x9A:
		z.sbc8(z.D); return 4
	case 0x9B:
		z.sbc8(z.E); return 4
	case 0x9C:
		z.sbc8(z.H); return 4
	case 0x9D:
		z.sbc8(z.L); return 4
	case 0x9E: // SBC A, (HL)
		z.sbc8(z.readByte(z.getHL())); return 7
	case 0x9F:
		z.sbc8(z.A); return 4
	case 0xA0:
		z.and8(z.B); return 4
	case 0xA1:
		z.and8(z.C); return 4
	case 0xA2:
		z.and8(z.D); return 4
	case 0xA3:
		z.and8(z.E); return 4
	case 0xA4:
		z.and8(z.H); return 4
	case 0xA5:
		z.and8(z.L); return 4
	case 0xA6: // AND (HL)
		z.and8(z.readByte(z.getHL())); return 7
	case 0xA7:
		z.and8(z.A); return 4
	case 0xA8:
		z.xor8(z.B); return 4
	case 0xA9:
		z.xor8(z.C); return 4
	case 0xAA:
		z.xor8(z.D); return 4
	case 0xAB:
		z.xor8(z.E); return 4
	case 0xAC:
		z.xor8(z.H); return 4
	case 0xAD:
		z.xor8(z.L); return 4
	case 0xAE: // XOR (HL)
		z.xor8(z.readByte(z.getHL())); return 7
	case 0xAF:
		z.xor8(z.A); return 4
	case 0xB0:
		z.or8(z.B); return 4
	case 0xB1:
		z.or8(z.C); return 4
	case 0xB2:
		z.or8(z.D); return 4
	case 0xB3:
		z.or8(z.E); return 4
	case 0xB4:
		z.or8(z.H); return 4
	case 0xB5:
		z.or8(z.L); return 4
	case 0xB6: // OR (HL)
		z.or8(z.readByte(z.getHL())); return 7
	case 0xB7:
		z.or8(z.A); return 4
	case 0xB8:
		z.cp8(z.B); return 4
	case 0xB9:
		z.cp8(z.C); return 4
	case 0xBA:
		z.cp8(z.D); return 4
	case 0xBB:
		z.cp8(z.E); return 4
	case 0xBC:
		z.cp8(z.H); return 4
	case 0xBD:
		z.cp8(z.L); return 4
	case 0xBE: // CP (HL)
		z.cp8(z.readByte(z.getHL())); return 7
	case 0xBF:
		z.cp8(z.A); return 4

	// RET cc
	case 0xC0: // RET NZ
		if !z.getFlag(FLAG_Z) {
			z.PC = z.pop(); z.MEMPTR = z.PC; return 11
		}
		return 5
	case 0xC1: // POP BC
		z.setBC(z.pop()); return 10
	case 0xC2: // JP NZ, nn
		addr := z.readImmediateWord(); z.MEMPTR = addr
		if !z.getFlag(FLAG_Z) {
			z.PC = addr; return 10
		}
		return 10
	case 0xC3: // JP nn
		addr := z.readImmediateWord()
		z.MEMPTR = addr; z.PC = addr
		return 10
	case 0xC4: // CALL NZ, nn
		addr := z.readImmediateWord(); z.MEMPTR = addr
		if !z.getFlag(FLAG_Z) {
			z.push(z.PC); z.PC = addr; return 17
		}
		return 10
	case 0xC5: // PUSH BC
		z.push(z.getBC()); return 11
	case 0xC6: // ADD A, n
		z.add8(z.readImmediateByte()); return 7
	case 0xC7: // RST 00H
		z.push(z.PC); z.PC = 0x0000; z.MEMPTR = 0x0000; return 11
	case 0xC8: // RET Z
		if z.getFlag(FLAG_Z) {
			z.PC = z.pop(); z.MEMPTR = z.PC; return 11
		}
		return 5
	case 0xC9: // RET
		z.PC = z.pop(); z.MEMPTR = z.PC; return 10
	case 0xCA: // JP Z, nn
		addr := z.readImmediateWord(); z.MEMPTR = addr
		if z.getFlag(FLAG_Z) {
			z.PC = addr; return 10
		}
		return 10
	// 0xCB: handled in ExecuteOneInstruction
	case 0xCB:
		return 0
	case 0xCC: // CALL Z, nn
		addr := z.readImmediateWord(); z.MEMPTR = addr
		if z.getFlag(FLAG_Z) {
			z.push(z.PC); z.PC = addr; return 17
		}
		return 10
	case 0xCD: // CALL nn
		addr := z.readImmediateWord()
		z.push(z.PC); z.PC = addr; z.MEMPTR = addr
		return 17
	case 0xCE: // ADC A, n
		z.adc8(z.readImmediateByte()); return 7
	case 0xCF: // RST 08H
		z.push(z.PC); z.PC = 0x0008; z.MEMPTR = 0x0008; return 11

	case 0xD0: // RET NC
		if !z.getFlag(FLAG_C) {
			z.PC = z.pop(); z.MEMPTR = z.PC; return 11
		}
		return 5
	case 0xD1: // POP DE
		z.setDE(z.pop()); return 10
	case 0xD2: // JP NC, nn
		addr := z.readImmediateWord(); z.MEMPTR = addr
		if !z.getFlag(FLAG_C) {
			z.PC = addr; return 10
		}
		return 10
	case 0xD3: // OUT (n), A
		n := z.readImmediateByte()
		port := uint16(n) | (uint16(z.A) << 8)
		z.writePort(port, z.A)
		z.MEMPTR = (uint16(z.A) << 8) | uint16((n+1)&0xFF)
		return 11
	case 0xD4: // CALL NC, nn
		addr := z.readImmediateWord(); z.MEMPTR = addr
		if !z.getFlag(FLAG_C) {
			z.push(z.PC); z.PC = addr; return 17
		}
		return 10
	case 0xD5: // PUSH DE
		z.push(z.getDE()); return 11
	case 0xD6: // SUB n
		z.sub8(z.readImmediateByte()); return 7
	case 0xD7: // RST 10H
		z.push(z.PC); z.PC = 0x0010; z.MEMPTR = 0x0010; return 11
	case 0xD8: // RET C
		if z.getFlag(FLAG_C) {
			z.PC = z.pop(); z.MEMPTR = z.PC; return 11
		}
		return 5
	case 0xD9: // EXX
		tempBC, tempDE, tempHL := z.getBC(), z.getDE(), z.getHL()
		z.setBC(z.getBC_()); z.setDE(z.getDE_()); z.setHL(z.getHL_())
		z.setBC_(tempBC); z.setDE_(tempDE); z.setHL_(tempHL)
		return 4
	case 0xDA: // JP C, nn
		addr := z.readImmediateWord(); z.MEMPTR = addr
		if z.getFlag(FLAG_C) {
			z.PC = addr; return 10
		}
		return 10
	case 0xDB: // IN A, (n)
		// MEMPTR is built from the A value *before* the read, then incremented
		// as a 16-bit add (it may carry into the high byte when n == 0xFF).
		n := z.readImmediateByte()
		port := uint16(n) | (uint16(z.A) << 8)
		z.A = z.readPort(port)
		z.MEMPTR = port + 1
		return 11
	case 0xDC: // CALL C, nn
		addr := z.readImmediateWord(); z.MEMPTR = addr
		if z.getFlag(FLAG_C) {
			z.push(z.PC); z.PC = addr; return 17
		}
		return 10
	// 0xDD: handled in ExecuteOneInstruction
	case 0xDD:
		return 0
	case 0xDE: // SBC A, n
		z.sbc8(z.readImmediateByte()); return 7
	case 0xDF: // RST 18H
		z.push(z.PC); z.PC = 0x0018; z.MEMPTR = 0x0018; return 11

	case 0xE0: // RET PO
		if !z.getFlag(FLAG_PV) {
			z.PC = z.pop(); z.MEMPTR = z.PC; return 11
		}
		return 5
	case 0xE1: // POP HL
		z.setHL(z.pop()); return 10
	case 0xE2: // JP PO, nn
		addr := z.readImmediateWord(); z.MEMPTR = addr
		if !z.getFlag(FLAG_PV) {
			z.PC = addr; return 10
		}
		return 10
	case 0xE3: // EX (SP), HL
		temp := z.readWord(z.SP)
		z.writeWord(z.SP, z.getHL())
		z.setHL(temp)
		z.MEMPTR = temp
		return 19
	case 0xE4: // CALL PO, nn
		addr := z.readImmediateWord(); z.MEMPTR = addr
		if !z.getFlag(FLAG_PV) {
			z.push(z.PC); z.PC = addr; return 17
		}
		return 10
	case 0xE5: // PUSH HL
		z.push(z.getHL()); return 11
	case 0xE6: // AND n
		z.and8(z.readImmediateByte()); return 7
	case 0xE7: // RST 20H
		z.push(z.PC); z.PC = 0x0020; z.MEMPTR = 0x0020; return 11
	case 0xE8: // RET PE
		if z.getFlag(FLAG_PV) {
			z.PC = z.pop(); z.MEMPTR = z.PC; return 11
		}
		return 5
	case 0xE9: // JP (HL)
		z.PC = z.getHL(); return 4
	case 0xEA: // JP PE, nn
		addr := z.readImmediateWord(); z.MEMPTR = addr
		if z.getFlag(FLAG_PV) {
			z.PC = addr; return 10
		}
		return 10
	case 0xEB: // EX DE, HL
		tempDE := z.getDE()
		z.setDE(z.getHL())
		z.setHL(tempDE)
		return 4
	case 0xEC: // CALL PE, nn
		addr := z.readImmediateWord(); z.MEMPTR = addr
		if z.getFlag(FLAG_PV) {
			z.push(z.PC); z.PC = addr; return 17
		}
		return 10
	// 0xED: handled in ExecuteOneInstruction
	case 0xED:
		return 0
	case 0xEE: // XOR n
		z.xor8(z.readImmediateByte()); return 7
	case 0xEF: // RST 28H
		z.push(z.PC); z.PC = 0x0028; z.MEMPTR = 0x0028; return 11

	case 0xF0: // RET P
		if !z.getFlag(FLAG_S) {
			z.PC = z.pop(); z.MEMPTR = z.PC; return 11
		}
		return 5
	case 0xF1: // POP AF
		z.setAF(z.pop()); return 10
	case 0xF2: // JP P, nn
		addr := z.readImmediateWord(); z.MEMPTR = addr
		if !z.getFlag(FLAG_S) {
			z.PC = addr; return 10
		}
		return 10
	case 0xF3: // DI
		z.IFF1 = false; z.IFF2 = false; return 4
	case 0xF4: // CALL P, nn
		addr := z.readImmediateWord(); z.MEMPTR = addr
		if !z.getFlag(FLAG_S) {
			z.push(z.PC); z.PC = addr; return 17
		}
		return 10
	case 0xF5: // PUSH AF
		z.push(z.getAF()); return 11
	case 0xF6: // OR n
		z.or8(z.readImmediateByte()); return 7
	case 0xF7: // RST 30H
		z.push(z.PC); z.PC = 0x0030; z.MEMPTR = 0x0030; return 11
	case 0xF8: // RET M
		if z.getFlag(FLAG_S) {
			z.PC = z.pop(); z.MEMPTR = z.PC; return 11
		}
		return 5
	case 0xF9: // LD SP, HL
		z.SP = z.getHL(); return 6
	case 0xFA: // JP M, nn
		addr := z.readImmediateWord(); z.MEMPTR = addr
		if z.getFlag(FLAG_S) {
			z.PC = addr; return 10
		}
		return 10
	case 0xFB: // EI -- set IFF immediately, but interrupt is blocked for 1 instruction
		z.IFF1 = true
		z.IFF2 = true
		z.eipending = true // block interrupt until after next instruction
		return 4
	case 0xFC: // CALL M, nn
		addr := z.readImmediateWord(); z.MEMPTR = addr
		if z.getFlag(FLAG_S) {
			z.push(z.PC); z.PC = addr; return 17
		}
		return 10
	// 0xFD: handled in ExecuteOneInstruction
	case 0xFD:
		return 0
	case 0xFE: // CP n
		z.cp8(z.readImmediateByte()); return 7
	case 0xFF: // RST 38H
		z.push(z.PC); z.PC = 0x0038; z.MEMPTR = 0x0038; return 11
	default:
		panic(fmt.Sprintf("unexpected opcode 0x%02X at PC=0x%04X", opcode, z.PC-1))
	}
}
