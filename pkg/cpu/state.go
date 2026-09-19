package cpu

import "github.com/kiltum/zxgo-v3/pkg/state"

// SaveState and LoadState are the CPU's half of the native state format
// (STATE_DESIGN.md). They are additive: nothing here changes how an instruction
// executes, and the Fuse and ZEXALL suites are the gate on that.
//
// The payload is written in the field order below and is read back the same
// way. A field added later is appended and the chunk version bumped; a reader
// checks More rather than assuming a layout.
//
// Two things are deliberately absent:
//
//   - The bus and the machine-cycle observer. They are wiring, decoded from the
//     machine, and LoadState must never replace them: the CPU in a restored
//     machine is the same object, holding the same bus.
//
// interruptPending IS stored. It is the /INT level sampled at the last
// instruction boundary, and while the *Spectrum's* CPU has its level recomputed
// by the emulator after a load (the ULA's interrupt deadline is the source of
// truth for it), a CPU inside a device does not: the General Sound's Z80 latches
// its own level when the card's interrupt period is crossed, and nothing outside
// the card can recompute that. Dropping it made a restored card take an
// interrupt an instruction late, which moved a DAC write and changed the audio.
//
// cycleOffset is also not stored. It is per-instruction bookkeeping that is zero
// at every boundary, and a state is only ever written or read between two
// instructions.

// SaveState encodes every register and behaviour switch the CPU owns.
func (z *Z80) SaveState(e *state.Encoder) error {
	// Main registers.
	e.U8(z.A)
	e.U8(z.F)
	e.U8(z.B)
	e.U8(z.C)
	e.U8(z.D)
	e.U8(z.E)
	e.U8(z.H)
	e.U8(z.L)

	// Alternate set. It holds a partially executed EX AF,AF' or EXX, so it is
	// live state rather than a convenience copy.
	e.U8(z.A_)
	e.U8(z.F_)
	e.U8(z.B_)
	e.U8(z.C_)
	e.U8(z.D_)
	e.U8(z.E_)
	e.U8(z.H_)
	e.U8(z.L_)

	e.U16(z.IX)
	e.U16(z.IY)
	e.U16(z.SP)
	e.U16(z.PC)

	// R is the 7-bit refresh counter; its top bit is not part of the count but
	// the CPU preserves whatever was written there, so the byte is stored whole.
	e.U8(z.I)
	e.U8(z.R)
	e.U8(z.IM)

	e.Bool(z.IFF1)
	e.Bool(z.IFF2)
	e.Bool(z.HALT)
	e.U16(z.MEMPTR)

	// The EI delay: a pending EI takes effect after the next instruction, so it
	// spans the boundary a state is written at.
	e.Bool(z.eipending)

	// The /INT level sampled at the last instruction boundary.
	e.Bool(z.interruptPending)

	// The two behaviour switches. They are not "configuration" in the sense that
	// disqualifies them from the file: IsNMOS decides how the flags of every
	// later instruction come out, and memptrReal decides what the repeating
	// block I/O instructions leave in MEMPTR, so a restored machine with the
	// wrong pair would diverge immediately.
	e.Bool(z.IsNMOS)
	e.Bool(z.memptrReal)
	return nil
}

// LoadState applies a payload written by SaveState.
//
// It parses the whole payload into locals before assigning, so a payload that
// stops early leaves the CPU exactly as it was.
func (z *Z80) LoadState(d *state.Decoder) error {
	var (
		a, f, b, c, dd, ee, h, l       uint8
		a_, f_, b_, c_, d_, e_, h_, l_ uint8
		ix, iy, sp, pc, memptr         uint16
		i, r, im                       uint8
		iff1, iff2, halted             bool
		eipending, isNMOS, memptrReal  bool
		intPending                     bool
	)

	a, f = d.U8(), d.U8()
	b, c = d.U8(), d.U8()
	dd, ee = d.U8(), d.U8()
	h, l = d.U8(), d.U8()

	a_, f_ = d.U8(), d.U8()
	b_, c_ = d.U8(), d.U8()
	d_, e_ = d.U8(), d.U8()
	h_, l_ = d.U8(), d.U8()

	ix, iy = d.U16(), d.U16()
	sp, pc = d.U16(), d.U16()

	i, r, im = d.U8(), d.U8(), d.U8()

	iff1, iff2, halted = d.Bool(), d.Bool(), d.Bool()
	memptr = d.U16()
	eipending = d.Bool()
	intPending = d.Bool()
	isNMOS, memptrReal = d.Bool(), d.Bool()

	if err := d.Err(); err != nil {
		return err
	}

	z.A, z.F = a, f
	z.B, z.C = b, c
	z.D, z.E = dd, ee
	z.H, z.L = h, l

	z.A_, z.F_ = a_, f_
	z.B_, z.C_ = b_, c_
	z.D_, z.E_ = d_, e_
	z.H_, z.L_ = h_, l_

	z.IX, z.IY = ix, iy
	z.SP, z.PC = sp, pc

	z.I, z.R, z.IM = i, r, im

	z.IFF1, z.IFF2, z.HALT = iff1, iff2, halted
	z.MEMPTR = memptr
	z.eipending = eipending
	z.interruptPending = intPending
	z.IsNMOS, z.memptrReal = isNMOS, memptrReal
	return nil
}

var _ state.Component = (*Z80)(nil)
