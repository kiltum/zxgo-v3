package ula

import (
	"testing"

	"github.com/kiltum/zxgo-v3/pkg/model"
)

// TestFloatingBus verifies the ULA drives the data bus with the screen bitmap
// or attribute byte at the current raster position during the visible area, and
// 0xFF outside it. newTestULA fills the screen bitmap with 0xFF and attributes
// with 0x07; we also plant a distinct screen byte so the screen phase is not
// confusable with the idle 0xFF.
func TestFloatingBus(t *testing.T) {
	u := newTestULA(model.Spectrum48K)
	u.mem.WriteByte(0x4000, 0xA5) // distinct first screen byte

	sawAttr := false
	sawScreen := false
	for clock := 0; clock < u.clockEndFrame; clock++ {
		u.clock = clock
		switch u.FloatingBus() {
		case 0x07:
			sawAttr = true
		case 0xA5:
			sawScreen = true
		}
	}

	if !sawAttr {
		t.Error("never returned the attribute byte inside the screen area")
	}
	if !sawScreen {
		t.Error("never returned the distinct screen byte (0xA5)")
	}

	// Outside the visible screen area the bus is idle (0xFF).
	u.clock = 0
	if v := u.FloatingBus(); v != 0xFF {
		t.Errorf("border: FloatingBus() = 0x%02X, want 0xFF", v)
	}
}
