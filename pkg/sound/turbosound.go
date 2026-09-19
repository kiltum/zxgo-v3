package sound

import (
	"github.com/kiltum/zxgo-v3/pkg/io_ports"
	"github.com/kiltum/zxgo-v3/pkg/state"
)

// AYChip is the interface the emulator uses for the AY sound source: a single
// AY-8912, a TurboSound pair or a TurboSound FM pair. All implement Source,
// PortHandler, the lifecycle methods and the state contract, so the mixer, the
// port bus and the state coordinator treat them identically.
//
// state.Identified is what lets the emulator save whichever chip the model has
// without knowing which one it is: the chip reports the chunk it is written as,
// because the three carry different payloads.
type AYChip interface {
	Source
	io_ports.PortHandler
	state.Identified
	SetTick(int64)
	Reset()
	SetClockFrequency(uint32)
}

// TurboSound is two AY-3-8912 chips selected by the NedoPC scheme: writing 0xFF
// to the register-select port (0xFFFD) selects chip 0, 0xFE selects chip 1, and
// any other value selects a register on the currently-selected chip. The pair is
// mixed stereo (chip 0 left, chip 1 right), which is the standard TurboSound
// output.
type TurboSound struct {
	ay1, ay2 *AY8912
	current  *AY8912
}

// NewTurboSound creates a TurboSound with both AYs clocked at chipFreq.
func NewTurboSound(chipFreq uint32) *TurboSound {
	ay1 := NewAY8912()
	ay2 := NewAY8912()
	ay1.SetClockFrequency(chipFreq)
	ay2.SetClockFrequency(chipFreq)
	return &TurboSound{ay1: ay1, ay2: ay2, current: ay1}
}

// SetClockFrequency clocks both chips.
func (t *TurboSound) SetClockFrequency(hz uint32) {
	t.ay1.SetClockFrequency(hz)
	t.ay2.SetClockFrequency(hz)
}

// ChunkID names the chunk a TurboSound pair is written as.
func (t *TurboSound) ChunkID() state.ID { return state.IDTurboSound }

// --- io_ports.PortHandler ---

// HandlesPort decodes the two AY ports (0xFFFD select, 0xBFFD data), the same
// as the single AY8912.
func (t *TurboSound) HandlesPort(port uint16) bool {
	if (port & 0x0002) != 0 {
		return false
	}
	sel := port & 0xC000
	return sel == 0xC000 || sel == 0x8000
}

// Read returns the selected register of the current chip (or 0xFF outside 0..13).
func (t *TurboSound) Read(port uint16) uint8 {
	if (port&0x0002) == 0 && (port&0xC000) == 0xC000 && t.current.selected <= 13 {
		return t.current.regs[t.current.selected]
	}
	return 0xFF
}

// Write handles the select port (chip switch or register select) and the data
// port (write to the current chip's latched register).
func (t *TurboSound) Write(port uint16, value uint8) {
	switch port & 0xC000 {
	case 0xC000: // 0xFFFD: chip switch (0xFF/0xFE) or register select
		switch value {
		case 0xFF:
			t.current = t.ay1
		case 0xFE:
			t.current = t.ay2
		default:
			t.current.selected = value & 0x0F
		}
	case 0x8000: // 0xBFFD: data write to the latched register
		t.current.writeRegister(t.current.selected, value)
	}
}

// --- sound.Source ---

// SetTick forwards the CPU tick to both chips for write timestamping.
func (t *TurboSound) SetTick(tick int64) {
	t.ay1.SetTick(tick)
	t.ay2.SetTick(tick)
}

// MeanLevel returns the mono downmix (average of the two chips).
func (t *TurboSound) MeanLevel(from, to int64) int16 {
	l := int32(t.ay1.MeanLevel(from, to))
	r := int32(t.ay2.MeanLevel(from, to))
	return int16((l + r) / 2)
}

// MeanLevelStereo returns chip 0 on the left ear and chip 1 on the right.
func (t *TurboSound) MeanLevelStereo(from, to int64) (int16, int16) {
	return t.ay1.MeanLevel(from, to), t.ay2.MeanLevel(from, to)
}

// Reset returns both chips to their power-on state and selects chip 0.
func (t *TurboSound) Reset() {
	t.ay1.Reset()
	t.ay2.Reset()
	t.current = t.ay1
}

var _ AYChip = (*TurboSound)(nil)
var _ io_ports.PortHandler = (*TurboSound)(nil)
var _ Source = (*TurboSound)(nil)
var _ StereoSource = (*TurboSound)(nil)

// AY8912 also satisfies AYChip.
var _ AYChip = (*AY8912)(nil)
