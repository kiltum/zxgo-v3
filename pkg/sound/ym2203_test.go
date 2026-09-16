package sound

import (
	"math"
	"testing"
)

// TestYM2203ProducesSound programs a simple note and verifies the FM core emits
// non-silence after key-on. This is a smoke test for the MAME fm.c port.
func TestYM2203ProducesSound(t *testing.T) {
	y := NewYM2203(1773450)

	// Algorithm 0, feedback 0.
	y.Write(0xFFFD, 0xB0)
	y.Write(0xBFFD, 0x00)
	// All four operators of channel 0: multiple=1, detune=0.
	for _, op := range []uint8{0x30, 0x34, 0x38, 0x3C} {
		y.Write(0xFFFD, op)
		y.Write(0xBFFD, 0x01)
	}
	// Total level = 0 (max volume).
	for _, op := range []uint8{0x40, 0x44, 0x48, 0x4C} {
		y.Write(0xFFFD, op)
		y.Write(0xBFFD, 0x00)
	}
	// Attack rate = 31 (fast), KSR = 0.
	for _, op := range []uint8{0x50, 0x54, 0x58, 0x5C} {
		y.Write(0xFFFD, op)
		y.Write(0xBFFD, 0x1F)
	}
	// fnum = 65 (block 0) -> ~440 Hz at 1.77345 MHz.
	y.Write(0xFFFD, 0xA0)
	y.Write(0xBFFD, 0x41)
	y.Write(0xFFFD, 0xA4)
	y.Write(0xBFFD, 0x00)
	// Key on channel 0, all four operators.
	y.Write(0xFFFD, 0x28)
	y.Write(0xBFFD, 0xF0)

	var maxV, minV int16 = 0, 0
	for i := int64(0); i < 4410; i++ {
		v := y.MeanLevel(i*80, (i+1)*80)
		if v > maxV {
			maxV = v
		}
		if v < minV {
			minV = v
		}
	}
	if maxV == 0 && minV == 0 {
		t.Fatal("YM2203 produced silence after key-on")
	}
	t.Logf("YM2203 output range: [%d, %d]", minV, maxV)
}

// TestTSFMSwitch verifies the pseudo-register chip-select scheme (%11111frc,
// bit 0 = c) and the TFM operator register layout (channel = bits 0-1,
// operator = bits 2-3).
func TestTSFMSwitch(t *testing.T) {
	ts := NewTSFM(1773450)

	// Select chip 1 (c=1), set channel 0 operator 0 multiple = 1 (mul = 2).
	ts.Write(0xFFFD, 0xF9)
	ts.Write(0xFFFD, 0x30)
	ts.Write(0xBFFD, 0x01)
	if got := ts.chip1.ch[0].slots[0].mul; got != 2 {
		t.Fatalf("chip1 ch0 op0 mul = %d, want 2", got)
	}

	// Register 0x31 = channel 2 (bits 0-1 = 1), operator 1 (bits 2-3 = 0): on
	// chip 0 it must set channel 1's operator 0, not channel 0's operator 1.
	ts.Write(0xFFFD, 0xF8) // chip 0 (c=0)
	ts.Write(0xFFFD, 0x31)
	ts.Write(0xBFFD, 0x0F) // multiple = 15 -> mul = 30
	if got := ts.chip0.ch[1].slots[0].mul; got != 30 {
		t.Fatalf("chip0 ch1 op0 mul = %d, want 30", got)
	}
	if got := ts.chip0.ch[0].slots[1].mul; got != 2 {
		t.Errorf("chip0 ch0 op1 mul = %d, want 2 (unaffected by 0x31)", got)
	}
	// Chip 1 unchanged.
	if got := ts.chip1.ch[0].slots[0].mul; got != 2 {
		t.Errorf("chip1 ch0 op0 mul changed to %d, want 2 (unchanged)", got)
	}
}

