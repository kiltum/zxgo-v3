package cpu

// executeEDOpcode executes an ED-prefixed opcode. Returns T-states used.
func (z *Z80) executeEDOpcode(opcode uint8) int {
	switch opcode {
	// Block transfer instructions
	case 0xA0: // LDI
		z.ldi()
		return 16
	case 0xA1: // CPI
		z.cpi()
		return 16
	case 0xA2: // INI
		z.ini()
		return 16
	case 0xA3: // OUTI
		z.outi()
		return 16
	case 0xA8: // LDD
		z.ldd()
		return 16
	case 0xA9: // CPD
		z.cpd()
		return 16
	case 0xAA: // IND
		z.ind()
		return 16
	case 0xAB: // OUTD
		z.outd()
		return 16
	case 0xB0: // LDIR
		return z.ldir()
	case 0xB1: // CPIR
		return z.cpir()
	case 0xB2: // INIR
		return z.inir()
	case 0xB3: // OTIR
		return z.otir()
	case 0xB8: // LDDR
		return z.lddr()
	case 0xB9: // CPDR
		return z.cpdr()
	case 0xBA: // INDR
		return z.indr()
	case 0xBB: // OTDR
		return z.otdr()

	// IN r, (C) / OUT (C), r -- register index in bits 5-3
	case 0x40: // IN B, (C)
		return z.executeIN(0)
	case 0x41: // OUT (C), B
		return z.executeOUT(0)
	case 0x42: // SBC HL, BC
		z.setHL(z.sbc16(z.getHL(), z.getBC()))
		return 15
	case 0x43: // LD (nn), BC
		addr := z.readImmediateWord()
		z.writeWord(addr, z.getBC())
		z.MEMPTR = addr + 1
		return 20
	case 0x44, 0x4C, 0x54, 0x5C, 0x64, 0x6C, 0x74, 0x7C: // NEG
		z.neg()
		return 8
	case 0x45, 0x55, 0x5D, 0x65, 0x6D, 0x75, 0x7D: // RETN
		z.retn()
		return 14
	case 0x46, 0x4E, 0x66, 0x6E: // IM 0 -- the mirror set differs in bit 3
		z.IM = 0
		return 8
	case 0x47: // LD I, A
		z.I = z.A
		return 9
	case 0x48: // IN C, (C)
		return z.executeIN(1)
	case 0x49: // OUT (C), C
		return z.executeOUT(1)
	case 0x4A: // ADC HL, BC
		z.setHL(z.adc16(z.getHL(), z.getBC()))
		return 15
	case 0x4B: // LD BC, (nn)
		addr := z.readImmediateWord()
		z.setBC(z.readWord(addr))
		z.MEMPTR = addr + 1
		return 20
	case 0x4D: // RETI
		z.reti()
		return 14
	case 0x4F: // LD R, A
		// R is 7-bit. Bit 7 preserved (or not -- both behaviors exist).
		// zxgo fix for zen80 tests: store full A
		z.R = z.A
		return 9
	case 0x50: // IN D, (C)
		return z.executeIN(2)
	case 0x51: // OUT (C), D
		return z.executeOUT(2)
	case 0x52: // SBC HL, DE
		z.setHL(z.sbc16(z.getHL(), z.getDE()))
		return 15
	case 0x53: // LD (nn), DE
		addr := z.readImmediateWord()
		z.writeWord(addr, z.getDE())
		z.MEMPTR = addr + 1
		return 20
	case 0x56, 0x76: // IM 1
		z.IM = 1
		return 8
	case 0x57: // LD A, I
		z.ldAI()
		return 9
	case 0x58: // IN E, (C)
		return z.executeIN(3)
	case 0x59: // OUT (C), E
		return z.executeOUT(3)
	case 0x5A: // ADC HL, DE
		z.setHL(z.adc16(z.getHL(), z.getDE()))
		return 15
	case 0x5B: // LD DE, (nn)
		addr := z.readImmediateWord()
		z.setDE(z.readWord(addr))
		z.MEMPTR = addr + 1
		return 20
	case 0x5E, 0x7E: // IM 2
		z.IM = 2
		return 8
	case 0x5F: // LD A, R
		z.ldAR()
		return 9
	case 0x60: // IN H, (C)
		return z.executeIN(4)
	case 0x61: // OUT (C), H
		return z.executeOUT(4)
	case 0x62: // SBC HL, HL
		z.setHL(z.sbc16(z.getHL(), z.getHL()))
		return 15
	case 0x63: // LD (nn), HL
		addr := z.readImmediateWord()
		z.writeWord(addr, z.getHL())
		z.MEMPTR = addr + 1
		return 20
	case 0x67: // RRD
		z.rrd()
		return 18
	case 0x68: // IN L, (C)
		return z.executeIN(5)
	case 0x69: // OUT (C), L
		return z.executeOUT(5)
	case 0x6A: // ADC HL, HL
		z.setHL(z.adc16(z.getHL(), z.getHL()))
		return 15
	case 0x6B: // LD HL, (nn)
		addr := z.readImmediateWord()
		z.setHL(z.readWord(addr))
		z.MEMPTR = addr + 1
		return 20
	case 0x6F: // RLD
		z.rld()
		return 18
	case 0x70: // IN (C) -- undocumented (dummy read)
		bc := z.getBC()
		value := z.inC()
		z.updateSZXY(value)
		z.F &^= FLAG_H | FLAG_N
		z.setFlagCond(FLAG_PV, parityEven(value))
		z.MEMPTR = bc + 1
		return 12
	case 0x71: // OUT (C), 0 -- undocumented
		z.outC(0)
		z.MEMPTR = z.getBC() + 1
		return 12
	case 0x72: // SBC HL, SP
		z.setHL(z.sbc16(z.getHL(), z.SP))
		return 15
	case 0x73: // LD (nn), SP
		addr := z.readImmediateWord()
		z.writeWord(addr, z.SP)
		z.MEMPTR = addr + 1
		return 20
	case 0x78: // IN A, (C)
		return z.executeIN(7)
	case 0x79: // OUT (C), A
		return z.executeOUT(7)
	case 0x7A: // ADC HL, SP
		z.setHL(z.adc16(z.getHL(), z.SP))
		return 15
	case 0x7B: // LD SP, (nn)
		addr := z.readImmediateWord()
		z.SP = z.readWord(addr)
		z.MEMPTR = addr + 1
		return 20
	default:
		//panic(fmt.Sprintf("ED unexpected opcode 0xED%02X at PC=0x%04X", opcode, z.PC))
		return 8 // All undefined opcodes are treated as NOP
	}
}

