package cpu

import (
	"bufio"
	"fmt"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/kiltum/zxgo-v3/pkg/bus"
	"github.com/kiltum/zxgo-v3/pkg/io_ports"
	"github.com/kiltum/zxgo-v3/pkg/mem"
	"github.com/kiltum/zxgo-v3/pkg/model"
)

// fusePortHandler returns high byte of port for IN instructions (Fuse test convention).
type fusePortHandler struct{}

func (h *fusePortHandler) HandlesPort(port uint16) bool   { return true }
func (h *fusePortHandler) Read(port uint16) uint8         { return uint8(port >> 8) }
func (h *fusePortHandler) Write(port uint16, value uint8) {}

var _ io_ports.PortHandler = (*fusePortHandler)(nil)

func newTestBus48K() *bus.DefaultBus {
	cfg := model.Config{Name: "test-48K"}
	mapper := mem.NewFlat48K(cfg)
	mapper.SetROMWritable(true)
	portBus := io_ports.NewPortBus()
	portBus.Register(&fusePortHandler{})
	return bus.NewDefaultBus(mapper, portBus)
}

// --- Fuse test suite ---

// expectedCycleEvent represents one bus cycle event from tests.expected.
type expectedCycleEvent struct {
	Time    int    // T-state within the case (Fuse's first field)
	Type    string // "MR", "MW", "PR", "PW", "MC", "PC"
	Address uint16
	Data    uint8 // zero for MC/PC (no data field)
}

// Per-access timing against Fuse's records. Fuse stamps every access with the
// T-state it completes at; the per-machine-cycle hook places each access at its
// nominal cycle offset (M1 4T, memory 3T, I/O 4T), which is exact for 675 of the
// 1356 cases. The rest are two known residuals, both measured rather than
// papered over:
//
//   - 512 DDCB/FDCB cases. Those instructions do not use nominal cycles: Fuse's
//     own records show 4,4,3,3,5,4 across the prefix, displacement, opcode,
//     read and write, where we report 4,4,3,4,3,3. The instruction's total is
//     correct (23T); only the placement of the accesses inside it is off.
//   - 169 cases with internal T-states between two accesses, e.g. INC (HL) is
//     4+3+1+3, so our write lands a T-state early. Fuse shows the internal cycle
//     as an extra MC line we do not emit.
//
// fuseAccessTimesMatch counts cases whose access times all matched. The floor in
// TestFuseData_CMOS is a ratchet, not a target: it catches a systematic cycle
// offset regression (an M1 counted as 3T, say) across thousands of cases
// without pretending the residual is closed. Raise it when either residual is
// closed - the Fuse records say exactly what to aim for.
var (
	fuseAccessTimesMatch int
	fuseAccessTimeCases  int
)

type expectedResult struct {
	testName    string
	cycleEvents []expectedCycleEvent // parsed bus cycle events (all types)
	registers   []string
	flags       []string
	memoryLines []string
}

func TestFuseData_CMOS(t *testing.T) {
	runFuseTests(t, false)
	reportFuseAccessTimes(t)
}

// fuseAccessTimeFloor is the measured number of cases whose access times match
// Fuse's records exactly. It is a ratchet: the number must not drop, which is
// what catches a systematic cycle-offset regression across the whole corpus.
// Raise it when the internal-cycle residual is closed.
const fuseAccessTimeFloor = 675

func reportFuseAccessTimes(t *testing.T) {
	if fuseAccessTimeCases == 0 {
		t.Fatal("no Fuse cases ran: the timing oracle measured nothing")
	}
	t.Logf("per-access timing: %d/%d cases match Fuse's T-states exactly (%.1f%%)",
		fuseAccessTimesMatch, fuseAccessTimeCases,
		100*float64(fuseAccessTimesMatch)/float64(fuseAccessTimeCases))

	if fuseAccessTimesMatch < fuseAccessTimeFloor {
		t.Errorf("per-access timing regressed: %d cases match, floor is %d",
			fuseAccessTimesMatch, fuseAccessTimeFloor)
	}
}

