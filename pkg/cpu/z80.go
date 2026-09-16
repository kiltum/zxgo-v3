// Package cpu implements a Z80 CPU emulator with support for all documented
// and undocumented opcodes, flags, and registers.
//
// The CPU communicates with the outside world through a bus.BusReaderWriter
// interface, which abstracts memory and I/O access. This decoupling enables
// contention modelling, memory paging, and clean testing.
package cpu

import (
	"log/slog"

	"github.com/kiltum/zxgo-v3/pkg/bus"
)

// intLog is an optional logger for interrupt timing diagnostics.
var intLog *slog.Logger

// SetInterruptLogger wires a logger into the CPU for interrupt tracing.
func SetInterruptLogger(l *slog.Logger) { intLog = l }

// FLAG_* constants represent the bit positions of flags in the F register.
const (
	FLAG_C  = 0x01 // Carry flag
	FLAG_N  = 0x02 // Add/Subtract flag
	FLAG_PV = 0x04 // Parity/Overflow flag
	FLAG_X  = 0x08 // Undocumented bit 3 flag (from result)
	FLAG_H  = 0x10 // Half Carry flag
	FLAG_Y  = 0x20 // Undocumented bit 5 flag (from result)
	FLAG_Z  = 0x40 // Zero flag
	FLAG_S  = 0x80 // Sign flag
)

// Z80 represents a Z80 CPU.
// All fields are exported for snapshot/debugger access.
type Z80 struct {
	// Main registers
	A    uint8 // Accumulator
	F    uint8 // Flags register
	B, C uint8 // BC register pair
	D, E uint8 // DE register pair
	H, L uint8 // HL register pair

	// Alternate registers
	A_, F_ uint8 // Alternate AF
	B_, C_ uint8 // Alternate BC
	D_, E_ uint8 // Alternate DE
	H_, L_ uint8 // Alternate HL

	// Index registers
	IX uint16 // Index register X
	IY uint16 // Index register Y

	// Special registers
	SP               uint16 // Stack pointer
	PC               uint16 // Program counter
	I                uint8  // Interrupt vector
	R                uint8  // Memory refresh (7-bit, bit 7 preserved)
	IM               uint8  // Interrupt mode (0, 1, or 2)
	IFF1             bool   // Interrupt flip-flop 1 (master enable)
	IFF2             bool   // Interrupt flip-flop 2 (temporary storage)
	HALT             bool   // HALT state
	MEMPTR           uint16 // Undocumented MEMPTR (WZ) register
	interruptPending bool   // Current /INT level sampled at the instruction boundary

	// EI delay: after EI, interrupts should be enabled after the NEXT instruction.
	// eipending is set by EI and consumed by the next ExecuteOneInstruction call.
	eipending bool

	IsNMOS bool // CPU type: NMOS (true) or CMOS (false)

	// Bus interface -- the CPU's only connection to the outside world.
	bus bus.BusReaderWriter

	// mcycle is the bus asking to see each machine cycle, resolved once in New
	// and nil for every bus that does not implement bus.MCycleObserver. It must
	// never be type-asserted per access: this is the hottest path in the CPU.
	mcycle bus.MCycleObserver
	// cycleOffset is the T-state position of the next machine cycle within the
	// instruction being executed. Only meaningful while an instruction runs.
	cycleOffset int

	// memptrReal selects the MEMPTR behaviour of the repeating block I/O
	// instructions. Two answers are in circulation and they disagree:
	//
	//   true  ("real")       MEMPTR = PC+1 while the instruction repeats, so on
	//                        the last iteration it is fixed by the instruction's
	//                        address rather than by BC. This is what
	//                        testdata/cpd-test.tzx measures on hardware, and the
	//                        default.
	//   false ("documented") the BC-derived value the Zilog documentation
	//                        implies, which Fuse's test data encodes.
	//
	// The two are visible only on the repeat path of INIR/INDR/OTIR/OTDR, and
	// nothing in between: the last (non-repeating) iteration is BC-derived either
	// way. The CPU test suite runs with false so it can keep Fuse as its oracle;
	// everything else runs with the default.
	memptrReal bool
}

