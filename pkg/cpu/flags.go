package cpu

// ---------- Flag calculation helpers ----------

// updateSZ updates S and Z flags from an 8-bit result.
func (z *Z80) updateSZ(result uint8) {
	z.setFlagCond(FLAG_S, (result&0x80) != 0)
	z.setFlagCond(FLAG_Z, result == 0)
}

// updateXYFromValue updates X and Y flags from an 8-bit value.
func (z *Z80) updateXYFromValue(value uint8) {
	z.setFlagCond(FLAG_X, (value&FLAG_X) != 0)
	z.setFlagCond(FLAG_Y, (value&FLAG_Y) != 0)
}

// updateXYFromAddr updates X and Y flags from the high byte of an address.
func (z *Z80) updateXYFromAddr(addr uint16) {
	hi := uint8(addr >> 8)
	z.setFlagCond(FLAG_X, (hi&FLAG_X) != 0)
	z.setFlagCond(FLAG_Y, (hi&FLAG_Y) != 0)
}

// updateSZXY updates S, Z, X, Y flags from an 8-bit result.
func (z *Z80) updateSZXY(result uint8) {
	z.updateSZ(result)
	z.updateXYFromValue(result)
}

// updateSZXYPV updates S, Z, X, Y, and P/V flags from an 8-bit result.
// P/V is set to parity (even -> 1, odd -> 0).
func (z *Z80) updateSZXYPV(result uint8) {
	z.updateSZ(result)
	z.updateXYFromValue(result)
	z.setFlagCond(FLAG_PV, parityEven(result))
}

// ---------- Parity ----------

// parityEven returns true if the value has an even number of 1-bits.
// On Z80, P/V flag = 1 for even parity, 0 for odd parity.
// This is the CORRECTED version -- the original zxgo had this inverted.
func parityEven(val uint8) bool {
	count := 0
	for i := 0; i < 8; i++ {
		if val&(1<<i) != 0 {
			count++
		}
	}
	return count%2 == 0
}

// ---------- 8-bit ALU operations ----------

// inc8 increments an 8-bit value and updates flags. Returns the new value.
func (z *Z80) inc8(value uint8) uint8 {
	result := value + 1
	z.setFlagCond(FLAG_H, (value&0x0F) == 0x0F)
	z.F &^= FLAG_N // clear N
	z.updateSZXY(result)
	// PV set if incrementing 0x7F (overflow from positive to negative)
	z.setFlagCond(FLAG_PV, value == 0x7F)
	return result
}

// dec8 decrements an 8-bit value and updates flags. Returns the new value.
func (z *Z80) dec8(value uint8) uint8 {
	result := value - 1
	z.setFlagCond(FLAG_H, (value&0x0F) == 0x00)
	z.setFlag(FLAG_N, true)
	z.updateSZXY(result)
	// PV set if decrementing 0x80 (overflow from negative to positive)
	z.setFlagCond(FLAG_PV, value == 0x80)
	return result
}

// add8 adds an 8-bit value to A and updates flags.
func (z *Z80) add8(value uint8) {
	a := z.A
	result := uint16(a) + uint16(value)
	z.setFlagCond(FLAG_C, result > 0xFF)
	z.setFlagCond(FLAG_H, (a&0x0F)+(value&0x0F) > 0x0F)
	z.F &^= FLAG_N
	r := uint8(result)
	z.updateSZXY(r)
	// Overflow: same sign operands, different sign result
	overflow := ((a^value)&0x80 == 0) && ((a^r)&0x80 != 0)
	z.setFlagCond(FLAG_PV, overflow)
	z.A = r
}

// adc8 adds an 8-bit value + carry to A and updates flags.
func (z *Z80) adc8(value uint8) {
	a := z.A
	carry := uint16(0)
	if z.getFlag(FLAG_C) {
		carry = 1
	}
	result := uint16(a) + uint16(value) + carry
	z.setFlagCond(FLAG_C, result > 0xFF)
	z.setFlagCond(FLAG_H, (a&0x0F)+(value&0x0F)+uint8(carry) > 0x0F)
	z.F &^= FLAG_N
	r := uint8(result)
	z.updateSZXY(r)
	overflow := ((a^value)&0x80 == 0) && ((a^r)&0x80 != 0)
	z.setFlagCond(FLAG_PV, overflow)
	z.A = r
}