// executeIN handles IN r, (C) instructions. reg: 0=B,1=C,2=D,3=E,4=H,5=L,7=A.
func (z *Z80) executeIN(reg uint8) int {
	bc := z.getBC()
	value := z.inC()

	z.updateSZXYPV(value)
	z.F &^= FLAG_H | FLAG_N
	z.MEMPTR = bc + 1

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
	return 12
}

// executeOUT handles OUT (C), r instructions.
func (z *Z80) executeOUT(reg uint8) int {
	var value uint8
	switch reg {
	case 0:
		value = z.B
	case 1:
		value = z.C
	case 2:
		value = z.D
	case 3:
		value = z.E
	case 4:
		value = z.H
	case 5:
		value = z.L
	case 7:
		value = z.A
	}
	z.outC(value)
	z.MEMPTR = z.getBC() + 1
	return 12
}

// inC reads from port (BC).
func (z *Z80) inC() uint8 {
	return z.readPort(z.getBC())
}

// outC writes to port (BC).
func (z *Z80) outC(value uint8) {
	z.writePort(z.getBC(), value)
}

// ---------- Block instructions ----------

func (z *Z80) ldi() {
	value := z.readByte(z.getHL())
	z.writeByte(z.getDE(), value)
	z.setDE(z.getDE() + 1)
	z.setHL(z.getHL() + 1)
	z.setBC(z.getBC() - 1)

	n := value + z.A
	z.F = (z.F & (FLAG_S | FLAG_Z | FLAG_C)) | (n & FLAG_X) | ((n << 4) & FLAG_Y)
	z.F &^= FLAG_H
	z.setFlagCond(FLAG_PV, z.getBC() != 0)
	z.F &^= FLAG_N
}

func (z *Z80) cpi() {
	value := z.readByte(z.getHL())
	result := z.A - value
	z.setHL(z.getHL() + 1)
	z.setBC(z.getBC() - 1)
	z.setFlag(FLAG_N, true)
	z.updateSZ(result)

	z.setFlagCond(FLAG_H, (z.A&0x0F) < (value&0x0F))
	temp := result - boolToByte(z.getFlag(FLAG_H))
	z.setFlagCond(FLAG_X, (temp&0x08) != 0)
	z.setFlagCond(FLAG_Y, (temp&0x02) != 0)
	z.setFlagCond(FLAG_PV, z.getBC() != 0)
	z.MEMPTR++ // cpi/cpd step MEMPTR; they do not derive it from PC
}

