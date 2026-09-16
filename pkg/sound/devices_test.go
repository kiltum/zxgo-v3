package sound

import "testing"

// TestCovox verifies the Covox decodes its port and maps the 8-bit value to the
// DAC level (value * covoxScale, matching Fuse).
func TestCovox(t *testing.T) {
	c := NewCovox(0xFB)
	if !c.HandlesPort(0xFB) {
		t.Error("Covox should handle 0xFB")
	}
	if c.HandlesPort(0xFC) {
		t.Error("Covox should not handle 0xFC")
	}
	c.SetTick(0)
	c.Write(0xFB, 0x80) // 128 -> 128*128 = 16384
	if got := c.MeanLevel(0, 1000); got != 16384 {
		t.Errorf("Covox MeanLevel = %d, want 16384", got)
	}
}

// TestTurboSoundSwitch verifies the NedoPC chip-select scheme: writing 0xFF to
// 0xFFFD selects chip 0, 0xFE selects chip 1, and register/data writes then go
// to the selected chip only.
func TestTurboSoundSwitch(t *testing.T) {
	ts := NewTurboSound(1773450)

	ts.Write(0xFFFD, 0xFF) // chip 0
	ts.Write(0xFFFD, 0x08) // register 8
	ts.Write(0xBFFD, 0x0F) // data
	if ts.ay1.regs[8] != 0x0F {
		t.Fatalf("chip 0 reg 8 = %d, want 15", ts.ay1.regs[8])
	}

	ts.Write(0xFFFD, 0xFE) // chip 1
	ts.Write(0xFFFD, 0x08) // register 8
	ts.Write(0xBFFD, 0x07) // data
	if ts.ay2.regs[8] != 0x07 {
		t.Fatalf("chip 1 reg 8 = %d, want 7", ts.ay2.regs[8])
	}
	if ts.ay1.regs[8] != 0x0F {
		t.Errorf("chip 0 reg 8 changed to %d, want 15 (unchanged)", ts.ay1.regs[8])
	}
}

// TestSounDriveChannels verifies the four ports route to the right channel and
// the stereo mix is A+C on the left, B+D on the right (each halved).
func TestSounDriveChannels(t *testing.T) {
	s := NewSounDrive()
	for _, p := range []uint16{0x0F, 0x1F, 0x4F, 0x5F} {
		if !s.HandlesPort(p) {
			t.Errorf("SounDrive should handle 0x%02X", p)
		}
	}
	s.SetTick(0)
	s.Write(0x0F, 0x80) // A = 16384
	s.Write(0x1F, 0x40) // B = 8192
	s.Write(0x4F, 0x20) // C = 4096
	s.Write(0x5F, 0x10) // D = 2048

	l, r := s.MeanLevelStereo(0, 100)
	if l != 10240 || r != 5120 {
		t.Errorf("stereo = (%d,%d), want (10240,5120)", l, r)
	}
}