// sub8 subtracts an 8-bit value from A and updates flags.
func (z *Z80) sub8(value uint8) {
	a := z.A
	result := uint16(a) - uint16(value)
	z.setFlagCond(FLAG_C, a < value)
	z.setFlagCond(FLAG_H, (a&0x0F) < (value&0x0F))
	z.setFlag(FLAG_N, true)
	r := uint8(result)
	z.updateSZXY(r)
	// Overflow: operands have different signs AND result has same sign as subtrahend
	overflow := ((a^value)&0x80 != 0) && ((a^r)&0x80 != 0)
	z.setFlagCond(FLAG_PV, overflow)
	z.A = r
}

// sbc8 subtracts an 8-bit value + carry from A and updates flags.
func (z *Z80) sbc8(value uint8) {
	a := z.A
	carry := uint16(0)
	if z.getFlag(FLAG_C) {
		carry = 1
	}
	result := uint16(a) - uint16(value) - carry
	z.setFlagCond(FLAG_C, uint16(a) < uint16(value)+carry)
	z.setFlagCond(FLAG_H, (a&0x0F) < (value&0x0F)+uint8(carry))
	z.setFlag(FLAG_N, true)
	r := uint8(result)
	z.updateSZXY(r)
	overflow := ((a^value)&0x80 != 0) && ((a^r)&0x80 != 0)
	z.setFlagCond(FLAG_PV, overflow)
	z.A = r
}

// and8 performs bitwise AND with A and updates flags.
func (z *Z80) and8(value uint8) {
	z.A &= value
	z.setFlagCond(FLAG_C, false)
	z.setFlagCond(FLAG_N, false)
	z.setFlagCond(FLAG_H, true)
	z.updateSZXY(z.A)
	z.setFlagCond(FLAG_PV, parityEven(z.A))
}

// xor8 performs bitwise XOR with A and updates flags.
func (z *Z80) xor8(value uint8) {
	z.A ^= value
	z.setFlagCond(FLAG_C, false)
	z.setFlagCond(FLAG_N, false)
	z.setFlagCond(FLAG_H, false)
	z.updateSZXY(z.A)
	z.setFlagCond(FLAG_PV, parityEven(z.A))
}

// or8 performs bitwise OR with A and updates flags.
func (z *Z80) or8(value uint8) {
	z.A |= value
	z.setFlagCond(FLAG_C, false)
	z.setFlagCond(FLAG_N, false)
	z.setFlagCond(FLAG_H, false)
	z.updateSZXY(z.A)
	z.setFlagCond(FLAG_PV, parityEven(z.A))
}

// cp8 compares A with an 8-bit value and updates flags (A unchanged).
func (z *Z80) cp8(value uint8) {
	a := z.A
	result := uint16(a) - uint16(value)
	z.setFlagCond(FLAG_C, a < value)
	z.setFlagCond(FLAG_H, (a&0x0F) < (value&0x0F))
	z.setFlag(FLAG_N, true)
	r := uint8(result)
	z.updateSZ(r)
	// X and Y from the operand (not the result) for CP
	z.updateXYFromValue(value)
	overflow := ((a^value)&0x80 != 0) && ((a^r)&0x80 != 0)
	z.setFlagCond(FLAG_PV, overflow)
}

// ---------- 16-bit ALU operations ----------

// add16 adds two 16-bit values and updates flags. Returns the sum.
func (z *Z80) add16(a, b uint16) uint16 {
	result := uint32(a) + uint32(b)
	z.setFlagCond(FLAG_C, result > 0xFFFF)
	z.setFlagCond(FLAG_H, (a&0x0FFF)+(b&0x0FFF) > 0x0FFF)
	z.F &^= FLAG_N
	z.updateXYFromAddr(uint16(result))
	return uint16(result)
}

// adc16 adds two 16-bit values + carry and updates flags. Returns the sum.
func (z *Z80) adc16(a, b uint16) uint16 {
	carry := uint32(0)
	if z.getFlag(FLAG_C) {
		carry = 1
	}
	result := uint32(a) + uint32(b) + carry
	r := uint16(result)
	z.setFlagCond(FLAG_S, r&0x8000 != 0)
	z.setFlagCond(FLAG_Z, r == 0)
	z.setFlagCond(FLAG_H, (a&0x0FFF)+(b&0x0FFF)+uint16(carry) > 0x0FFF)
	overflow := ((a^b)&0x8000 == 0) && ((a^r)&0x8000 != 0)
	z.setFlagCond(FLAG_PV, overflow)
	z.F &^= FLAG_N
	z.setFlagCond(FLAG_C, result > 0xFFFF)
	z.updateXYFromAddr(r)
	z.MEMPTR = a + 1
	return r
}

