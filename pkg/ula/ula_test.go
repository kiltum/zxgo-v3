package ula

import (
	"testing"

	"github.com/kiltum/zxgo-v3/pkg/mem"
	"github.com/kiltum/zxgo-v3/pkg/model"
)

// newTestULA builds a ULA backed by a flat mapper whose screen area is filled
// with white-on-black pixels, so the rendered screen area is distinguishable
// from the black border.
func newTestULA(cfg model.Config) *ULA {
	m := mem.NewFlat48K(cfg)

	// Screen bitmap (0x4000-0x57FF) 0xFF (all pixels -> ink), attributes
	// (0x5800-0x5AFF) 0x07 (ink 7 = white, paper 0 = black, no bright/flash).
	// NewFlat48K returns a flat mapper whose SetBank is a no-op, so write the
	// screen bytes through the mapper's WriteByte instead.
	for i := 0; i < 6144; i++ {
		m.WriteByte(uint16(0x4000+i), 0xFF)
	}
	for i := 0; i < 768; i++ {
		m.WriteByte(uint16(0x5800+i), 0x07)
	}

	return New(m, cfg)
}

var allConfigs = []model.Config{
	model.Spectrum48K,
	model.Spectrum128K,
	model.Spectrum2A3,
	model.Pentagon128,
}

// TestULAGeometry verifies the framebuffer dimensions are derived from the
// model timing and geometry, not hardcoded to 352x288.
func TestULAGeometry(t *testing.T) {
	for _, cfg := range allConfigs {
		u := newTestULA(cfg)
		wantWidth := cfg.FramebufferWidth()
		wantHeight := cfg.FramebufferHeight()
		if u.Width() != wantWidth {
			t.Errorf("%s: Width() = %d, want %d", cfg.Name, u.Width(), wantWidth)
		}
		if u.Height() != wantHeight {
			t.Errorf("%s: Height() = %d, want %d", cfg.Name, u.Height(), wantHeight)
		}
		if len(u.screen) != u.Width()*u.Height() {
			t.Errorf("%s: framebuffer len = %d, want %d", cfg.Name, len(u.screen), u.Width()*u.Height())
		}
	}
}

// TestULAFullFrameNoOverflow renders one complete frame per model and asserts
// that no out-of-bounds write occurs and that the interrupt is asserted at the
// frame boundary. This is the regression test for the Pentagon 320-line frame
// that previously overflowed the fixed 288-line framebuffer.
func TestULAFullFrameNoOverflow(t *testing.T) {
	for _, cfg := range allConfigs {
		u := newTestULA(cfg)
		for i := 0; i < cfg.ClockEndFrame(); i++ {
			u.OneTick()
		}
		if u.Clock() != 0 {
			t.Errorf("%s: Clock() = %d after full frame, want 0", cfg.Name, u.Clock())
		}
		if u.IntAssertedUntil() <= 0 {
			t.Errorf("%s: interrupt not asserted after a full frame", cfg.Name)
		}
	}
}

// TestULAInterruptAtFrameWrap verifies /INT is asserted at the frame wrap and
// never anywhere else in the frame, on every model.
//
// ula_timing.txt describes the interrupt as 16 T-states into the scanline,
// "directly above the first pixel", but its line origin is 16 T before the first
// pixel while ours is 24 T before it (LeftBlank + LeftBorder), so that number is
// not ours. Measured on a 48K with a raster demo: the frame wrap is what lines
// the bars up with the screen, which is also the classic hardware behaviour for
// the 48K and the 128K alike (they share the interrupt position). Pinned here so
// a future reading of the diagram does not move it again on the strength of the
// number alone.
func TestULAInterruptAtFrameWrap(t *testing.T) {
	for _, cfg := range allConfigs {
		if cfg.InterruptOffset != 0 {
			t.Errorf("%s: InterruptOffset = %d, want 0", cfg.Name, cfg.InterruptOffset)
		}

		u := newTestULA(cfg)
		// IntAssertedUntil keeps the previous frame's tick (expired, and the
		// consumer compares it against the current tick), so "not asserted"
		// means "unchanged", not "zero".
		before := u.IntAssertedUntil()
		for i := 0; i < cfg.ClockEndFrame()-1; i++ {
			u.OneTick()
			if u.IntAssertedUntil() != before {
				t.Fatalf("%s: interrupt asserted at clock %d, want it at the wrap",
					cfg.Name, u.Clock())
			}
		}
		u.OneTick()
		if u.Clock() != 0 {
			t.Fatalf("%s: clock = %d after the wrap, want 0", cfg.Name, u.Clock())
		}
		if u.IntAssertedUntil() <= before {
			t.Errorf("%s: interrupt not asserted at the frame wrap", cfg.Name)
		}
	}
}