func runFuseTests(t *testing.T, isNMOS bool) {
	expectedResults, err := parseExpectedResults("testdata/tests.expected")
	if err != nil {
		t.Fatalf("parse expected results: %v", err)
	}

	f, err := os.Open("testdata/tests.in")
	if err != nil {
		t.Fatalf("open tests.in: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var name string
	var regs, flags, memLines []string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			if name != "" && len(regs) > 0 {
				exp, found := expectedResults[name]
				if !found {
					t.Errorf("no expected result for %s", name)
				}
				runFuseTest(t, name, regs, flags, memLines, exp, isNMOS)
			}
			name, regs, flags, memLines = "", nil, nil, nil
			continue
		}
		if name == "" {
			name = line
			continue
		}
		if len(regs) == 0 {
			regs = strings.Fields(line)
			continue
		}
		if len(flags) == 0 {
			flags = strings.Fields(line)
			continue
		}
		if line == "-1" {
			continue
		}
		memLines = append(memLines, line)
	}
	if name != "" && len(regs) > 0 {
		exp, found := expectedResults[name]
		if !found {
			t.Errorf("no expected result for %s", name)
		}
		runFuseTest(t, name, regs, flags, memLines, exp, isNMOS)
	}
}

// parseExpectedResults reads tests.expected, parsing bus cycle events (MC/MR/MW/PC/PR/PW),
// then register/flags/memory lines as before.
func parseExpectedResults(filename string) (map[string]expectedResult, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	results := make(map[string]expectedResult)
	scanner := bufio.NewScanner(f)
	var name string
	var cur expectedResult
	state := 0
	// state encoding:
	// 0: expect test name
	// 1: cycle events OR registers line (13 fields)
	// 2: not used (registers already consumed)
	// 3: expect flags line (6+ fields)
	// 4: expect memory lines or empty

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			if name != "" && len(cur.registers) > 0 && len(cur.flags) > 0 {
				cur.testName = name
				results[name] = cur
			}
			name = ""
			cur = expectedResult{}
			state = 0
			continue
		}
		if state == 0 {
			name = line
			state = 1
			continue
		}
		if state == 1 {
			// Cycle events: "<time> <type> <address> [data]"
			// Discriminate from registers: < 6 fields (event) vs >= 13 fields (registers).
			// Register lines have exactly 13 hex values.
			// Flag lines have 6-7 fields: I R IFF1 IFF2 IM halted [tstates]
			// Memory lines: hex address, hex bytes..., -1
			fields := strings.Fields(line)
			if (len(fields) == 3 || len(fields) == 4) &&
				(fields[1] == "MC" || fields[1] == "MR" || fields[1] == "MW" ||
					fields[1] == "PC" || fields[1] == "PR" || fields[1] == "PW") {
				// Parse cycle event: time type addr [data]
				addr, _ := strconv.ParseUint(fields[2], 16, 16)
				ts, _ := strconv.Atoi(fields[0])
				ev := expectedCycleEvent{
					Time:    ts,
					Type:    fields[1],
					Address: uint16(addr),
				}
				if len(fields) == 4 {
					d, _ := strconv.ParseUint(fields[3], 16, 8)
					ev.Data = uint8(d)
				}
				cur.cycleEvents = append(cur.cycleEvents, ev)
				continue
			}
			if len(fields) >= 13 {
				cur.registers = fields
				state = 3
				continue
			}
			// Could also be flags line directly (no cycle events for this test), but
			// that would mean state 3 or a fall-through. Handle that below.
			// Fall through to flags check explicitly.
		}
		if state == 1 || state == 3 {
			fields := strings.Fields(line)
			if len(fields) >= 6 && len(fields) <= 7 {
				// Distinguish flags from memory lines: memory lines have an address
				// (value >= 0x100) as first field, flags have small values like I/R.
				// Or: flags[0] is always 1-2 hex digits (I register), flags[1] is 2 hex digits (R).
				// Memory lines: first field is a 4-digit hex address.
				if len(fields[0]) <= 2 && len(fields[1]) <= 2 {
					cur.flags = fields
					state = 4
					continue
				}
			}
			state = 4 // fall through to memory
		}
		if state == 4 && line != "-1" {
			cur.memoryLines = append(cur.memoryLines, line)
		}
	}
	if name != "" && len(cur.registers) > 0 && len(cur.flags) > 0 {
		cur.testName = name
		results[name] = cur
	}
	return results, nil
}