// New creates a Z80 CPU connected to the given bus.
func New(b bus.BusReaderWriter) *Z80 {
	z := &Z80{
		bus:              b,
		SP:               0xFFFF, // Top of memory after reset
		IFF1:             false,
		IFF2:             false,
		interruptPending: false,
		IsNMOS:           true, // Default to NMOS
		memptrReal:       true, // Default to the measured hardware behaviour
	}
	// Resolve the optional per-machine-cycle observer once, here, so no access
	// pays for a type assertion later.
	if o, ok := b.(bus.MCycleObserver); ok {
		z.mcycle = o
	}
	return z
}

// SetCPUType sets the CPU type. true = NMOS, false = CMOS.
func (z *Z80) SetCPUType(isNMOS bool) {
	z.IsNMOS = isNMOS
}

// SetMEMPTRReal selects the MEMPTR behaviour of the repeating block I/O
// instructions. true is the measured hardware behaviour (and testdata/cpd-test's
// oracle); false is the documented BC-derived one that Fuse's data expects. See
// the field comment for what actually differs - only the repeat path.
func (z *Z80) SetMEMPTRReal(on bool) {
	z.memptrReal = on
}

// SetInterruptLevel sets the current /INT level seen by the CPU.
// The Z80 samples /INT at the end of each instruction, so the emulator calls
// this once per instruction with the level at the instruction boundary. Setting
// active=false once a pulse has ended ensures a level-triggered interrupt is not
// latched past the pulse (the previous CallInterrupt edge design could do that).
func (z *Z80) SetInterruptLevel(active bool) {
	z.interruptPending = active
}

// ExecuteOneInstruction executes a single instruction and returns T-states used.
func (z *Z80) ExecuteOneInstruction() int {
	// Machine-cycle positions are relative to the start of this instruction.
	z.cycleOffset = 0

	// EI delay: if eipending was set by a previous EI, the next instruction runs
	// normally (IFF1 already set), but interrupts are blocked for THIS instruction.
	// When this call returns and the NEXT instruction starts, eipending is false.
	blockInterrupt := z.eipending
	z.eipending = false

	// Handle interrupts if enabled and not blocked by EI delay.
	// On real Z80, INT is sampled at the end of each instruction.
	// If IFF1 is false when sampled, the pulse is missed -- it does NOT persist.
	// CRITICAL: Interrupt handling must come BEFORE HALT check - HALT can respond to interrupts!
	if z.IFF1 && z.interruptPending && !blockInterrupt {
		z.interruptPending = false // Only clear when actually taking interrupt
		if intLog != nil {
			intLog.Debug("int_taken", "pc", z.PC, "im", z.IM)
		}
		return z.handleInterrupt()
	}

	// Handle HALT state
	if z.HALT {
		return 4 // 4 T-states for HALT
	}

	// Read the next opcode
	opcode := z.readOpcode()

	// Handle prefixed opcodes
	switch opcode {
	case 0xCB:
		cbOpcode := z.readOpcode()
		return z.executeCBOpcode(cbOpcode)
	case 0xDD:
		ddOpcode := z.readOpcode()
		return z.executeDDOpcode(ddOpcode)
	case 0xED:
		edOpcode := z.readOpcode()
		return z.executeEDOpcode(edOpcode)
	case 0xFD:
		fdOpcode := z.readOpcode()
		return z.executeFDOpcode(fdOpcode)
	default:
		return z.executeOpcode(opcode)
	}
}

// handleInterrupt handles interrupt processing.
func (z *Z80) handleInterrupt() int {
	// Exit HALT state
	if z.HALT {
		z.HALT = false
		z.PC++
	}

	// Disable interrupts
	z.IFF1 = false
	z.IFF2 = false

	// Handle based on interrupt mode. Accepting an interrupt also updates
	// MEMPTR: it ends up holding the address the CPU jumped to, which is what
	// an ISR observes (and what cpd-test's interrupt test checks).
	switch z.IM {
	case 0, 1:
		// Mode 0/1: RST 38h
		z.push(z.PC)
		z.PC = 0x0038
		z.MEMPTR = 0x0038
		return 13
	case 2:
		// Mode 2: Call via I register + bus value
		z.push(z.PC)
		vectorAddr := (uint16(z.I) << 8) | 0xFF
		lo := z.readByte(vectorAddr)
		hi := z.readByte(vectorAddr + 1)
		z.PC = (uint16(hi) << 8) | uint16(lo)
		// MEMPTR holds the handler address that was fetched from the table.
		z.MEMPTR = z.PC
		return 19
	default:
		return 0
	}
}

