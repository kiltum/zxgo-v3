package bus

import (
	"github.com/kiltum/zxgo-v3/pkg/io_ports"
	"github.com/kiltum/zxgo-v3/pkg/mem"
	"github.com/kiltum/zxgo-v3/pkg/model"
)

// DefaultBus is the standard BusReaderWriter implementation.
// It routes memory access through a MemoryMapper and I/O through a PortBus.
//
// Contention is modelled by accumulating extra T-states during contended memory
// accesses. The emulator loop must call UpdateFrameClock() before each instruction
// and FlushContention() afterwards to add the extra delays to the ULA step.
type DefaultBus struct {
	mapper  mem.MemoryMapper
	portBus *io_ports.PortBus
	cfg     model.Config

	// Contention tracking
	frameClock      int                // current T-state within frame (0 to ClockEndFrame-1)
	contentionAccum int                // accumulated contention delays since last flush
	kind            mem.ContentionKind // which delay pattern this model uses
	contended       bool               // false when the model has no contention at all

	// Per-machine-cycle timing. instrStart is the frame T-state at the start of
	// the instruction in progress and instrTick its absolute tick, both set by
	// the emulator before the instruction runs. OnMCycle then places frameClock
	// at the exact T-state of every access, and cycleDelta records how far into
	// the instruction the access sits (contention included) so the floating bus
	// can be sampled there. observed says whether the CPU reported any cycle for
	// this instruction, which decides whether the older successive-access
	// approximation in addContention is still needed.
	instrStart int
	instrTick  int64
	cycleDelta int
	observed   bool

	// writeTrace, when set, is called for every memory (kind 0) and port
	// (kind 1) write during an instruction. Used by the emulator delta trace.
	writeTrace func(kind byte, addr uint16, value uint8)

	// onMCycle, when set, is called for every machine cycle with its absolute
	// tick. The emulator uses it to timestamp audio and FDC writes in the cycle
	// they happen in rather than at the instruction boundary.
	onMCycle func(kind MCycleKind, addr uint16, tick int64, value uint8)
}

// NewDefaultBus creates a BusReaderWriter backed by the given mapper and port bus.
func NewDefaultBus(mapper mem.MemoryMapper, portBus *io_ports.PortBus) *DefaultBus {
	cfg := mapper.Model()
	// The pattern is resolved once here, not per access: 128K and +2A/+3 are
	// timing-identical, so only Config.ContentionModel can tell them apart.
	kind := mem.ContentionKindFor(cfg.ContentionModel)
	return &DefaultBus{
		mapper:    mapper,
		portBus:   portBus,
		cfg:       cfg,
		kind:      kind,
		contended: kind != mem.ContentionNone,
	}
}

// SetWriteTrace installs a callback for every memory/port write. kind is 0
// for memory, 1 for port.
func (b *DefaultBus) SetWriteTrace(f func(kind byte, addr uint16, value uint8)) {
	b.writeTrace = f
}

// SetMCycleCallback installs the per-machine-cycle callback.
func (b *DefaultBus) SetMCycleCallback(f func(kind MCycleKind, addr uint16, tick int64, value uint8)) {
	b.onMCycle = f
}

// SetInstructionTick records the absolute tick at which the next instruction
// starts, and clears the per-instruction cycle bookkeeping. The emulator calls
// it alongside UpdateFrameClock, before each instruction.
func (b *DefaultBus) SetInstructionTick(tick int64) {
	b.instrTick = tick
	b.cycleDelta = 0
	b.observed = false
}

// OnMCycle implements MCycleObserver: the CPU calls it once per machine cycle,
// before the access, with the cycle's offset inside the instruction.
func (b *DefaultBus) OnMCycle(kind MCycleKind, addr uint16, offset int, value uint8) {
	b.observed = true
	b.cycleDelta = offset + b.contentionAccum
	if b.kind != mem.ContentionNone {
		b.frameClock = (b.instrStart + b.cycleDelta) % b.cfg.ClockEndFrame()
	}
	// The refresh half of an M1 cycle drives the refresh address, so the ULA
	// contends on that instead of the fetch address - the 48K "snow" effect. It
	// shares the M1's T-states (advance 0): the delay is the whole point, the
	// cycle is already accounted for.
	//
	// 48K only. The effect needs I inside the contended range, and the ROM of a
	// 48K keeps I at 0x3F, which is ROM and therefore never contended - it only
	// appears when a program moves I there deliberately. On the 128K the ROM's
	// IM2 vector table lives in RAM, so I is contended as a matter of course and
	// applying this stalls every M1, which shows as a flickering, shifting
	// picture. Whatever the 128K's ULA really does here needs measuring on real
	// hardware; until then it gets only what the 48K is known to do.
	if kind == MCycleM1Refresh && b.kind == mem.Contention48k {
		b.addContention(addr, 0)
	}
	if b.onMCycle != nil {
		b.onMCycle(kind, addr, b.instrTick+int64(b.cycleDelta), value)
	}
}

// IODelta returns how far into the current instruction the cycle being accessed
// sits, contention included. The floating bus is sampled this far ahead of the
// ULA's instruction-start clock, plus the cycle itself (see ReadIO).
func (b *DefaultBus) IODelta() int { return b.cycleDelta }

