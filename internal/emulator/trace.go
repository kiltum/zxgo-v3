package emulator

import "github.com/kiltum/zxgo-v3/pkg/cpu"

// TraceRecord is one instruction's state delta: the PC before the instruction,
// the registers that changed, and the memory and port bytes written.
type TraceRecord struct {
	PC    uint16       `json:"pc"`
	MW    uint16       `json:"mw"`
	Regs  []RegChange  `json:"regs,omitempty"`
	Mem   []ByteChange `json:"mem,omitempty"`
	Ports []PortChange `json:"ports,omitempty"`
}

// RegChange names a register and its new value.
type RegChange struct {
	Reg   string `json:"reg"`
	Value uint16 `json:"val"`
}

// ByteChange is a single memory byte write.
type ByteChange struct {
	Addr  uint16 `json:"a"`
	Value uint8  `json:"v"`
}

// PortChange is a single I/O port write.
type PortChange struct {
	Port  uint16 `json:"p"`
	Value uint8  `json:"v"`
}

// Trace is a fixed-size ring buffer of per-instruction deltas. Recording is off
// by default; the emulator's step() fills it when enabled.
type Trace struct {
	enabled bool
	records []TraceRecord
	head    int
	count   int

	// scratch for the instruction currently executing
	cur        TraceRecord
	memWrites  []ByteChange
	portWrites []PortChange
}

// registerNames is indexed by snapshotRegs order.
var registerNames = [7]string{"AF", "BC", "DE", "HL", "SP", "IX", "IY"}

func newTrace(size int) *Trace {
	if size < 16 {
		size = 16
	}
	return &Trace{records: make([]TraceRecord, size)}
}

// SetEnabled turns recording on or off.
func (t *Trace) SetEnabled(on bool) { t.enabled = on }

// Enabled reports whether recording is on.
func (t *Trace) Enabled() bool { return t.enabled }

// beginInstruction resets the per-instruction scratch and records the PC.
func (t *Trace) beginInstruction(pc uint16) {
	t.cur = TraceRecord{PC: pc}
	t.memWrites = t.memWrites[:0]
	t.portWrites = t.portWrites[:0]
}

// recordWrite captures a memory (kind 0) or port (kind 1) write.
func (t *Trace) recordWrite(kind byte, addr uint16, value uint8) {
	if kind == 0 {
		t.memWrites = append(t.memWrites, ByteChange{addr, value})
	} else {
		t.portWrites = append(t.portWrites, PortChange{addr, value})
	}
}

// endInstruction computes the register delta against the before snapshot and
// appends the record.
func (t *Trace) endInstruction(before [7]uint16, after *cpu.Z80) {
	af := snapshotRegs(after)
	for i := 0; i < 7; i++ {
		if before[i] != af[i] {
			t.cur.Regs = append(t.cur.Regs, RegChange{registerNames[i], af[i]})
		}
	}
	t.cur.MW = after.MEMPTR

	if len(t.memWrites) > 0 {
		t.cur.Mem = append(t.cur.Mem, t.memWrites...)
	}
	if len(t.portWrites) > 0 {
		t.cur.Ports = append(t.cur.Ports, t.portWrites...)
	}

	t.records[t.head] = t.cur
	t.head = (t.head + 1) % len(t.records)
	if t.count < len(t.records) {
		t.count++
	}
}

// Get returns up to n most-recent records, oldest first.
func (t *Trace) Get(n int) []TraceRecord {
	if n <= 0 || n > t.count {
		n = t.count
	}
	out := make([]TraceRecord, 0, n)
	start := (t.head - n + len(t.records)) % len(t.records)
	for i := 0; i < n; i++ {
		out = append(out, t.records[(start+i)%len(t.records)])
	}
	return out
}

// snapshotRegs packs the key register pairs into a fixed array for comparison.
func snapshotRegs(c *cpu.Z80) [7]uint16 {
	return [7]uint16{
		uint16(c.A)<<8 | uint16(c.F), // AF
		uint16(c.B)<<8 | uint16(c.C), // BC
		uint16(c.D)<<8 | uint16(c.E), // DE
		uint16(c.H)<<8 | uint16(c.L), // HL
		c.SP,
		c.IX,
		c.IY,
	}
}

// SetTraceEnabled turns the emulator's delta trace on or off.
func (e *Emulator) SetTraceEnabled(on bool) {
	if e.trace != nil {
		e.trace.SetEnabled(on)
	}
}

// TraceGet returns up to n most-recent trace records.
func (e *Emulator) TraceGet(n int) []TraceRecord {
	if e.trace == nil {
		return nil
	}
	return e.trace.Get(n)
}
