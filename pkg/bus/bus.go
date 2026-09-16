// Package bus defines the interface between the Z80 CPU and the outside world.
// The CPU only sees BusReaderWriter -- never memory, ports, or peripherals directly.
package bus

// BusReaderWriter is the CPU's window to memory and I/O.
// This decoupling enables contention modelling, memory paging, and clean testing.
type BusReaderWriter interface {
	// Memory access
	ReadMemory(addr uint16) uint8
	FetchOpcode(addr uint16) uint8 // M1 cycle instruction fetch (TR-DOS support)
	WriteMemory(addr uint16, value uint8)

	// I/O port access
	ReadIO(port uint16) uint8
	WriteIO(port uint16, value uint8)
}

// MCycleKind identifies a Z80 machine cycle. The CPU reports them through
// MCycleObserver so a bus can time contention and the floating bus at the exact
// T-state of each access rather than once per instruction.
type MCycleKind uint8

const (
	MCycleM1       MCycleKind = iota // M1 opcode fetch, 4 T-states
	MCycleMemRead                    // memory read, 3 T-states
	MCycleMemWrite                   // memory write, 3 T-states
	MCycleIORead                     // I/O read, 4 T-states
	MCycleIOWrite                    // I/O write, 4 T-states
	// MCycleM1Refresh is the second half of an M1 cycle, where the Z80 drives
	// the refresh address (I in the high byte, R in the low) instead of the
	// fetch address. It is reported at the M1's own offset and adds no T-states
	// of its own - the 4 are already the M1's - but the ULA contends on it like
	// a memory access, which is the 48K "snow" effect: an I register inside the
	// contended range stalls every M1.
	MCycleM1Refresh
)

// MCycleObserver is implemented by a bus that wants to see each machine cycle
// before the CPU performs the access it describes. offset is the cycle's start
// relative to the start of the current instruction, in T-states; it carries the
// nominal cycle costs and so does not include contention.
//
// It is a capability interface, deliberately NOT part of BusReaderWriter: the
// General Sound card's second Z80 and the CPU test harness both implement that
// interface and must not be forced to care. A CPU talking to a bus that does not
// implement this simply gets the old instruction-granularity behaviour.
type MCycleObserver interface {
	// value is the byte the CPU is driving, for MCycleMemWrite and MCycleIOWrite
	// (the writes where the bus belongs to the CPU); it is 0 for every read.
	OnMCycle(kind MCycleKind, addr uint16, offset int, value uint8)
}
