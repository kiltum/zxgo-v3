package cpu

// executeDDOpcode executes a DD-prefixed opcode. Returns T-states used.
func (z *Z80) executeDDOpcode(opcode uint8) int {
	if opcode == 0xCB {
		return z.executeDDCB()
	}
	if opcode == 0xED {
		// ED has no indexed group: the DD prefix is ignored and the ED
		// instruction executes. The extra 4 T-states are the prefix's M1.
		return 4 + z.executeEDOpcode(z.readOpcode())
	}
	if opcode == 0xDD || opcode == 0xFD {
		return 8
	}
	return z.executeIndexedOpcode(ixRef{z: z}, opcode)
}

// executeFDOpcode executes an FD-prefixed opcode. Returns T-states used.
func (z *Z80) executeFDOpcode(opcode uint8) int {
	if opcode == 0xCB {
		return z.executeFDCB()
	}
	if opcode == 0xED {
		// As for DD above: the index prefix is ignored before ED.
		return 4 + z.executeEDOpcode(z.readOpcode())
	}
	if opcode == 0xDD || opcode == 0xFD {
		return 8
	}
	return z.executeIndexedOpcode(iyRef{z: z}, opcode)
}

// executeIndexedOpcode is the shared DD/FD handler via indexReg.
// Opcodes that reference H,L,(HL) are redirected to IXH,IXL,(IX+d).
// Opcodes that don't reference H/L/(HL) fall through to the unprefixed handler.
func (z *Z80) executeIndexedOpcode(idx indexReg, opcode uint8) int {
	switch opcode {
	// --- 16-bit group ---
	case 0x09: // ADD idx, BC
		old := idx.Full()
		result := z.add16(old, z.getBC())
		idx.SetFull(result)
		z.MEMPTR = old + 1
		z.updateXYFromAddr(result)
		return 15
	case 0x19: // ADD idx, DE
		old := idx.Full()
		result := z.add16(old, z.getDE())
		idx.SetFull(result)
		z.MEMPTR = old + 1
		z.updateXYFromAddr(result)
		return 15
	case 0x21: // LD idx, nn
		idx.SetFull(z.readImmediateWord())
		return 14
	case 0x22: // LD (nn), idx
		addr := z.readImmediateWord()
		z.writeWord(addr, idx.Full())
		z.MEMPTR = addr + 1
		return 20
	case 0x23: // INC idx
		idx.SetFull(idx.Full() + 1)
		return 10
	case 0x29: // ADD idx, idx
		old := idx.Full()
		idx.SetFull(z.add16(old, old))
		z.MEMPTR = old + 1
		return 15
	case 0x2A: // LD idx, (nn)
		addr := z.readImmediateWord()
		idx.SetFull(z.readWord(addr))
		z.MEMPTR = addr + 1
		return 20
	case 0x2B: // DEC idx
		idx.SetFull(idx.Full() - 1)
		return 10
	case 0x39: // ADD idx, SP
		old := idx.Full()
		idx.SetFull(z.add16(old, z.SP))
		z.MEMPTR = old + 1
		return 15

	// --- 8-bit INC/DEC/LD (idxH, idxL, (idx+d)) ---
	case 0x24: // INC idxH
		idx.SetHigh(z.inc8(idx.High()))
		return 8
	case 0x25: // DEC idxH
		idx.SetHigh(z.dec8(idx.High()))
		return 8
	case 0x26: // LD idxH, n
		idx.SetHigh(z.readImmediateByte())
		return 11
	case 0x2C: // INC idxL
		idx.SetLow(z.inc8(idx.Low()))
		return 8
	case 0x2D: // DEC idxL
		idx.SetLow(z.dec8(idx.Low()))
		return 8
	case 0x2E: // LD idxL, n
		idx.SetLow(z.readImmediateByte())
		return 11

	// --- INC/DEC/LD (idx+d) ---
	case 0x34: // INC (idx+d)
		d := z.readDisplacement()
		addr := uint16(int32(idx.Full()) + int32(d))
		v := z.readByte(addr)
		z.writeByte(addr, z.inc8(v))
		z.MEMPTR = addr
		return 23
	case 0x35: // DEC (idx+d)
		d := z.readDisplacement()
		addr := uint16(int32(idx.Full()) + int32(d))
		v := z.readByte(addr)
		z.writeByte(addr, z.dec8(v))
		z.MEMPTR = addr
		return 23
	case 0x36: // LD (idx+d), n
		d := z.readDisplacement()
		addr := uint16(int32(idx.Full()) + int32(d))
		z.writeByte(addr, z.readImmediateByte())
		z.MEMPTR = addr
		return 19

	// --- LD r, idxH / idxL / (idx+d) ---
	case 0x44: // LD B, idxH
		z.B = idx.High(); return 8
	case 0x45: // LD B, idxL
		z.B = idx.Low(); return 8
	case 0x46: // LD B, (idx+d)
		return z.loadFromIndexed(idx, 0)
	case 0x4C: // LD C, idxH
		z.C = idx.High(); return 8
	case 0x4D: // LD C, idxL
		z.C = idx.Low(); return 8
	case 0x4E: // LD C, (idx+d)
		return z.loadFromIndexed(idx, 1)
	case 0x54: // LD D, idxH
		z.D = idx.High(); return 8
	case 0x55: // LD D, idxL
		z.D = idx.Low(); return 8
	case 0x56: // LD D, (idx+d)
		return z.loadFromIndexed(idx, 2)
	case 0x5C: // LD E, idxH
		z.E = idx.High(); return 8
	case 0x5D: // LD E, idxL
		z.E = idx.Low(); return 8
	case 0x5E: // LD E, (idx+d)
		return z.loadFromIndexed(idx, 3)
	case 0x66: // LD H, (idx+d)
		return z.loadFromIndexed(idx, 4)
	case 0x6E: // LD L, (idx+d)
		return z.loadFromIndexed(idx, 5)
	case 0x7C: // LD A, idxH
		z.A = idx.High(); return 8
	case 0x7D: // LD A, idxL
		z.A = idx.Low(); return 8
	case 0x7E: // LD A, (idx+d)
		return z.loadFromIndexed(idx, 7)

	// --- LD idxH, r ---
	case 0x60: // LD idxH, B
		idx.SetHigh(z.B); return 8
	case 0x61: // LD idxH, C
		idx.SetHigh(z.C); return 8
	case 0x62: // LD idxH, D
		idx.SetHigh(z.D); return 8
	case 0x63: // LD idxH, E
		idx.SetHigh(z.E); return 8
	case 0x64: // LD idxH, idxH (NOP)
		return 8
	case 0x65: // LD idxH, idxL
		idx.SetHigh(idx.Low()); return 8
	case 0x67: // LD idxH, A
		idx.SetHigh(z.A); return 8

	// --- LD idxL, r ---
	case 0x68: // LD idxL, B
		idx.SetLow(z.B); return 8
	case 0x69: // LD idxL, C
		idx.SetLow(z.C); return 8
	case 0x6A: // LD idxL, D
		idx.SetLow(z.D); return 8
	case 0x6B: // LD idxL, E
		idx.SetLow(z.E); return 8
	case 0x6C: // LD idxL, idxH
		idx.SetLow(idx.High()); return 8
	case 0x6D: // LD idxL, idxL (NOP)
		return 8
	case 0x6F: // LD idxL, A
		idx.SetLow(z.A); return 8

	// --- LD (idx+d), r ---
	case 0x70: // LD (idx+d), B
		return z.storeToIndexed(idx, z.B)
	case 0x71: // LD (idx+d), C
		return z.storeToIndexed(idx, z.C)
	case 0x72: // LD (idx+d), D
		return z.storeToIndexed(idx, z.D)
	case 0x73: // LD (idx+d), E
		return z.storeToIndexed(idx, z.E)
	case 0x74: // LD (idx+d), H
		return z.storeToIndexed(idx, z.H)
	case 0x75: // LD (idx+d), L
		return z.storeToIndexed(idx, z.L)
	case 0x77: // LD (idx+d), A
		return z.storeToIndexed(idx, z.A)

	// --- ALU with idxH / idxL / (idx+d) ---
	case 0x84: // ADD A, idxH
		z.add8(idx.High()); return 8
	case 0x85: // ADD A, idxL
		z.add8(idx.Low()); return 8
	case 0x86: // ADD A, (idx+d)
		return z.aluIndexed(idx, 0)
	case 0x8C: // ADC A, idxH
		z.adc8(idx.High()); return 8
	case 0x8D: // ADC A, idxL
		z.adc8(idx.Low()); return 8
	case 0x8E: // ADC A, (idx+d)
		return z.aluIndexed(idx, 1)
	case 0x94: // SUB idxH
		z.sub8(idx.High()); return 8
	case 0x95: // SUB idxL
		z.sub8(idx.Low()); return 8
	case 0x96: // SUB (idx+d)
		return z.aluIndexed(idx, 2)
	case 0x9C: // SBC A, idxH
		z.sbc8(idx.High()); return 8
	case 0x9D: // SBC A, idxL
		z.sbc8(idx.Low()); return 8
	case 0x9E: // SBC A, (idx+d)
		return z.aluIndexed(idx, 3)
	case 0xA4: // AND idxH
		z.and8(idx.High()); return 8
	case 0xA5: // AND idxL
		z.and8(idx.Low()); return 8
	case 0xA6: // AND (idx+d)
		return z.aluIndexed(idx, 4)
	case 0xAC: // XOR idxH
		z.xor8(idx.High()); return 8
	case 0xAD: // XOR idxL
		z.xor8(idx.Low()); return 8
	case 0xAE: // XOR (idx+d)
		return z.aluIndexed(idx, 5)
	case 0xB4: // OR idxH
		z.or8(idx.High()); return 8
	case 0xB5: // OR idxL
		z.or8(idx.Low()); return 8
	case 0xB6: // OR (idx+d)
		return z.aluIndexed(idx, 6)
	case 0xBC: // CP idxH
		z.cp8(idx.High()); return 8
	case 0xBD: // CP idxL
		z.cp8(idx.Low()); return 8
	case 0xBE: // CP (idx+d)
		return z.aluIndexed(idx, 7)

	// --- POP/PUSH/EX/JP with idx ---
	case 0xE1: // POP idx
		idx.SetFull(z.pop())
		return 14
	case 0xE3: // EX (SP), idx
		temp := z.readWord(z.SP)
		z.writeWord(z.SP, idx.Full())
		idx.SetFull(temp)
		z.MEMPTR = temp
		return 23
	case 0xE5: // PUSH idx
		z.push(idx.Full())
		return 15
	case 0xE9: // JP (idx)
		z.PC = idx.Full()
		return 8
	case 0xF9: // LD SP, idx
		z.SP = idx.Full()
		return 10

	// NOP-like: DD/FD 00, ED, DD, FD are extended NOPs
	case 0x00:
		return 8

	default:
		// For opcodes that reference HL but we already handled H/L/(HL)
		// redirects above: 0x40-0x43, 0x47-0x4B, 0x4F, 0x50-0x53,
		// 0x57-0x5B, 0x5F, 0x78-0x7B, 0x7F.
		// These don't reference H/L/(HL) so the unprefixed handler is fine.
		// Also non-LD opcodes like conditional jumps, ALU, etc.
		return z.executeOpcode(opcode)
	}
}