// sbc16 subtracts two 16-bit values + carry and updates flags. Returns the difference.
func (z *Z80) sbc16(a, b uint16) uint16 {
	carry := int32(0)
	if z.getFlag(FLAG_C) {
		carry = 1
	}
	result := int32(a) - int32(b) - carry
	r := uint16(result)
	halfCarry := (int16(a&0x0FFF) - int16(b&0x0FFF) - int16(carry)) < 0
	overflow := ((a^b)&0x8000 != 0) && ((a^r)&0x8000 != 0)

	z.setFlagCond(FLAG_S, r&0x8000 != 0)
	z.setFlagCond(FLAG_Z, r == 0)
	z.setFlagCond(FLAG_H, halfCarry)
	z.setFlagCond(FLAG_PV, overflow)
	z.setFlag(FLAG_N, true)
	z.setFlagCond(FLAG_C, result < 0)
	z.updateXYFromAddr(r)
	z.MEMPTR = a + 1
	return r
}

// ---------- Rotates ----------

func (z *Z80) rlca() {
	result := (z.A << 1) | (z.A >> 7)
	z.setFlagCond(FLAG_C, (z.A&0x80) != 0)
	z.F &^= FLAG_H | FLAG_N
	z.A = result
	z.updateXYFromValue(z.A)
}

func (z *Z80) rla() {
	oldCarry := z.getFlag(FLAG_C)
	result := z.A << 1
	if oldCarry {
		result |= 0x01
	}
	z.setFlagCond(FLAG_C, (z.A&0x80) != 0)
	z.F &^= FLAG_H | FLAG_N
	z.A = result
	z.updateXYFromValue(z.A)
}

func (z *Z80) rrca() {
	result := (z.A >> 1) | (z.A << 7)
	z.setFlagCond(FLAG_C, (z.A&0x01) != 0)
	z.F &^= FLAG_H | FLAG_N
	z.A = result
	z.updateXYFromValue(z.A)
}

func (z *Z80) rra() {
	oldCarry := z.getFlag(FLAG_C)
	result := z.A >> 1
	if oldCarry {
		result |= 0x80
	}
	z.setFlagCond(FLAG_C, (z.A&0x01) != 0)
	z.F &^= FLAG_H | FLAG_N
	z.A = result
	z.updateXYFromValue(z.A)
}

// ---------- Miscellaneous ----------

func (z *Z80) cpl() {
	z.A = ^z.A
	z.setFlag(FLAG_H, true)
	z.setFlag(FLAG_N, true)
	z.updateXYFromValue(z.A)
}

func (z *Z80) scf() {
	z.setFlag(FLAG_C, true)
	z.F &^= FLAG_H | FLAG_N
	if z.IsNMOS {
		z.updateXYFromValue(z.A)
	} else {
		// CMOS: set X and Y only if both bits 3 and 5 are set in A
		if (z.A&FLAG_Y) == FLAG_Y && (z.A&FLAG_X) == FLAG_X {
			z.F |= FLAG_X | FLAG_Y
		}
	}
}

func (z *Z80) ccf() {
	oldCarry := z.getFlag(FLAG_C)
	z.setFlagCond(FLAG_C, !oldCarry)
	z.setFlagCond(FLAG_H, oldCarry) // H = old C
	z.F &^= FLAG_N
	if z.IsNMOS {
		z.updateXYFromValue(z.A)
	} else {
		if (z.A&FLAG_Y) == FLAG_Y && (z.A&FLAG_X) == FLAG_X {
			z.F |= FLAG_X | FLAG_Y
		}
	}
}

func (z *Z80) daa() {
	temp := z.A
	correction := uint8(0)
	carry := z.getFlag(FLAG_C)

	if z.getFlag(FLAG_H) || (z.A&0x0F) > 9 {
		correction |= 0x06
	}
	if carry || z.A > 0x99 {
		correction |= 0x60
	}

	if z.getFlag(FLAG_N) {
		z.A -= correction
	} else {
		z.A += correction
	}

	z.setFlagCond(FLAG_S, (z.A&0x80) != 0)
	z.setFlagCond(FLAG_Z, z.A == 0)
	z.setFlagCond(FLAG_H, ((temp^correction^z.A)&0x10) != 0)
	z.setFlagCond(FLAG_PV, parityEven(z.A))
	z.setFlagCond(FLAG_C, carry || (correction&0x60 != 0))
	z.updateXYFromValue(z.A)
}

func (z *Z80) neg() {
	value := z.A
	z.A = 0
	z.sub8(value)
}

// boolToByte converts bool to 0 or 1.
func boolToByte(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}