// ini/ind build MEMPTR from BC *before* B is decremented; outi/outd use the
// post-decrement BC (and write to that same port). Fuse, UnrealSpeccy, unrealac
// and ZEsarUX agree on this split; ZXMAK2 uses the pre-decrement BC for all
// four, which is what makes it disagree with cpd-test on OTIR/OTDR.
func (z *Z80) ini() {
	value := z.readPort(z.getBC())
	z.writeByte(z.getHL(), value)
	z.setHL(z.getHL() + 1)
	origBC := z.getBC()
	z.B--

	k := int(value) + int((z.C+1)&0xFF)
	z.setFlagCond(FLAG_Z, z.B == 0)
	z.setFlagCond(FLAG_S, (z.B&0x80) != 0)
	z.setFlagCond(FLAG_N, (value&0x80) != 0)
	z.setFlagCond(FLAG_H, k > 0xFF)
	z.setFlagCond(FLAG_C, k > 0xFF)
	z.setFlagCond(FLAG_PV, parityEven(uint8(k&0x07)^z.B))
	z.F = (z.F & 0xD7) | (z.B & (FLAG_X | FLAG_Y))
	z.MEMPTR = origBC + 1
}

func (z *Z80) outi() {
	val := z.readByte(z.getHL())
	z.B--
	z.writePort(z.getBC(), val)
	z.setHL(z.getHL() + 1)

	k := int(val) + int(z.L)
	z.setFlagCond(FLAG_Z, z.B == 0)
	z.setFlagCond(FLAG_S, (z.B&0x80) != 0)
	z.setFlagCond(FLAG_N, (val&0x80) != 0)
	z.setFlagCond(FLAG_H, k > 0xFF)
	z.setFlagCond(FLAG_C, k > 0xFF)
	z.setFlagCond(FLAG_PV, parityEven(uint8(k&0x07)^z.B))
	z.F = (z.F & 0xD7) | (z.B & (FLAG_X | FLAG_Y))
	z.MEMPTR = z.getBC() + 1
}

func (z *Z80) ldd() {
	value := z.readByte(z.getHL())
	z.writeByte(z.getDE(), value)
	z.setHL(z.getHL() - 1)
	z.setDE(z.getDE() - 1)
	z.setBC(z.getBC() - 1)

	n := value + z.A
	z.F = (z.F & (FLAG_S | FLAG_Z | FLAG_C)) | (n & FLAG_X) | ((n << 4) & FLAG_Y)
	z.F &^= FLAG_H | FLAG_N
	z.setFlagCond(FLAG_PV, z.getBC() != 0)
}

func (z *Z80) cpd() {
	val := z.readByte(z.getHL())
	result := int16(z.A) - int16(val)
	z.setHL(z.getHL() - 1)
	z.setBC(z.getBC() - 1)

	z.setFlagCond(FLAG_S, (uint8(result)&0x80) != 0)
	z.setFlagCond(FLAG_Z, uint8(result) == 0)
	z.setFlagCond(FLAG_H, (int8(z.A&0x0F)-int8(val&0x0F)) < 0)
	z.setFlagCond(FLAG_PV, z.getBC() != 0)
	z.setFlag(FLAG_N, true)

	n := uint8(result)
	if z.getFlag(FLAG_H) {
		n--
	}
	z.F = (z.F & (FLAG_S | FLAG_Z | FLAG_H | FLAG_PV | FLAG_N | FLAG_C)) | (n & FLAG_X) | ((n << 4) & FLAG_Y)
	z.MEMPTR--
}

func (z *Z80) ind() {
	val := z.readPort(z.getBC())
	z.writeByte(z.getHL(), val)
	z.setHL(z.getHL() - 1)
	z.MEMPTR = z.getBC() - 1
	z.B--

	z.setFlagCond(FLAG_Z, z.B == 0)
	z.setFlagCond(FLAG_S, (z.B&0x80) != 0)
	z.setFlagCond(FLAG_N, (val&0x80) != 0)

	diff := uint16(z.C-1) + uint16(val)
	z.setFlagCond(FLAG_H, diff > 0xFF)
	z.setFlagCond(FLAG_C, diff > 0xFF)
	temp := uint8((diff & 0x07) ^ uint16(z.B))
	z.setFlagCond(FLAG_PV, parityEven(temp))
	z.F = (z.F & 0xD7) | (z.B & (FLAG_X | FLAG_Y))
}

func (z *Z80) outd() {
	val := z.readByte(z.getHL())
	z.B--
	z.writePort(uint16(z.C)|(uint16(z.B)<<8), val)
	z.setHL(z.getHL() - 1)

	k := uint16(val) + uint16(z.L)
	z.setFlagCond(FLAG_Z, z.B == 0)
	z.setFlagCond(FLAG_S, (z.B&0x80) != 0)
	z.setFlagCond(FLAG_N, (val&0x80) != 0)
	z.setFlagCond(FLAG_H, k > 0xFF)
	z.setFlagCond(FLAG_C, k > 0xFF)
	z.setFlagCond(FLAG_PV, parityEven(uint8(k&0x07)^z.B))
	z.F = (z.F & 0xD7) | (z.B & (FLAG_X | FLAG_Y))
	z.MEMPTR = z.getBC() - 1
}