// recordingBus wraps a real DefaultBus and captures every bus operation.
// It delegates all real work to the inner bus so instructions execute correctly.
type recordingBus struct {
	inner *bus.DefaultBus

	// instrStart is the absolute T-state the instruction being executed began
	// at; cycleEnd is where the machine cycle last reported by OnMCycle
	// completes. Together they stamp every access with a time comparable to
	// Fuse's records.
	instrStart int
	cycleEnd   int

	// Captured events: MR/MW/PR/PW (mem reads, mem writes, port reads, port
	// writes). MC/PC (contend) events are not compared - Fuse emits them for
	// internal no-MREQ cycles too, which our hook does not report.
	captured []capturedEvent
}

// OnMCycle implements bus.MCycleObserver: the CPU reports each machine cycle
// before the access, and this turns the cycle's start offset into the T-state
// the access completes at.
func (r *recordingBus) OnMCycle(kind bus.MCycleKind, addr uint16, offset int, value uint8) {
	r.cycleEnd = r.instrStart + offset + cycleTStates(kind)
}

type capturedEvent struct {
	Time    int    // absolute T-state at which the access completes
	Type    string // "MR", "MW", "PR", "PW"
	Address uint16
	Data    uint8
}

// cycleTStates is the nominal length of each machine cycle, used to turn the
// hook's cycle-start offset into the T-state the access completes at (which is
// the instant Fuse's records stamp).
func cycleTStates(kind bus.MCycleKind) int {
	switch kind {
	case bus.MCycleM1:
		return 4
	case bus.MCycleIORead, bus.MCycleIOWrite:
		return 4
	case bus.MCycleM1Refresh:
		return 0 // shares the M1's T-states; no access of its own
	default:
		return 3
	}
}

// recordingReset zeroes the capture slice -- call before each instruction.
func (r *recordingBus) recordingReset() {
	r.captured = r.captured[:0]
}

// --- BusReaderWriter implementation (delegates + records) ---

func (r *recordingBus) ReadMemory(addr uint16) uint8 {
	v := r.inner.ReadMemory(addr)
	r.captured = append(r.captured, capturedEvent{r.cycleEnd, "MR", addr, v})
	return v
}

func (r *recordingBus) FetchOpcode(addr uint16) uint8 {
	v := r.inner.FetchOpcode(addr)
	// Recorded as MR -- Fuse does not distinguish M1 fetches from regular reads.
	r.captured = append(r.captured, capturedEvent{r.cycleEnd, "MR", addr, v})
	return v
}

func (r *recordingBus) WriteMemory(addr uint16, value uint8) {
	r.inner.WriteMemory(addr, value)
	r.captured = append(r.captured, capturedEvent{r.cycleEnd, "MW", addr, value})
}

func (r *recordingBus) ReadIO(port uint16) uint8 {
	v := r.inner.ReadIO(port)
	r.captured = append(r.captured, capturedEvent{r.cycleEnd, "PR", port, v})
	return v
}

func (r *recordingBus) WriteIO(port uint16, value uint8) {
	r.inner.WriteIO(port, value)
	r.captured = append(r.captured, capturedEvent{r.cycleEnd, "PW", port, value})
}

var _ bus.BusReaderWriter = (*recordingBus)(nil)

