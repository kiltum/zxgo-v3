package cpu

import "testing"

// A non-maskable interrupt is taken whatever IFF1 says: that is the whole point of it,
// and it is the one thing that distinguishes it from /INT (UI_DESIGN.md section 6.1).
func TestNMIIsTakenWithInterruptsDisabled(t *testing.T) {
	b := newTestBus48K()
	z := New(b)
	z.SP = 0xF000
	z.PC = 0x8000
	z.IFF1 = false
	z.IFF2 = false

	z.RequestNMI()
	ts := z.ExecuteOneInstruction()

	if ts != 11 {
		t.Errorf("NMI took %d T-states, want 11", ts)
	}
	if z.PC != 0x0066 {
		t.Errorf("PC = 0x%04X, want 0x0066", z.PC)
	}
	if z.SP != 0xEFFE {
		t.Errorf("SP = 0x%04X, want the return address pushed", z.SP)
	}
	if got := uint16(b.Mapper().ReadByte(0xEFFE)) | uint16(b.Mapper().ReadByte(0xEFFF))<<8; got != 0x8000 {
		t.Errorf("the pushed return address is 0x%04X, want 0x8000", got)
	}
	if z.IFF1 {
		t.Error("IFF1 is still set: the NMI did not disable interrupts")
	}
	if z.IFF2 {
		t.Error("IFF2 was set from a clear IFF1")
	}
}

// IFF2 takes IFF1's old value, which is what makes RETN safe: an NMI taken inside a
// critical section must not enable interrupts on the way out.
func TestNMIPreservesIFF2ForRETN(t *testing.T) {
	b := newTestBus48K()
	z := New(b)
	z.SP = 0xF000
	z.PC = 0x8000
	z.IFF1 = true
	z.IFF2 = true

	z.RequestNMI()
	z.ExecuteOneInstruction()

	if z.IFF1 {
		t.Error("IFF1 is still set after the NMI")
	}
	if !z.IFF2 {
		t.Error("IFF2 lost the value IFF1 had, so RETN would not restore it")
	}

	// And the return actually restores it.
	b.Mapper().WriteByte(0x0066, 0xED)
	b.Mapper().WriteByte(0x0067, 0x45) // RETN
	z.ExecuteOneInstruction()

	if z.PC != 0x8000 {
		t.Errorf("RETN returned to 0x%04X, want 0x8000", z.PC)
	}
	if !z.IFF1 {
		t.Error("RETN did not restore IFF1 from IFF2")
	}
}

// One request is one NMI: the latch clears when it is serviced, or a single press of
// the button would interrupt forever.
//
// The second instruction after the request is the point: 0x0066 holds a NOP, so a
// machine that had serviced the NMI again would take 11 T-states and land back at
// 0x0066, and one that had not would take the NOP's 4 and move on.
func TestNMIHappensOncePerRequest(t *testing.T) {
	b := newTestBus48K()
	z := New(b)
	z.SP = 0xF000
	z.PC = 0x8000

	z.RequestNMI()
	if ts := z.ExecuteOneInstruction(); ts != 11 {
		t.Fatalf("the first call took %d T-states, want the NMI's 11", ts)
	}
	if z.PC != 0x0066 {
		t.Fatalf("the NMI left PC at 0x%04X, want 0x0066", z.PC)
	}

	ts := z.ExecuteOneInstruction()
	if ts == 11 {
		t.Error("a second NMI was serviced for one request")
	}
	if z.PC != 0x0067 {
		t.Errorf("PC = 0x%04X after the instruction at 0x0066, want 0x0067 (it took %d T-states)",
			z.PC, ts)
	}
}

// The EI delay exists for /INT, whose effect is one instruction late. An NMI is not
// gated by it, and asking the latch during that window must still take it.
func TestNMIIsNotBlockedByTheEIDelay(t *testing.T) {
	b := newTestBus48K()
	z := New(b)
	z.SP = 0xF000
	z.PC = 0x8000
	z.IFF1 = true

	// EI, then a NOP, with the NMI requested during the delay.
	b.Mapper().WriteByte(0x8000, 0xFB) // EI
	z.ExecuteOneInstruction()          // sets the delay
	z.RequestNMI()
	ts := z.ExecuteOneInstruction()

	if z.PC != 0x0066 {
		t.Errorf("PC = 0x%04X after an NMI during the EI delay, want 0x0066 (%d T-states)", z.PC, ts)
	}
}

// A halted machine takes the NMI, and the address pushed is the one after the HALT:
// this core keeps PC *at* the HALT opcode while halted, so the service path is what
// moves it on. A wrong answer here is a RETN that halts again instead of resuming.
func TestNMITakesAHaltedMachineOnwards(t *testing.T) {
	b := newTestBus48K()
	z := New(b)
	z.SP = 0xF000
	z.PC = 0x8000
	z.HALT = true

	z.RequestNMI()
	z.ExecuteOneInstruction()

	if z.HALT {
		t.Error("the machine is still halted")
	}
	got := uint16(b.Mapper().ReadByte(0xEFFE)) | uint16(b.Mapper().ReadByte(0xEFFF))<<8
	if got != 0x8001 {
		t.Errorf("the pushed return address is 0x%04X, want the instruction after the HALT (0x8001)", got)
	}
}