// --- indexed helpers ---

func (z *Z80) loadFromIndexed(idx indexReg, reg uint8) int {
	d := z.readDisplacement()
	addr := uint16(int32(idx.Full()) + int32(d))
	value := z.readByte(addr)
	switch reg {
	case 0:
		z.B = value
	case 1:
		z.C = value
	case 2:
		z.D = value
	case 3:
		z.E = value
	case 4:
		z.H = value
	case 5:
		z.L = value
	case 7:
		z.A = value
	}
	z.MEMPTR = addr
	return 19
}

func (z *Z80) storeToIndexed(idx indexReg, value uint8) int {
	d := z.readDisplacement()
	addr := uint16(int32(idx.Full()) + int32(d))
	z.writeByte(addr, value)
	z.MEMPTR = addr
	return 19
}

func (z *Z80) aluIndexed(idx indexReg, opType uint8) int {
	d := z.readDisplacement()
	addr := uint16(int32(idx.Full()) + int32(d))
	value := z.readByte(addr)
	switch opType {
	case 0:
		z.add8(value)
	case 1:
		z.adc8(value)
	case 2:
		z.sub8(value)
	case 3:
		z.sbc8(value)
	case 4:
		z.and8(value)
	case 5:
		z.xor8(value)
	case 6:
		z.or8(value)
	case 7:
		z.cp8(value)
	}
	z.MEMPTR = addr
	return 19
}