// ReadMemory delegates to the memory mapper, accumulating contention delay.
func (b *DefaultBus) ReadMemory(addr uint16) uint8 {
	if b.contended {
		b.addContention(addr, 3) // non-M1 memory access is 3 T-states
	}
	return b.mapper.ReadByte(addr)
}

// FetchOpcode handles M1 cycle instruction reads with TR-DOS support.
// This is separate from ReadMemory to enable hardware activation via address trapping.
func (b *DefaultBus) FetchOpcode(addr uint16) uint8 {
	if b.contended {
		b.addContention(addr, 4) // M1 opcode fetch is 4 T-states
	}
	return b.mapper.FetchOpcode(addr)
}

// WriteMemory delegates to the memory mapper, accumulating contention delay.
func (b *DefaultBus) WriteMemory(addr uint16, value uint8) {
	if b.writeTrace != nil {
		b.writeTrace(0, addr, value)
	}
	if b.contended {
		b.addContention(addr, 3) // non-M1 memory access is 3 T-states
	}
	b.mapper.WriteByte(addr, value)
}

// ReadIO delegates to the port bus, accumulating I/O contention delay.
func (b *DefaultBus) ReadIO(port uint16) uint8 {
	delay := 0
	if b.contended {
		delay = b.addIOContention(port)
	}
	// Sample the floating bus where the chip is fetching at the moment the port
	// value is latched, contention included. Without per-cycle reporting this is
	// 0, which is the old instruction-granularity behaviour.
	delta := 0
	if b.observed {
		delta = b.cycleDelta + floatingBusSampleDelay + delay
	}
	b.portBus.SetFloatingOffset(delta)
	return b.portBus.Read(port)
}

// WriteIO delegates to the port bus, accumulating I/O contention delay.
func (b *DefaultBus) WriteIO(port uint16, value uint8) {
	if b.writeTrace != nil {
		b.writeTrace(1, port, value)
	}
	if b.contended {
		b.addIOContention(port)
	}
	b.portBus.Write(port, value)
}

// addContention calculates and accumulates contention delay for a memory access
// at the given address, then advances the frame clock by the access's base
// T-states: M1 opcode fetches are 4 T-states, all other memory accesses are 3
// (Fuse contend_read(PC,4) vs readbyte/writebyte tstates += 3). When the model
// has no contention ("" test harness, or "none" e.g. Pentagon), both the delay
// and the frame-clock advance are skipped.
func (b *DefaultBus) addContention(addr uint16, advance int) {
	// Only check for contention if the mapper says this address is in a contended bank.
	// T3: Use precomputed tables instead of heavy calculation
	if b.mapper.IsContended(addr) {
		d := int(mem.GetContentionDelay(b.kind, b.frameClock))
		if d > 0 {
			b.contentionAccum += d
		}
	}
	// An observing CPU places the frame clock exactly per cycle in OnMCycle, so
	// advancing it here as well would double-count. Without one, keep the
	// successive-access approximation - the best available.
	if !b.observed {
		b.frameClock = (b.frameClock + advance) % b.cfg.ClockEndFrame()
	}
}

// floatingBusSampleDelay is where inside the I/O cycle the floating bus is
// sampled, measured from the cycle's start. Fuse contends the port early, then
// late, and reads the floating bus in between (ref/fuse-1.9.0/periph.c:271-288
// -> readport_internal -> peripherals/ula.c:260-287): for an even port that is
// ioStart + 3 + the late contention. The rest of the cycle has no bearing on the
// byte, which only changes every four T-states anyway.
const floatingBusSampleDelay = 3

// addIOContention calculates contention delay for I/O port access at the given port.
// Only ports with A0 low reach the ULA, so only they can be contended. When the
// model has no contention at all ("" test harness, or "none" e.g. Pentagon) the
// delay is skipped -- applying it there skews border-timed loops diagonally. The
// +2A/+3 reaches here but its table is all zeros (no contended ports), so the
// frame clock still advances for its 4-T-state I/O cycle.
func (b *DefaultBus) addIOContention(port uint16) int {
	delay := 0
	if (port & 0x01) == 0 {
		// T2: Use precomputed I/O contention tables
		if d := int(mem.GetIOContentionDelay(b.kind, b.frameClock)); d > 0 {
			b.contentionAccum += d
			delay = d
		}
	}
	if !b.observed {
		b.frameClock = (b.frameClock + 4) % b.cfg.ClockEndFrame()
	}
	return delay
}

// UpdateFrameClock sets the current frame T-state position.
// Called by the emulator loop before each instruction.
// When the model has no contention (test harness, Pentagon), this is a no-op.
func (b *DefaultBus) UpdateFrameClock(tstate int) {
	if b.kind == mem.ContentionNone {
		return
	}
	b.frameClock = tstate % b.cfg.ClockEndFrame()
	b.instrStart = b.frameClock
}

// FlushContention returns accumulated contention delays and resets them.
// Called by the emulator loop after each instruction.
func (b *DefaultBus) FlushContention() int {
	d := b.contentionAccum
	b.contentionAccum = 0
	return d
}

// Mapper returns the underlying memory mapper (for ULA use).
func (b *DefaultBus) Mapper() mem.MemoryMapper {
	return b.mapper
}

// PortBus returns the underlying port bus (for registering handlers).
func (b *DefaultBus) PortBus() *io_ports.PortBus {
	return b.portBus
}

// Ensure DefaultBus implements BusReaderWriter.
var _ BusReaderWriter = (*DefaultBus)(nil)