// TestULABorderWriteDeferred verifies the border colour change is deferred by
// borderWriteDelay T-states rather than applied immediately at the port write.
func TestULABorderWriteDeferred(t *testing.T) {
	u := newTestULA(model.Pentagon128)

	// Write border colour 3 at absoluteClock 0; it must not be visible yet.
	u.Write(0xFE, 0x03)
	if u.borderColor != 0 {
		t.Fatalf("border colour applied immediately (got %d, want 0)", u.borderColor)
	}

	// Tick borderWriteDelay times: the change is still not latched (the write
	// lands at the start of the tick that renders column borderWriteDelay).
	for i := 0; i < borderWriteDelay; i++ {
		u.OneTick()
	}
	if u.borderColor != 0 {
		t.Fatalf("border colour applied too early (got %d after %d ticks)", u.borderColor, borderWriteDelay)
	}

	// One more tick latches the colour.
	u.OneTick()
	if u.borderColor != 3 {
		t.Fatalf("border colour not applied at deferred tick (got %d, want 3)", u.borderColor)
	}
}

// TestULAScreenTopLeftPixel verifies the screen's top-left pixel lands at the
// scanline derived from TopBorderLines, and that the border above it is
// border-coloured.
func TestULAScreenTopLeftPixel(t *testing.T) {
	for _, cfg := range allConfigs {
		u := newTestULA(cfg)
		for i := 0; i < cfg.ClockEndFrame(); i++ {
			u.OneTick()
		}

		screenTop := cfg.TopBorderLines
		leftBorderPx := cfg.LeftBorderTStates * 2
		// Screen top-left pixel: framebuffer row screenTop, col leftBorderPx.
		idx := screenTop*u.Width() + leftBorderPx
		if u.screen[idx] != u.colors[7] {
			t.Errorf("%s: top-left screen pixel = 0x%08X, want white 0x%08X", cfg.Name, u.screen[idx], u.colors[7])
		}

		// The row just above the screen must be border-coloured.
		if screenTop > 0 {
			bidx := (screenTop-1)*u.Width() + leftBorderPx
			if u.screen[bidx] != u.colors[0] {
				t.Errorf("%s: top border pixel = 0x%08X, want border 0x%08X", cfg.Name, u.screen[bidx], u.colors[0])
			}
		}
	}
}

// TestULAHorizontalBorderRanges verifies the border/screen horizontal split
// within a screen line: left and right border are border-coloured, the screen
// columns are screen pixels.
func TestULAHorizontalBorderRanges(t *testing.T) {
	cfg := model.Pentagon128
	u := newTestULA(cfg)
	for i := 0; i < cfg.ClockEndFrame(); i++ {
		u.OneTick()
	}

	screenTop := cfg.TopBorderLines
	row := screenTop * u.Width()

	checks := []struct {
		col  int // T-state column
		want uint32
	}{
		{cfg.LeftBorderTStates - 1, u.colors[0]},   // left border
		{cfg.LeftBorderTStates, u.colors[7]},       // screen left edge
		{cfg.LeftBorderTStates + 127, u.colors[7]}, // screen right edge
		{cfg.LeftBorderTStates + 128, u.colors[0]}, // right border
	}
	for _, c := range checks {
		if got := u.screen[row+c.col*2]; got != c.want {
			t.Errorf("col %d: pixel = 0x%08X, want 0x%08X", c.col, got, c.want)
		}
	}
}