// TestTSFMRegisterReadBack verifies the two read ports: the data port (0xBFFD)
// returns the selected SSG register, and the status port (0xFFFD) returns the
// timer overflow flags with the busy flag on bit 7 in readiness mode.
func TestTSFMRegisterReadBack(t *testing.T) {
	ts := NewTSFM(1773450)

	// Chip 0, readiness OFF (0xFA = f=0 r=1 c=0).
	ts.Write(0xFFFD, 0xFA)
	ts.Write(0xFFFD, 0x00) // register R0 (tone A fine)
	ts.Write(0xBFFD, 0xFF) // data 0xFF
	if got := ts.Read(0xBFFD); got != 0xFF {
		t.Errorf("read R0 (data port) = %02X, want FF", got)
	}

	// Chip 0, readiness ON (0xF8 = f=0 r=0 c=0): status port bit 7 is the busy
	// flag. The write above left the chip busy, so bit 7 reads 1 (0x80) until the
	// tick clock advances past the 32*6 master-cycle busy window.
	ts.Write(0xFFFD, 0xF8)
	if got := ts.Read(0xFFFD); got != 0x80 {
		t.Errorf("read status (readiness ON, busy) = %02X, want 80", got)
	}
	ts.SetTick(1000)
	if got := ts.Read(0xFFFD); got != 0x00 {
		t.Errorf("read status (readiness ON, ready) = %02X, want 00", got)
	}
}

// TestYM2203Prescaler verifies the 0x2D/0x2E/0x2F clock prescaler registers.
// They change the FM clock divisor (72 -> 36 -> 24), which scales the phase step
// of every channel's operators.
func TestYM2203Prescaler(t *testing.T) {
	y := NewYM2203(1773450)

	// A fixed fnum so the phase step is well defined: fnum = 256, block 0
	// (A0 = low 8 bits, A4 = high 3 bits + block). Default MUL = 1 (mul = 2).
	y.Write(0xFFFD, 0xA0)
	y.Write(0xBFFD, 0x00)
	y.Write(0xFFFD, 0xA4)
	y.Write(0xBFFD, 0x01)

	if y.prescalerSel != 2 {
		t.Fatalf("reset prescalerSel = %d, want 2", y.prescalerSel)
	}
	incr72 := y.ch[0].slots[0].incr

	// 0x2F clears the selector -> divisor 24 (3x the default step).
	y.Write(0xFFFD, 0x2F)
	y.Write(0xBFFD, 0x00)
	if y.prescalerSel != 0 {
		t.Fatalf("after 0x2F prescalerSel = %d, want 0", y.prescalerSel)
	}
	if got := y.ch[0].slots[0].incr; got < incr72*3-2 || got > incr72*3+2 {
		t.Errorf("after 0x2F incr = %d, want ~%d", got, incr72*3)
	}

	// 0x2D sets bit 1: 0 -> 2 -> divisor 72 (back to the default step).
	y.Write(0xFFFD, 0x2D)
	y.Write(0xBFFD, 0x00)
	if y.prescalerSel != 2 {
		t.Fatalf("after 0x2D prescalerSel = %d, want 2", y.prescalerSel)
	}
	if got := y.ch[0].slots[0].incr; got != incr72 {
		t.Errorf("after 0x2D incr = %d, want %d", got, incr72)
	}

	// 0x2E sets bit 0: 2 -> 3 -> divisor 36 (2x the default step).
	y.Write(0xFFFD, 0x2E)
	y.Write(0xBFFD, 0x00)
	if y.prescalerSel != 3 {
		t.Fatalf("after 0x2E prescalerSel = %d, want 3", y.prescalerSel)
	}
	if got := y.ch[0].slots[0].incr; got < incr72*2-2 || got > incr72*2+2 {
		t.Errorf("after 0x2E incr = %d, want ~%d", got, incr72*2)
	}
}

// TestYM2203Pitch verifies the frequency formula against the tfm-prg.pdf note
// table: fnum 1037 (0x40D) at block 4 is A4 = 440 Hz for a 4 MHz chip clock.
func TestYM2203Pitch(t *testing.T) {
	y := NewYM2203(4000000)

	// fnum = 1037: A0 = low 8 bits (0x0D), A4 = block 4 + high 3 bits (0x24).
	y.Write(0xFFFD, 0xA0)
	y.Write(0xBFFD, 0x0D)
	y.Write(0xFFFD, 0xA4)
	y.Write(0xBFFD, 0x24)

	// The phase step is 16.16 with a 1024-entry sine table, so one full wave is
	// 2^26 phase units: frequency = incr * sampleRate / 2^26.
	freq := float64(y.ch[0].slots[0].incr) * float64(y.sampleRate) / float64(uint64(1)<<26)
	if math.Abs(freq-440.0) > 2.0 {
		t.Errorf("fnum=1037 block=4 -> %.1f Hz, want ~440 Hz", freq)
	}
}