// ---------- Repeated block instructions ----------
//
// MEMPTR on these has two parts: the per-iteration value set by the single-step
// helper above (BC derived), and an override that happens only while the
// instruction repeats. On the repeat path PC has just been wound back to the
// instruction, so "PC + 1" is the address of the opcode byte - the same value
// LDIR/CPDR use. See KNOWN_BUGS.md ("Z80 MEMPTR / WZ") for the evidence and for
// why the four I/O repeats differ from every other emulator here.

func (z *Z80) ldir() int {
	z.ldi()
	if z.getBC() != 0 {
		z.PC -= 2
		z.MEMPTR = z.PC + 1
		return 21
	}
	return 16
}

func (z *Z80) cpir() int {
	z.cpi()
	if z.getBC() != 0 && !z.getFlag(FLAG_Z) {
		z.PC -= 2
		z.MEMPTR = z.PC + 1
		return 21
	}
	return 16
}

// inir/indr/otir/otdr set MEMPTR = PC+1 whenever they repeat. This is what
// cpd-test.tzx asserts (its INIR/INDR "split" tests are deterministic, so the
// expected value can only come from this rule), but Fuse, ZXMAK2, UnrealSpeccy
// and ZEsarUX all leave the BC-derived value instead and never touch MEMPTR on
// the repeat. Removing these four assignments costs exactly four cpd tests and
// restores Fuse's edb2_1/edb3_1/edba_1/edbb_1.

func (z *Z80) inir() int {
	z.ini()
	if z.B != 0 {
		z.PC -= 2
		if z.memptrReal {
			z.MEMPTR = z.PC + 1
		}
		return 21
	}
	return 16
}

func (z *Z80) otir() int {
	z.outi()
	if z.B != 0 {
		z.PC -= 2
		if z.memptrReal {
			z.MEMPTR = z.PC + 1
		}
		return 21
	}
	return 16
}

func (z *Z80) lddr() int {
	z.ldd()
	if z.getBC() != 0 {
		z.PC -= 2
		z.MEMPTR = z.PC + 1
		return 21
	}
	return 16
}

func (z *Z80) cpdr() int {
	z.cpd()
	if z.getBC() != 0 && !z.getFlag(FLAG_Z) {
		z.PC -= 2
		z.MEMPTR = z.PC + 1
		return 21
	}
	return 16
}

func (z *Z80) indr() int {
	z.ind()
	if z.B != 0 {
		z.PC -= 2
		if z.memptrReal { // see inir above
			z.MEMPTR = z.PC + 1
		}
		return 21
	}
	return 16
}

func (z *Z80) otdr() int {
	z.outd()
	if z.B != 0 {
		z.PC -= 2
		if z.memptrReal { // see inir above
			z.MEMPTR = z.PC + 1
		}
		return 21
	}
	return 16
}

// ---------- ED-specific helpers ----------

func (z *Z80) retn() {
	z.PC = z.pop()
	z.MEMPTR = z.PC
	z.IFF1 = z.IFF2
}

func (z *Z80) reti() {
	z.PC = z.pop()
	z.MEMPTR = z.PC
	z.IFF1 = z.IFF2
}

func (z *Z80) ldAI() {
	z.A = z.I
	z.updateSZXY(z.A)
	z.F &^= FLAG_H | FLAG_N
	z.setFlagCond(FLAG_PV, z.IFF2)
}

func (z *Z80) ldAR() {
	z.A = z.R
	z.updateSZXY(z.A)
	z.F &^= FLAG_H | FLAG_N
	z.setFlagCond(FLAG_PV, z.IFF2)
}

func (z *Z80) rrd() {
	value := z.readByte(z.getHL())
	ah := z.A & 0xF0
	al := z.A & 0x0F
	z.A = ah | (value & 0x0F)
	newHL := ((value & 0xF0) >> 4) | (al << 4)
	z.writeByte(z.getHL(), newHL)
	z.updateSZXYPV(z.A)
	z.F &^= FLAG_H | FLAG_N
	z.MEMPTR = z.getHL() + 1
}

func (z *Z80) rld() {
	value := z.readByte(z.getHL())
	ah := z.A & 0xF0
	al := z.A & 0x0F
	z.A = ah | (value >> 4)
	newHL := ((value & 0x0F) << 4) | al
	z.writeByte(z.getHL(), newHL)
	z.updateSZXYPV(z.A)
	z.F &^= FLAG_H | FLAG_N
	z.MEMPTR = z.getHL() + 1
}