// TestKeyboardANDLogic verifies that multiple selected half-rows are ANDed together (C3).
func TestKeyboardANDLogic(t *testing.T) {
	u := newTestULA(model.Spectrum48K)
	u.audioState = false

	// Keyboard uses 0 = pressed, 1 = released (open-collector logic).
	u.keyboard[0] = 0xE0 // row 0: bits 0-4 pressed (cleared)
	u.keyboard[1] = 0xE0 // row 1: bits 0-4 pressed

	// Read with both row 0 and row 1 selected (ALL KEYS scan).
	allKeysResult := u.Read(0x00FE)

	// With correct AND logic: 0xE0 (row 0) & 0xE0 (row 1) = 0xE0 (bits 0-4 pressed).
	expected := uint8(0xE0)
	expected &^= 0x40 // Apply EAR bit (audioState false clears it)

	if allKeysResult != expected {
		t.Errorf("Multiple rows selected: got 0x%02X, expected 0x%02X (should AND all selected rows)", allKeysResult, expected)
	}

	// Single row selection works.
	singleRowResult := u.Read(0xFE00) // Only row 0 selected (bit 0 clear)
	singleExpected := u.keyboard[0]
	singleExpected &^= 0x40 // Apply EAR bit

	if singleRowResult != singleExpected {
		t.Errorf("Single row selected: got 0x%02X, expected 0x%02X", singleRowResult, singleExpected)
	}

	// Conflicting rows AND: released (0xFF) & pressed (0xE0) = 0xE0.
	u.keyboard[2] = 0xFF
	u.keyboard[3] = 0xE0

	mixedResult := u.Read(0xFDFF) // Select rows 2 and 3 (bits 2 and 3 clear)
	mixedExpected := uint8(0xE0)
	mixedExpected &^= 0x40 // Apply EAR bit

	if mixedResult != mixedExpected {
		t.Errorf("Mixed selection (row 2=0xFF, row 3=0xE0): got 0x%02X, expected 0x%02X (should AND with 0=pressed, 1=released)", mixedResult, mixedExpected)
	}
}

// TestKeyboardNoSelection returns 0xBF with EAR bit cleared when no rows are selected.
func TestKeyboardNoSelection(t *testing.T) {
	u := newTestULA(model.Spectrum48K)
	u.audioState = false

	// Set all rows to show some keys pressed.
	for i := 0; i < 8; i++ {
		u.keyboard[i] = 0xE0 // bits 0-4 pressed
	}

	// Read with no rows selected (all bits set in high byte: 0xFF00).
	result := u.Read(0xFF00)

	// With no rows selected, result starts at 0xFF and stays 0xFF, then EAR bit applied.
	expected := uint8(0xFF)
	expected &^= 0x40 // Clear EAR bit

	if result != expected {
		t.Errorf("No rows selected: got 0x%02X, expected 0x%02X (0xFF with EAR cleared = 0xBF)", result, expected)
	}
}

// TestKeyboardEARBit verifies the EAR tape input bit is correctly applied (C4).
func TestKeyboardEARBit(t *testing.T) {
	u := newTestULA(model.Spectrum48K)
	u.keyboard[0] = 0x1F

	// EAR bit low.
	u.audioState = false
	resultLow := u.Read(0xFE00)
	if resultLow&0x40 != 0 {
		t.Errorf("EAR bit checked: expected bit 6 clear, got 0x%02X", resultLow)
	}

	// EAR bit high.
	u.audioState = true
	resultHigh := u.Read(0xFE00)
	if resultHigh&0x40 == 0 {
		t.Errorf("EAR bit checked: expected bit 6 set, got 0x%02X", resultHigh)
	}
}