// Bus returns the CPU's bus interface (for debugger/snapshot use).
func (z *Z80) Bus() bus.BusReaderWriter {
	return z.bus
}

// ---------- Bus access helpers ----------
//
// Every one of these reports its machine cycle before the access. The offsets
// are the nominal cycle costs (M1 4T, memory 3T, I/O 4T), which places each
// access exactly for instructions whose accesses are separated only by other
// accesses; the few instructions with internal T-states between two accesses
// (INC (HL) is 4+3+1+3) report the second one a T-state or two early. Closing
// that residual would mean editing T-state literals across the opcode files,
// which the CPU's frozen-semantics rule forbids - the Fuse MC records pin how
// much is left rather than pretending it is gone.

// refresh reports the refresh half of an M1 cycle, which shares the M1's offset.
func (z *Z80) refresh(kind bus.MCycleKind, addr uint16) {
	if z.mcycle != nil {
		z.mcycle.OnMCycle(kind, addr, z.cycleOffset-4, 0)
	}
}

// cycle reports one machine cycle to the bus, if it is observing. The offset
// advances by the nominal cost so the next access lands in the right place.
func (z *Z80) cycle(kind bus.MCycleKind, addr uint16, tstates int) {
	z.cycleValue(kind, addr, tstates, 0)
}

// cycleValue is cycle for the accesses where the CPU drives the bus, so the
// consumer can see the byte (the ULA reads it during the 48K snow effect).
func (z *Z80) cycleValue(kind bus.MCycleKind, addr uint16, tstates int, value uint8) {
	if z.mcycle != nil {
		z.mcycle.OnMCycle(kind, addr, z.cycleOffset, value)
	}
	z.cycleOffset += tstates
}

func (z *Z80) readByte(addr uint16) uint8 {
	z.cycle(bus.MCycleMemRead, addr, 3)
	return z.bus.ReadMemory(addr)
}

func (z *Z80) writeByte(addr uint16, value uint8) {
	z.cycleValue(bus.MCycleMemWrite, addr, 3, value)
	z.bus.WriteMemory(addr, value)
}

func (z *Z80) readWord(addr uint16) uint16 {
	lo := z.readByte(addr)
	hi := z.readByte(addr + 1)
	return (uint16(hi) << 8) | uint16(lo)
}

func (z *Z80) writeWord(addr uint16, value uint16) {
	z.writeByte(addr, uint8(value))
	z.writeByte(addr+1, uint8(value>>8))
}

func (z *Z80) readPort(port uint16) uint8 {
	z.cycle(bus.MCycleIORead, port, 4)
	return z.bus.ReadIO(port)
}

func (z *Z80) writePort(port uint16, value uint8) {
	z.cycleValue(bus.MCycleIOWrite, port, 4, value)
	z.bus.WriteIO(port, value)
}

// ---------- Instruction helpers ----------

func (z *Z80) readImmediateByte() uint8 {
	v := z.readByte(z.PC)
	z.PC++
	return v
}

func (z *Z80) readImmediateWord() uint16 {
	lo := z.readByte(z.PC)
	z.PC++
	hi := z.readByte(z.PC)
	z.PC++
	return (uint16(hi) << 8) | uint16(lo)
}

func (z *Z80) readDisplacement() int8 {
	v := z.readByte(z.PC)
	z.PC++
	return int8(v)
}

