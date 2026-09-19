package cpu

import "fmt"

// executeCBOpcode executes a CB-prefixed opcode. Returns T-states used.
// CB opcodes have three groups selected by bits 7-6:
//
//	00 = rotate/shift: bits 5-3 = operation, bits 2-0 = register
//	01 = BIT:  bits 5-3 = bit to test, bits 2-0 = register
//	10 = RES:  bits 5-3 = bit to clear, bits 2-0 = register
//	11 = SET:  bits 5-3 = bit to set,   bits 2-0 = register
func (z *Z80) executeCBOpcode(opcode uint8) int {
	group := (opcode >> 6) & 0x03
	x := (opcode >> 3) & 0x07 // operation, or bit number
	reg := opcode & 0x07

	switch group {
	case 0: // rotate/shift
		return z.cbRotShift(x, reg)
	case 1: // BIT
		return z.cbBit(x, reg)
	case 2: // RES
		return z.cbRes(x, reg)
	case 3: // SET
		return z.cbSet(x, reg)
	default:
		panic(fmt.Sprintf("unexpected CB opcode 0xCB%02X", opcode))
	}
}

// cbRotShift handles rotate/shift operations (CB group 0).
func (z *Z80) cbRotShift(op uint8, reg uint8) int {
	value := z.readReg8(reg)
	var result uint8
	var carry bool

	switch op {
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
	case 6: // SLL (undocumented -- shift left, set bit 0)
		carry = (value & 0x80) != 0
		result = (value << 1) | 0x01
	case 7: // SRL
		carry = (value & 0x01) != 0
		result = value >> 1
	}

	z.writeReg8(reg, result)
	z.updateSZXYPV(result)
	z.F &^= FLAG_H | FLAG_N
	z.setFlagCond(FLAG_C, carry)

	if reg == 6 {
		return 15
	}
	return 8
}

// cbBit handles BIT b, r (CB group 1).
func (z *Z80) cbBit(bit uint8, reg uint8) int {
	value := z.readReg8(reg)
	bitSet := (value & (1 << bit)) != 0
	z.setFlagCond(FLAG_Z, !bitSet)
	z.setFlagCond(FLAG_PV, !bitSet) // PV mirrors Z for BIT
	z.setFlagCond(FLAG_H, true)
	z.F &^= FLAG_N
	if bit == 7 {
		z.setFlagCond(FLAG_S, (value&0x80) != 0)
	} else {
		z.F &^= FLAG_S
	}
	// X and Y: for reg != (HL), from the tested register's value.
	// For (HL), from MEMPTR high byte.
	if reg == 6 {
		z.updateXYFromAddr(z.MEMPTR)
	} else {
		z.updateXYFromValue(value)
	}
	if reg == 6 {
		return 12
	}
	return 8
}

// cbRes handles RES b, r (CB group 2).
func (z *Z80) cbRes(bit uint8, reg uint8) int {
	value := z.readReg8(reg)
	value &^= (1 << bit)
	z.writeReg8(reg, value)
	if reg == 6 {
		return 15
	}
	return 8
}

// cbSet handles SET b, r (CB group 3).
func (z *Z80) cbSet(bit uint8, reg uint8) int {
	value := z.readReg8(reg)
	value |= (1 << bit)
	z.writeReg8(reg, value)
	if reg == 6 {
		return 15
	}
	return 8
}

// readReg8 reads an 8-bit register by index (0=B,1=C,2=D,3=E,4=H,5=L,6=(HL),7=A).
func (z *Z80) readReg8(reg uint8) uint8 {
	switch reg {
	case 0:
		return z.B
	case 1:
		return z.C
	case 2:
		return z.D
	case 3:
		return z.E
	case 4:
		return z.H
	case 5:
		return z.L
	case 6:
		return z.readByte(z.getHL())
	case 7:
		return z.A
	}
	return 0
}

// writeReg8 writes an 8-bit register by index.
func (z *Z80) writeReg8(reg uint8, value uint8) {
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
	case 6:
		z.writeByte(z.getHL(), value)
	case 7:
		z.A = value
	}
}
