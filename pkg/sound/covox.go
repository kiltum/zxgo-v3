package sound

import "github.com/kiltum/zxgo-v3/pkg/io_ports"

// covoxScale maps the 8-bit DAC value to a level. The value is unipolar
// (0..255 -> 0..32640), matching Fuse's sound_covox_write (val * 128); the
// mixer's DC-block AC-couples it, so a full-scale write swings ~±16k around
// zero after coupling.
const covoxScale = 128

// Covox is a mono 8-bit DAC (the "Covox" add-on). It implements PortHandler and
// Source (via its internal DAC). The Pentagon/ATM variant decodes port 0xFB, the
// Scorpion/ZX Profi variant 0xDD (both full low-byte decode).
type Covox struct {
	dac  *DAC
	port uint8
}

// NewCovox creates a Covox responding to the given low-byte port (0xFB =
// Pentagon/ATM, 0xDD = Scorpion/ZX Profi).
func NewCovox(port uint8) *Covox {
	return &Covox{dac: NewDAC(), port: port}
}

// DAC exposes the underlying DAC (tests, and the SounDrive panning).
func (c *Covox) DAC() *DAC { return c.dac }

// --- io_ports.PortHandler ---

// HandlesPort decodes the Covox port on the full low byte.
func (c *Covox) HandlesPort(port uint16) bool {
	return byte(port) == c.port
}

// Read returns 0xFF: the Covox has no read registers and must not pull down bits
// other handlers own on a shared bus.
func (c *Covox) Read(port uint16) uint8 { return 0xFF }

// Write sets the DAC level from the 8-bit sample.
func (c *Covox) Write(port uint16, value uint8) {
	c.dac.SetLevel(int16(value) * covoxScale)
}

// --- sound.Source ---

// SetTick forwards the CPU tick to the DAC for write timestamping.
func (c *Covox) SetTick(t int64) { c.dac.SetTick(t) }

// MeanLevel forwards to the DAC.
func (c *Covox) MeanLevel(from, to int64) int16 { return c.dac.MeanLevel(from, to) }

// Reset returns the Covox to silence.
func (c *Covox) Reset() { c.dac.Reset() }

// ResetEvents forwards to the DAC.
func (c *Covox) ResetEvents() { c.dac.ResetEvents() }

var _ io_ports.PortHandler = (*Covox)(nil)
var _ Source = (*Covox)(nil)
