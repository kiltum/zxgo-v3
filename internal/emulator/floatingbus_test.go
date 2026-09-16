package emulator

import (
	"testing"

	"github.com/kiltum/zxgo-v3/pkg/bus"
	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/sound"
)

// A read of the ULA port (low byte 0xFE) must return the floating bus on the
// two bits the ULA does not drive: D5 and D7. Before this was implemented the
// port read returned them hard-high, so a program that samples the screen
// through 0xFE saw 0xFF instead of the fetched byte.
//
// The test blanks the screen so the fetched byte has D5 and D7 clear, then puts
// the ULA inside the display window and reads the port: the two bits must be
// clear, which is only possible if they came from the floating bus. Outside the
// window the floating bus is 0xFF, so the read is exactly what it always was -
// that half pins the no-regression direction.
func TestULAPortReturnsFloatingBus(t *testing.T) {
	e := New(model.Spectrum48K, &sound.NullOutput{})

	// Blank the screen so every fetch inside the window is 0x00.
	for addr := 0x4000; addr < 0x5B00; addr++ {
		e.Mapper().WriteByte(uint16(addr), 0x00)
	}

	// Advance the ULA into the display window. Only the ULA's own clock matters
	// here: the port read below does not involve the CPU.
	for i := 0; i < 200000 && e.ula.FloatingBus() != 0x00; i++ {
		e.ula.OneTick()
	}
	if got := e.ula.FloatingBus(); got != 0x00 {
		t.Fatalf("could not reach the display window: floating bus = 0x%02X", got)
	}

	got := e.ReadPort(0xFE)
	if got&0xA0 != 0 {
		t.Errorf("port 0xFE read = 0x%02X: D5/D7 should float (screen byte is 0x00)", got)
	}
	if got&0x1F != 0x1F {
		t.Errorf("port 0xFE read = 0x%02X: keyboard bits should still be driven high", got)
	}

	// Outside the window the floating bus is 0xFF, so those bits read high
	// again - the pre-existing behaviour, preserved.
	for i := 0; i < 200000 && e.ula.FloatingBus() != 0xFF; i++ {
		e.ula.OneTick()
	}
	if got := e.ula.FloatingBus(); got != 0xFF {
		t.Fatalf("could not reach the border: floating bus = 0x%02X", got)
	}
	if got := e.ReadPort(0xFE); got&0xA0 != 0xA0 {
		t.Errorf("port 0xFE read = 0x%02X in the border: D5/D7 should read high", got)
	}
}

// The 48K "snow" effect: in the second half of an M1 cycle the Z80 drives the
// refresh address, whose high byte is the I register. If I lands inside the
// contended range the ULA stalls that M1 like any contended memory access, which
// is why the effect appears only when a program sets I to 0x40-0x7F (the ROM
// keeps I at 0x3F, so it never snows).
//
// It is deliberately 48K-only: on the 128K the ROM's IM2 vector table is in RAM,
// so the same rule would stall every M1 and wreck the picture.
func TestM1RefreshContentionSnowEffect(t *testing.T) {
	e := New(model.Spectrum48K, &sound.NullOutput{})

	// Park the frame clock inside the first contended screen line.
	e.bus.UpdateFrameClock(14336 + 10)

	// An M1 fetch from uncontended RAM, followed by its refresh half.
	m1 := func(refreshAddr uint16) int {
		e.bus.SetInstructionTick(0)
		e.bus.OnMCycle(bus.MCycleM1, 0x8000, 0, 0)
		e.bus.OnMCycle(bus.MCycleM1Refresh, refreshAddr, 0, 0)
		return e.bus.FlushContention()
	}

	snow := m1(0x4000)  // I = 0x40: refresh address inside bank 5
	quiet := m1(0x3F00) // I = 0x3F: outside the contended range
	if snow <= 0 {
		t.Errorf("refresh address 0x4000 charged %d T-states of contention, want > 0", snow)
	}
	if quiet != 0 {
		t.Errorf("refresh address 0x3F00 charged %d T-states, want 0", quiet)
	}
}

// The 48K snow effect: a CPU write to contended memory puts its byte on the data
// bus, and a ULA screen fetch that happens in that window reads the CPU's byte
// instead of the screen's, which shows as grain. Modelled as the bus conflict it
// is, so it only fires when the two overlap in time - a write elsewhere in the
// instruction leaves the picture alone.
func TestSnowEffectSubstitutesOnBusConflict(t *testing.T) {
	const addr = 0x4000

	// fresh returns an emulator whose screen byte at addr is 0x11.
	fresh := func() *Emulator {
		e := New(model.Spectrum48K, &sound.NullOutput{})
		e.ula.SetSnowEffect(true) // off by default; -snow turns it on
		e.Mapper().WriteByte(addr, 0x11)
		return e
	}

	// A write still on the bus when the ULA fetches: the ULA reads the CPU's byte.
	e := fresh()
	e.ula.SnowWrite(e.ula.AbsoluteClock()-2, 0xE7)
	if got := e.ula.ScreenByteForTest(addr); got != 0xE7 {
		t.Errorf("overlapping fetch = 0x%02X, want the CPU's byte 0xE7", got)
	}

	// A write that finished before the fetch: memory wins.
	e = fresh()
	e.ula.SnowWrite(e.ula.AbsoluteClock()-100, 0xE7)
	if got := e.ula.ScreenByteForTest(addr); got != 0x11 {
		t.Errorf("finished write = 0x%02X, want the screen byte 0x11", got)
	}

	// A machine without the effect never substitutes, even mid-conflict.
	e = fresh()
	e.ula.SetSnowEffect(false)
	e.ula.SnowWrite(e.ula.AbsoluteClock()-1, 0xE7)
	if got := e.ula.ScreenByteForTest(addr); got != 0x11 {
		t.Errorf("with the effect off = 0x%02X, want the screen byte 0x11", got)
	}
}