// --- DDCB / FDCB ---

func (z *Z80) executeDDCB() int {
	originalR := z.R
	displacement := z.readDisplacement()
	opcode := z.readOpcode()
	z.R = originalR // undo R increments from prefix bytes
	addr := uint16(int32(z.IX) + int32(displacement))
	return z.executeIndexedCBOpcode(addr, ixRef{z: z}, opcode)
}

func (z *Z80) executeFDCB() int {
	displacement := z.readDisplacement()
	opcode := z.readOpcode()
	z.R-- // FDCB: undo the extra R increment from displacement read
	addr := uint16(int32(z.IY) + int32(displacement))
	return z.executeIndexedCBOpcode(addr, iyRef{z: z}, opcode)
}

func (z *Z80) executeIndexedCBOpcode(addr uint16, idx indexReg, opcode uint8) int {
	value := z.readByte(addr)
	group := (opcode >> 6) & 0x03
	x := (opcode >> 3) & 0x07
	reg := opcode & 0x07

	var result uint8
	var carry bool

	switch group {
	case 0: // rotate/shift
		switch x {
		case 0: // RLC
			carry = (value & 0x80) != 0
			result = (value << 1) | boolToByte(carry)
		case 1: // RRC
			carry = (value & 0x01) != 0
			result = (value >> 1) | (boolToByte(carry) << 7)
		case 2: // RL
			oldCarry := z.getFlag(FLAG_C)
			carry = (value & 0x80) != 0
			result = (value << 1) | boolToByte(oldCarry)
		case 3: // RR
			oldCarry := z.getFlag(FLAG_C)
			carry = (value & 0x01) != 0
			result = (value >> 1) | (boolToByte(oldCarry) << 7)
		case 4: // SLA
			carry = (value & 0x80) != 0
			result = value << 1
		case 5: // SRA
			carry = (value & 0x01) != 0
			result = (value >> 1) | (value & 0x80)
		case 6: // SLL
			carry = (value & 0x80) != 0
			result = (value << 1) | 0x01
		case 7: // SRL
			carry = (value & 0x01) != 0
			result = value >> 1
		}
		z.writeByte(addr, result)
		if reg != 6 { // reg 6 is (HL) -- already written to memory
			z.writeReg8(reg, result)
		}
		z.updateSZXYPV(result)
		z.F &^= FLAG_H | FLAG_N
		z.setFlagCond(FLAG_C, carry)
		z.MEMPTR = addr
		return 23

	case 1: // BIT b, (idx+d)
		bitSet := (value & (1 << x)) != 0
		z.setFlagCond(FLAG_Z, !bitSet)
		z.setFlagCond(FLAG_PV, !bitSet)
		z.setFlagCond(FLAG_H, true)
		z.F &^= FLAG_N
		if x == 7 {
			z.setFlagCond(FLAG_S, (value&0x80) != 0)
		} else {
			z.F &^= FLAG_S
		}
		z.updateXYFromAddr(addr)
		z.MEMPTR = addr
		return 20

	case 2: // RES b, (idx+d)
		value &^= (1 << x)
		z.writeByte(addr, value)
		if reg != 6 {
			z.writeReg8(reg, value)
		}
		z.MEMPTR = addr
		return 23

	case 3: // SET b, (idx+d)
		value |= (1 << x)
		z.writeByte(addr, value)
		if reg != 6 {
			z.writeReg8(reg, value)
		}
		z.MEMPTR = addr
		return 23
	}

	return 23
}
