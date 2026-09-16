package sound

import "github.com/kiltum/zxgo-v3/pkg/io_ports"

// SounDrive is a 4-channel 8-bit DAC on ports 0x0F, 0x1F, 0x4F and 0x5F
// (channels A, B, C, D). It is mixed stereo as A+C on the left and B+D on the
// right. Note: ports 0x1F and 0x5F overlap the Beta Disk interface, so a
// SounDrive and a Beta Disk cannot be fitted to the same machine; the emulator
// therefore does not register a SounDrive by default.
type SounDrive struct {
	a, b, c, d *DAC
}

// NewSounDrive creates a SounDrive with all four channels at rest.
func NewSounDrive() *SounDrive {
	return &SounDrive{a: NewDAC(), b: NewDAC(), c: NewDAC(), d: NewDAC()}
}

// --- io_ports.PortHandler ---

func (s *SounDrive) HandlesPort(port uint16) bool {
	switch byte(port) {
	case 0x0F, 0x1F, 0x4F, 0x5F:
		return true
	}
	return false
}

func (s *SounDrive) Read(port uint16) uint8 { return 0xFF }

func (s *SounDrive) Write(port uint16, value uint8) {
	lvl := int16(value) * covoxScale
	switch byte(port) {
	case 0x0F:
		s.a.SetLevel(lvl)
	case 0x1F:
		s.b.SetLevel(lvl)
	case 0x4F:
		s.c.SetLevel(lvl)
	case 0x5F:
		s.d.SetLevel(lvl)
	}
}

// --- sound.Source ---

func (s *SounDrive) SetTick(t int64) {
	s.a.SetTick(t)
	s.b.SetTick(t)
	s.c.SetTick(t)
	s.d.SetTick(t)
}

// MeanLevel returns the mono downmix (average of the four channels).
func (s *SounDrive) MeanLevel(from, to int64) int16 {
	sum := int32(s.a.MeanLevel(from, to)) + int32(s.b.MeanLevel(from, to)) +
		int32(s.c.MeanLevel(from, to)) + int32(s.d.MeanLevel(from, to))
	return int16(sum / 4)
}

// MeanLevelStereo returns A+C on the left and B+D on the right, each halved so
// two full-scale channels still fit in int16 after the mixer's DC-block.
func (s *SounDrive) MeanLevelStereo(from, to int64) (int16, int16) {
	l := (int32(s.a.MeanLevel(from, to)) + int32(s.c.MeanLevel(from, to))) / 2
	r := (int32(s.b.MeanLevel(from, to)) + int32(s.d.MeanLevel(from, to))) / 2
	return int16(l), int16(r)
}

func (s *SounDrive) Reset() {
	s.a.Reset()
	s.b.Reset()
	s.c.Reset()
	s.d.Reset()
}

func (s *SounDrive) ResetEvents() {
	s.a.ResetEvents()
	s.b.ResetEvents()
	s.c.ResetEvents()
	s.d.ResetEvents()
}

var _ io_ports.PortHandler = (*SounDrive)(nil)
var _ Source = (*SounDrive)(nil)
var _ StereoSource = (*SounDrive)(nil)