// compareFuseCycleEvents filters the expected events to MR/MW/PR/PW only
// (skipping MC/PC contend events), then compares the sequence against the
// captured events.
//
// MW events are normalised because write order for multi-byte operations
// (PUSH, CALL, RST, EX (SP)) differs between implementations: Fuse's PUSH
// macro writes the high byte first, while our push writes the low byte first.
// Both orders are functionally identical; the test sorts adjacent MW events by
// address before comparing so that either ordering is accepted.
func compareFuseCycleEvents(t *testing.T, name string, expected []expectedCycleEvent, captured []capturedEvent) {
	t.Helper()

	// Build expected list: only events with a data byte (MR/MW/PR/PW).
	// MC and PC have no data; they cannot be compared without per-M-cycle timing.
	var expectedData []expectedCycleEvent
	for _, ev := range expected {
		if ev.Type == "MR" || ev.Type == "MW" || ev.Type == "PR" || ev.Type == "PW" {
			expectedData = append(expectedData, ev)
		}
	}

	// Normalise adjacent MW groups by sorting them by address.
	expectedData = normaliseMWEvents(expectedData)
	// Normalise captured MW groups similarly.
	var cap []expectedCycleEvent
	for _, ev := range captured {
		cap = append(cap, expectedCycleEvent{Time: ev.Time, Type: ev.Type, Address: ev.Address, Data: ev.Data})
	}
	cap = normaliseMWEvents(cap)

	if len(cap) != len(expectedData) {
		t.Errorf("bus event count: expected %d (excluding MC/PC), got %d",
			len(expectedData), len(cap))
		var log strings.Builder
		log.WriteString("expected data events:\n")
		for _, ev := range expectedData {
			fmt.Fprintf(&log, "  %s %04x %02x\n", ev.Type, ev.Address, ev.Data)
		}
		log.WriteString("captured events:\n")
		for _, ev := range cap {
			fmt.Fprintf(&log, "  %s %04x %02x\n", ev.Type, ev.Address, ev.Data)
		}
		t.Log(log.String())
		return
	}

	// Per-access timing: every access that matches in type/address/data must
	// also complete at Fuse's T-state. Mismatches here are the known residual
	// (internal T-states between two accesses shift the later one), so they are
	// counted rather than failed - the ratchet in TestFuseData_CMOS is the gate.
	// Compared as a multiset: Fuse lists the two halves of a 16-bit write in
	// the opposite order from us, so an order-sensitive comparison would report
	// a false mismatch on every PUSH, CALL and LD (nn),rr.
	expKeys := make([]string, 0, len(expectedData))
	for _, e := range expectedData {
		expKeys = append(expKeys, fmt.Sprintf("%s %04x %02x %d", e.Type, e.Address, e.Data, e.Time))
	}
	capKeys := make([]string, 0, len(cap))
	for _, c := range cap {
		capKeys = append(capKeys, fmt.Sprintf("%s %04x %02x %d", c.Type, c.Address, c.Data, c.Time))
	}
	sort.Strings(expKeys)
	sort.Strings(capKeys)
	fuseAccessTimeCases++
	if slices.Equal(expKeys, capKeys) {
		fuseAccessTimesMatch++
	}

	for i := range expectedData {
		ee := expectedData[i]
		ce := cap[i]
		if ee.Type != ce.Type || ee.Address != ce.Address || ee.Data != ce.Data {
			t.Errorf("event %d: expected %s %04x %02x, got %s %04x %02x",
				i, ee.Type, ee.Address, ee.Data, ce.Type, ce.Address, ce.Data)
			ctx := max(0, i-2)
			endIdx := min(len(expectedData), i+3)
			var log strings.Builder
			log.WriteString("expected (near mismatch):\n")
			for j := ctx; j < endIdx; j++ {
				marker := " "
				if j == i {
					marker = "*"
				}
				e := expectedData[j]
				fmt.Fprintf(&log, " %s %d: %s %04x %02x\n", marker, j, e.Type, e.Address, e.Data)
			}
			endIdx2 := min(len(cap), i+3)
			log.WriteString("captured (near mismatch):\n")
			for j := ctx; j < endIdx2; j++ {
				marker := " "
				if j == i {
					marker = "*"
				}
				c := cap[j]
				fmt.Fprintf(&log, " %s %d: %s %04x %02x\n", marker, j, c.Type, c.Address, c.Data)
			}
			t.Log(log.String())
			break
		}
	}
}

// normaliseMWEvents sorts adjacent groups of MW events by address within the
// event stream.  Only MW events participate; other event types remain
// in their original positions.  Adjacent MW events that write to consecutive
// addresses (PUSH, CALL, RST, EX (SP)) will be ordered low-address-first
// regardless of the order in which the write ops were issued.
func normaliseMWEvents(events []expectedCycleEvent) []expectedCycleEvent {
	var result []expectedCycleEvent
	i := 0
	for i < len(events) {
		if events[i].Type == "MW" {
			// Collect a contiguous run of MW events.
			run := []expectedCycleEvent{events[i]}
			j := i + 1
			for j < len(events) && events[j].Type == "MW" {
				run = append(run, events[j])
				j++
			}
			// Sort this run by Address so ordering differences for PUSH-type
			// writes don't trigger false failures.
			sortCycleEventsByAddr(run)
			result = append(result, run...)
			i = j
		} else {
			result = append(result, events[i])
			i++
		}
	}
	return result
}