func (z *Z80) readOpcode() uint8 {
	z.cycle(bus.MCycleM1, z.PC, 4)
	opcode := z.bus.FetchOpcode(z.PC)
	z.PC++
	// R register: 7-bit counter, bit 7 preserved
	z.R = (z.R & 0x80) | ((z.R + 1) & 0x7F)
	// Second half of M1: the bus carries the refresh address, not the fetch
	// address. Reported at the M1's own offset with no T-states of its own, so a
	// bus can contend on it (the 48K snow effect) without disturbing the cycle
	// accounting.
	z.refresh(bus.MCycleM1Refresh, (uint16(z.I)<<8)|uint16(z.R))
	return opcode
}

// RequiresInterrupt returns true if the CPU should take an interrupt
// Used by the emulator to sample the interrupt level state
func (z *Z80) RequiresInterrupt() bool {
	return z.IFF1 && !z.eipending
}

func (z *Z80) push(value uint16) {
	z.SP -= 2
	z.writeWord(z.SP, value)
}

func (z *Z80) pop() uint16 {
	v := z.readWord(z.SP)
	z.SP += 2
	return v
}

// ---------- Register access helpers ----------

func (z *Z80) getAF() uint16  { return (uint16(z.A) << 8) | uint16(z.F) }
func (z *Z80) setAF(v uint16) { z.A = uint8(v >> 8); z.F = uint8(v) }
func (z *Z80) getBC() uint16  { return (uint16(z.B) << 8) | uint16(z.C) }
func (z *Z80) setBC(v uint16) { z.B = uint8(v >> 8); z.C = uint8(v) }
func (z *Z80) getDE() uint16  { return (uint16(z.D) << 8) | uint16(z.E) }
func (z *Z80) setDE(v uint16) { z.D = uint8(v >> 8); z.E = uint8(v) }
func (z *Z80) getHL() uint16  { return (uint16(z.H) << 8) | uint16(z.L) }
func (z *Z80) setHL(v uint16) { z.H = uint8(v >> 8); z.L = uint8(v) }

func (z *Z80) getAF_() uint16  { return (uint16(z.A_) << 8) | uint16(z.F_) }
func (z *Z80) setAF_(v uint16) { z.A_ = uint8(v >> 8); z.F_ = uint8(v) }
func (z *Z80) getBC_() uint16  { return (uint16(z.B_) << 8) | uint16(z.C_) }
func (z *Z80) setBC_(v uint16) { z.B_ = uint8(v >> 8); z.C_ = uint8(v) }
func (z *Z80) getDE_() uint16  { return (uint16(z.D_) << 8) | uint16(z.E_) }
func (z *Z80) setDE_(v uint16) { z.D_ = uint8(v >> 8); z.E_ = uint8(v) }
func (z *Z80) getHL_() uint16  { return (uint16(z.H_) << 8) | uint16(z.L_) }
func (z *Z80) setHL_(v uint16) { z.H_ = uint8(v >> 8); z.L_ = uint8(v) }

func (z *Z80) getIXH() uint8  { return uint8(z.IX >> 8) }
func (z *Z80) getIXL() uint8  { return uint8(z.IX) }
func (z *Z80) getIYH() uint8  { return uint8(z.IY >> 8) }
func (z *Z80) getIYL() uint8  { return uint8(z.IY) }
func (z *Z80) setIXH(v uint8) { z.IX = (z.IX & 0x00FF) | (uint16(v) << 8) }
func (z *Z80) setIXL(v uint8) { z.IX = (z.IX & 0xFF00) | uint16(v) }
func (z *Z80) setIYH(v uint8) { z.IY = (z.IY & 0x00FF) | (uint16(v) << 8) }
func (z *Z80) setIYL(v uint8) { z.IY = (z.IY & 0xFF00) | uint16(v) }

// ---------- Flag access ----------

func (z *Z80) getFlag(flag uint8) bool { return (z.F & flag) != 0 }

func (z *Z80) setFlag(flag uint8, state bool) {
	if state {
		z.F |= flag
	} else {
		z.F &^= flag
	}
}

func (z *Z80) setFlagCond(flag uint8, condition bool) {
	if condition {
		z.F |= flag
	} else {
		z.F &^= flag
	}
}