// TestYM2203KeyOff verifies that key-on enters attack and key-off (bits cleared
// in the 0x28 mask) moves an attack/decay/sustain operator into release.
func TestYM2203KeyOff(t *testing.T) {
	y := NewYM2203(4000000)

	// Key on channel 0, all four operators.
	y.Write(0xFFFD, 0x28)
	y.Write(0xBFFD, 0xF0)
	for op := 0; op < 4; op++ {
		if got := y.ch[0].slots[op].state; got != ymEgAtt {
			t.Fatalf("op %d state after key-on = %d, want attack (%d)", op, got, ymEgAtt)
		}
	}

	// Key off channel 0: the cleared operator bits now release.
	y.Write(0xFFFD, 0x28)
	y.Write(0xBFFD, 0x00)
	for op := 0; op < 4; op++ {
		if got := y.ch[0].slots[op].state; got != ymEgRel {
			t.Fatalf("op %d state after key-off = %d, want release (%d)", op, got, ymEgRel)
		}
		if y.ch[0].slots[op].key {
			t.Fatalf("op %d still keyed after key-off", op)
		}
	}
}

// TestYM2203Ch3Mode verifies the 3-slot mode: with 0x27 bit 6 set, channel 3's
// operators take independent frequencies from 0xA8-0xAE instead of the shared
// channel frequency.
func TestYM2203Ch3Mode(t *testing.T) {
	y := NewYM2203(4000000)

	// Give channel 3 a shared frequency of fnum 256, block 0.
	y.Write(0xFFFD, 0xA2)
	y.Write(0xBFFD, 0x00)
	y.Write(0xFFFD, 0xA6)
	y.Write(0xBFFD, 0x01)
	shared := y.ch[2].slots[0].incr

	// Independent frequency for slot 0 (register 0xA9/0xAD): fnum 512, block 1.
	y.Write(0xFFFD, 0xA9)
	y.Write(0xBFFD, 0x00)
	y.Write(0xFFFD, 0xAD)
	y.Write(0xBFFD, 0x09) // block 1, fnum high 3 = 1

	// Before 3-slot mode is enabled, the write above does not affect the shared
	// channel frequency.
	if got := y.ch[2].slots[0].incr; got != shared {
		t.Fatalf("ch3 op0 incr changed to %d before 3-slot mode, want %d", got, shared)
	}

	// Enable 3-slot mode.
	y.Write(0xFFFD, 0x27)
	y.Write(0xBFFD, 0x40)

	// op0 now uses ch3fc[1] (fnum 512 block 1), which is a higher pitch than the
	// shared fnum 256 block 0.
	if got := y.ch[2].slots[0].incr; got <= shared {
		t.Fatalf("ch3 op0 incr = %d in 3-slot mode, want > shared %d", got, shared)
	}
}

// TestYM2203Timer verifies the timers: loading a short Timer A and generating
// FM samples sets the overflow flag, which the 0x27 reset bit then clears.
func TestYM2203Timer(t *testing.T) {
	y := NewYM2203(4000000)

	// Timer A = 1023 -> period 1 FM-clock tick (~18 us at 4 MHz).
	y.Write(0xFFFD, 0x24)
	y.Write(0xBFFD, 0xFF)
	y.Write(0xFFFD, 0x25)
	y.Write(0xBFFD, 0x03)
	if y.ta != 1023 {
		t.Fatalf("ta = %d, want 1023", y.ta)
	}

	// Enable timer A: load (0x01) + enable (0x04).
	y.Write(0xFFFD, 0x27)
	y.Write(0xBFFD, 0x05)

	for i := 0; i < 100; i++ {
		y.generateOne()
	}
	if y.timerFlags&0x01 == 0 {
		t.Fatalf("timer A overflow flag not set after generating samples")
	}

	// Reset the flag (bit 4), keeping load+enable set.
	y.Write(0xFFFD, 0x27)
	y.Write(0xBFFD, 0x15) // reset A (0x10) + enable A (0x04) + load A (0x01)
	if y.timerFlags&0x01 != 0 {
		t.Fatalf("timer A flag not cleared by 0x27 reset")
	}
}