func sortCycleEventsByAddr(evs []expectedCycleEvent) {
	// Simple insertion sort since runs are very short (1-3 events).
	for i := 1; i < len(evs); i++ {
		key := evs[i]
		j := i - 1
		for j >= 0 && evs[j].Address > key.Address {
			evs[j+1] = evs[j]
			j--
		}
		evs[j+1] = key
	}
}

func runFuseTest(t *testing.T, name string, regs, flags, memLines []string, exp expectedResult, isNMOS bool) {
	t.Run(name, func(t *testing.T) {
		realBus := newTestBus48K()
		recBus := &recordingBus{inner: realBus}

		cpu := New(recBus)
		cpu.SetCPUType(isNMOS)
		// Fuse's data records the documented MEMPTR, not the measured one; the
		// two differ only on the repeat path of the block I/O instructions.
		cpu.SetMEMPTRReal(false)

		parse16 := func(s string) uint16 { v, _ := strconv.ParseUint(s, 16, 16); return uint16(v) }
		parse8 := func(s string) uint8 { v, _ := strconv.ParseUint(s, 16, 8); return uint8(v) }
		parseBool := func(s string) bool { v, _ := strconv.Atoi(s); return v != 0 }

		if len(regs) >= 13 {
			cpu.setAF(parse16(regs[0]))
			cpu.setBC(parse16(regs[1]))
			cpu.setDE(parse16(regs[2]))
			cpu.setHL(parse16(regs[3]))
			cpu.setAF_(parse16(regs[4]))
			cpu.setBC_(parse16(regs[5]))
			cpu.setDE_(parse16(regs[6]))
			cpu.setHL_(parse16(regs[7]))
			cpu.IX = parse16(regs[8])
			cpu.IY = parse16(regs[9])
			cpu.SP = parse16(regs[10])
			cpu.PC = parse16(regs[11])
			cpu.MEMPTR = parse16(regs[12])
		}

		var desiredTStates int
		if len(flags) >= 6 {
			cpu.I = parse8(flags[0])
			cpu.R = parse8(flags[1])
			cpu.IFF1 = parseBool(flags[2])
			cpu.IFF2 = parseBool(flags[3])
			v, _ := strconv.Atoi(flags[4])
			cpu.IM = uint8(v)
			cpu.HALT = parseBool(flags[5])
		}
		if len(flags) >= 7 {
			desiredTStates, _ = strconv.Atoi(flags[6])
		}

		for _, ml := range memLines {
			if ml == "-1" {
				break
			}
			parts := strings.Fields(ml)
			if len(parts) < 2 {
				continue
			}
			addr := parse16(parts[0])
			for i := 1; i < len(parts) && parts[i] != "-1"; i++ {
				realBus.Mapper().WriteByte(addr, parse8(parts[i]))
				addr++
			}
		}

		total := 0
		for desiredTStates > 0 && total < desiredTStates {
			recBus.instrStart = total
			ts := cpu.ExecuteOneInstruction()
			if ts <= 0 {
				t.Errorf("instruction returned %d T-states", ts)
				break
			}
			total += ts
		}

		// Compare accumulated bus events against expected cycle events (once, after all instructions).
		// The expected cycle events cover the entire T-state run (may span multiple instructions).
		compareFuseCycleEvents(t, name, exp.cycleEvents, recBus.captured)

		if len(exp.registers) > 0 {
			check := func(label, hex string, actual uint16) {
				if v, err := strconv.ParseUint(hex, 16, 16); err == nil && actual != uint16(v) {
					t.Errorf("%s mismatch: expected %s, got %04x", label, hex, actual)
				}
			}
			check("AF", exp.registers[0], cpu.getAF())
			check("BC", exp.registers[1], cpu.getBC())
			check("DE", exp.registers[2], cpu.getDE())
			check("HL", exp.registers[3], cpu.getHL())
			check("AF'", exp.registers[4], cpu.getAF_())
			check("BC'", exp.registers[5], cpu.getBC_())
			check("DE'", exp.registers[6], cpu.getDE_())
			check("HL'", exp.registers[7], cpu.getHL_())
			check("IX", exp.registers[8], cpu.IX)
			check("IY", exp.registers[9], cpu.IY)
			check("SP", exp.registers[10], cpu.SP)
			check("PC", exp.registers[11], cpu.PC)
			check("MEMPTR", exp.registers[12], cpu.MEMPTR)
		}
		if len(exp.flags) >= 6 {
			if v, err := strconv.ParseUint(exp.flags[0], 16, 8); err == nil && cpu.I != uint8(v) {
				t.Errorf("I mismatch: expected %s, got %02x", exp.flags[0], cpu.I)
			}
			if v, err := strconv.ParseUint(exp.flags[1], 16, 8); err == nil && cpu.R != uint8(v) {
				t.Errorf("R mismatch: expected %s, got %02x", exp.flags[1], cpu.R)
			}
			if v, err := strconv.Atoi(exp.flags[2]); err == nil && (cpu.IFF1 != (v != 0)) {
				t.Errorf("IFF2 mismatch: expected %d, got %v", v, cpu.IFF2)
			}
			if v, err := strconv.Atoi(exp.flags[3]); err == nil && (cpu.IFF2 != (v != 0)) {
				t.Errorf("IFF2 mismatch: expected %d, got %v", v, cpu.IFF2)
			}
		}

		// Final T-state count assertion (Step 4 of the plan).
		if len(exp.flags) >= 7 {
			expTStates, err := strconv.Atoi(exp.flags[6])
			if err == nil && total != expTStates {
				t.Errorf("T-states: expected %d, got %d", expTStates, total)
			}
		}

		for _, ml := range exp.memoryLines {
			if ml == "-1" {
				break
			}
			parts := strings.Fields(ml)
			if len(parts) < 3 {
				continue
			}
			addr := parse16(parts[0])
			for i := 1; i < len(parts)-1 && parts[i] != "-1"; i++ {
				exp := parse8(parts[i])
				got := realBus.Mapper().ReadByte(addr)
				if uint64(got) != uint64(exp) {
					t.Errorf("mem %04x: expected %02x, got %02x", addr, exp, got)
				}
				addr++
			}
		}
	})
}

