package emulator

import "testing"

// RunInstructions/RunTStates/RunUntil must advance totalTicks exactly by the
// amount reported, since they loop the same step() the interactive path uses.
func TestRunInstructionsAdvancesTicks(t *testing.T) {
	e := newTestEmu(t)

	before := e.totalTicks
	ticks := e.RunInstructions(1000)
	if ticks <= 0 {
		t.Fatalf("RunInstructions(1000) advanced %d ticks", ticks)
	}
	if e.totalTicks != before+int64(ticks) {
		t.Fatalf("totalTicks advanced by %d, want %d", e.totalTicks-before, ticks)
	}
	if e.CPU().PC == 0 {
		t.Error("PC is 0 after RunInstructions; CPU did not execute")
	}
}

func TestRunTStatesReachesTarget(t *testing.T) {
	e := newTestEmu(t)
	before := e.totalTicks
	ticks := e.RunTStates(10000)
	if ticks < 10000 {
		t.Fatalf("RunTStates(10000) executed %d ticks, want >= 10000", ticks)
	}
	if e.totalTicks != before+int64(ticks) {
		t.Fatalf("totalTicks advanced by %d, want %d", e.totalTicks-before, ticks)
	}
}

// RunUntil fires the moment the condition becomes true, after executing at
// least the instruction that moved PC off its reset value.
func TestRunUntilFiresAfterExecution(t *testing.T) {
	e := newTestEmu(t)
	start := e.CPU().PC

	hit, ticks := e.RunUntil(func() bool { return e.CPU().PC != start }, 1000000)
	if !hit {
		t.Fatal("RunUntil did not report the condition hit")
	}
	if ticks <= 0 {
		t.Fatal("RunUntil reported a hit for a PC change but executed no ticks")
	}
	if e.CPU().PC == start {
		t.Fatalf("PC = %04x, still at start %04x", e.CPU().PC, start)
	}
}

// The condition is checked before each instruction, so an already-true
// condition stops without executing anything.
func TestRunUntilAlreadyTrueStopsImmediately(t *testing.T) {
	e := newTestEmu(t)
	hit, ticks := e.RunUntil(func() bool { return true }, 1000000)
	if !hit {
		t.Fatal("RunUntil did not report the condition hit")
	}
	if ticks != 0 {
		t.Fatalf("RunUntil executed %d ticks for an already-true condition, want 0", ticks)
	}
}

// A never-true condition must exhaust maxInstructions and report the miss.
func TestRunUntilMaxInstructionsMiss(t *testing.T) {
	e := newTestEmu(t)
	hit, ticks := e.RunUntil(func() bool { return false }, 10)
	if hit {
		t.Fatal("RunUntil reported a hit for a never-true condition")
	}
	if ticks <= 0 {
		t.Fatalf("RunUntil with max=10 executed %d ticks; want > 0", ticks)
	}
}

// ReadPort/WritePort route through the real port bus: a write must not panic,
// and an unclaimed read must return the floating bus value 0xFF.
func TestReadWritePortSmoke(t *testing.T) {
	e := newTestEmu(t)
	e.WritePort(0x00FE, 0x02)
	if got := e.ReadPort(0x1234); got != 0xFF {
		t.Fatalf("unclaimed port read = 0x%02X, want 0xFF", got)
	}
}
