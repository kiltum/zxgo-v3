package sound

import "github.com/kiltum/zxgo-v3/pkg/io_ports"

// TSFM is the Turbo Sound FM board: two YM2203 (OPN) chips selected by the
// NedoPC scheme (write 0xFF to 0xFFFD selects chip 0, 0xFE selects chip 1,
// any other value selects a register on the current chip). This gives 6 FM plus
// 6 SSG channels, mixed stereo (chip 0 left, chip 1 right).
type TSFM struct {
	chip0, chip1 *YM2203
	current      *YM2203
	readiness    bool // r bit of the pseudo-register: true = poll ready flag on read
}

// NewTSFM creates a two-chip Turbo Sound FM clocked at fmClock Hz.
func NewTSFM(fmClock float64) *TSFM {
	c0 := NewYM2203(fmClock)
	c1 := NewYM2203(fmClock)
	return &TSFM{chip0: c0, chip1: c1, current: c0}
}

// SetClockFrequency sets the FM clock of both chips.
func (t *TSFM) SetClockFrequency(hz uint32) {
	t.chip0.SetClockFrequency(hz)
	t.chip1.SetClockFrequency(hz)
}

// SetCPUClock sets the Spectrum CPU clock of both chips (tick->sample conversion).
func (t *TSFM) SetCPUClock(hz int) {
	t.chip0.SetCPUClock(hz)
	t.chip1.SetCPUClock(hz)
}

// SetSampleRate sets the output sample rate of both chips.
func (t *TSFM) SetSampleRate(rate int) {
	t.chip0.SetSampleRate(rate)
	t.chip1.SetSampleRate(rate)
}

// SetTick forwards the tick to both chips' SSG parts.
func (t *TSFM) SetTick(tick int64) {
	t.chip0.SetTick(tick)
	t.chip1.SetTick(tick)
}

// --- io_ports.PortHandler ---

func (t *TSFM) HandlesPort(port uint16) bool {
	if (port & 0x0002) != 0 {
		return false
	}
	sel := port & 0xC000
	return sel == 0xC000 || sel == 0x8000
}

func (t *TSFM) Read(port uint16) uint8 {
	if (port & 0x0002) != 0 {
		return 0xFF
	}
	switch port & 0xC000 {
	case 0xC000: // 0xFFFD: status register (timer overflow flags + busy)
		v := t.current.status()
		if t.readiness && t.current.busy() {
			v |= 0x80
		}
		return v
	case 0x8000: // 0xBFFD: SSG register read-back
		if t.current.regAddr <= 0x0F {
			return t.current.ay.ReadRegister(t.current.regAddr)
		}
		return 0x00
	}
	return 0xFF
}

func (t *TSFM) Write(port uint16, value uint8) {
	if (port & 0x0002) != 0 {
		return
	}
	switch port & 0xC000 {
	case 0xC000: // 0xFFFD: pseudo-register (chip select) or register select
		if value >= 0xF8 {
			// pseudo-register %11111frc: bit 0 (c) selects the chip, bit 1 (r)
			// enables the ready-flag poll. The FPGA handles it; it does not change
			// the YM2203's current register.
			if value&1 == 0 {
				t.current = t.chip0
			} else {
				t.current = t.chip1
			}
			t.readiness = (value & 0x02) == 0
		} else {
			t.current.regAddr = value
			t.current.setBusy()
		}
	case 0x8000: // 0xBFFD: register data
		t.current.writeReg(t.current.regAddr, value)
		t.current.setBusy()
	}
}

// --- sound.Source ---

func (t *TSFM) MeanLevel(from, to int64) int16 {
	l, r := t.MeanLevelStereo(from, to)
	return int16((int32(l) + int32(r)) / 2)
}

func (t *TSFM) MeanLevelStereo(from, to int64) (int16, int16) {
	return t.chip0.MeanLevel(from, to), t.chip1.MeanLevel(from, to)
}

func (t *TSFM) Reset() {
	t.chip0.Reset()
	t.chip1.Reset()
	t.current = t.chip0
	t.readiness = false
}

var _ AYChip = (*TSFM)(nil)
var _ io_ports.PortHandler = (*TSFM)(nil)
var _ Source = (*TSFM)(nil)
var _ StereoSource = (*TSFM)(nil)