// --- ZEXALL ---

func TestZEXALL(t *testing.T) {
	zexFile := "testdata/zexall.com"
	if _, err := os.Stat(zexFile); err != nil {
		t.Skipf("ZEXALL file %s not found", zexFile)
	}

	testBus := newTestBus48K()
	cpu := New(testBus)
	cpu.SetCPUType(true) // NMOS -- ZEXALL expects NMOS, not CMOS like Fuse

	data, err := os.ReadFile(zexFile)
	if err != nil {
		t.Fatalf("read zexall: %v", err)
	}
	for i, b := range data {
		testBus.Mapper().WriteByte(uint16(0x100+i), b)
	}
	testBus.Mapper().WriteByte(0x0005, 0xC9) // RET at BDOS entry

	cpu.PC = 0x0100
	cpu.SP = 0xF000

	t.Log("ZEXALL starting...")
	instrs := 0
	var out strings.Builder

loop:
	for {
		if cpu.PC == 0x0005 {
			retAddr := cpu.pop()
			switch cpu.C {
			case 0:
				t.Logf("ZEXALL exit at %d instructions", instrs)
				break loop
			case 2:
				out.WriteByte(cpu.E)
			case 9:
				addr := cpu.getDE()
				for {
					b := testBus.Mapper().ReadByte(addr)
					if b == '$' {
						break
					}
					out.WriteByte(b)
					addr++
				}
			}
			cpu.PC = retAddr
			continue
		}
		if cpu.PC == 0 {
			t.Logf("ZEXALL PC=0 exit at %d instructions", instrs)
			break
		}
		cpu.ExecuteOneInstruction()
		instrs++
	}

	result := out.String()
	t.Logf("ZEXALL output (%d instr):\n%s", instrs, result)

	if strings.Contains(result, "FAIL") || strings.Contains(result, "ERROR") {
		t.Errorf("ZEXALL reported failures")
	}
}
