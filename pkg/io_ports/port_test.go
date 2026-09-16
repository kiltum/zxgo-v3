package io_ports

import "testing"

// fakeFloat is a fixed floating-bus provider for tests.
type fakeFloat struct{ v uint8 }

func (f *fakeFloat) FloatingBus() uint8 { return f.v }

// TestPortBusFloatingBusUnattached verifies that a port read no handler drives
// returns the floating bus value when one is installed, and 0xFF otherwise.
func TestPortBusFloatingBusUnattached(t *testing.T) {
	pb := NewPortBus()

	// No handler claims 0x00, so without a floating bus it reads 0xFF.
	if got := pb.Read(0x00); got != 0xFF {
		t.Fatalf("unattached read without floating bus = 0x%02X, want 0xFF", got)
	}

	pb.SetFloatingBus(&fakeFloat{0xA5})
	if got := pb.Read(0x00); got != 0xA5 {
		t.Errorf("unattached read with floating bus = 0x%02X, want 0xA5", got)
	}
}

// TestPortBusReadAnd verifies the open-collector AND across multiple handlers
// is preserved when a floating bus is installed (claimed reads are untouched).
func TestPortBusReadAnd(t *testing.T) {
	pb := NewPortBus()
	pb.SetFloatingBus(&fakeFloat{0xA5})

	pb.Register(&stubHandler{port: 0xFE, value: 0xF0})
	pb.Register(&stubHandler{port: 0xFE, value: 0x0F})
	if got := pb.Read(0xFE); got != 0x00 {
		t.Errorf("ANDed read = 0x%02X, want 0x00", got)
	}
}

type stubHandler struct {
	port  uint16
	value uint8
}

func (h *stubHandler) HandlesPort(port uint16) bool   { return port == h.port }
func (h *stubHandler) Read(port uint16) uint8         { return h.value }
func (h *stubHandler) Write(port uint16, value uint8) {}

// attachedHandler is a handler that drives only some of the data-bus bits, the
// way the ULA does on port 0xFE (keyboard + EAR) and the way the beeper drives
// none at all.
type attachedHandler struct {
	port     uint16
	value    uint8
	attached uint8
}

func (h *attachedHandler) HandlesPort(port uint16) bool   { return port == h.port }
func (h *attachedHandler) Read(port uint16) uint8         { return h.value }
func (h *attachedHandler) Write(port uint16, value uint8) {}
func (h *attachedHandler) ReadAttached(port uint16) (uint8, uint8) {
	return h.value, h.attached
}

// TestPortBusAttachedMask is the rule ported from Fuse's callback_info.attached:
// bits a handler drives keep its value, bits it leaves undriven take the
// floating byte, and a handler that reports no mask keeps driving all eight.
//
// The ULA's own value must leave its undriven bits HIGH (0xBF or 0xFF: keyboard
// released, EAR low or high) - the merge only ANDs, so a handler that returns 0
// in an undriven bit would pull that bit down regardless of the floating bus.
func TestPortBusAttachedMask(t *testing.T) {
	const (
		ulaMask    = 0x5F // D0-D4 keyboard, D6 EAR
		keysFree   = 0xFF // nothing pressed, EAR high
		keysFreeLo = 0xBF // nothing pressed, EAR low
	)
	cases := []struct {
		name     string
		value    uint8
		attached uint8
		floating uint8
		hasBus   bool
		want     uint8
	}{
		{"undriven D5/D7 come from the screen byte", keysFree, ulaMask, 0xA0, true, 0xFF},
		{"a screen byte with D5/D7 clear", keysFree, ulaMask, 0x00, true, 0x5F},
		{"driven keyboard/EAR bits are kept", keysFreeLo, ulaMask, 0xA0, true, 0xBF},
		{"no provider leaves the read exactly as it was", keysFreeLo, ulaMask, 0, false, 0xBF},
		{"nothing driven: the whole byte floats", 0xFF, 0x00, 0x55, true, 0x55},
		{"handler without a mask drives all eight", 0x42, 0xFF, 0x00, true, 0x42},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pb := NewPortBus()
			if c.hasBus {
				pb.SetFloatingBus(&fakeFloat{c.floating})
			}
			var h PortHandler = &attachedHandler{port: 0xFE, value: c.value, attached: c.attached}
			if c.attached == 0xFF {
				h = &stubHandler{port: 0xFE, value: c.value}
			}
			pb.Register(h)
			if got := pb.Read(0xFE); got != c.want {
				t.Errorf("read = 0x%02X, want 0x%02X", got, c.want)
			}
		})
	}
}

// The real 0xFE stack - the ULA plus the beeper, which claims the same port on
// every access but drives nothing - must still deliver the floating byte.
func TestPortBusAttachedAndNonAttachedMix(t *testing.T) {
	pb := NewPortBus()
	pb.SetFloatingBus(&fakeFloat{0xA0}) // a screen byte with D5 and D7 set

	pb.Register(&attachedHandler{port: 0xFE, value: 0xFF, attached: 0x5F}) // ULA: keys released, EAR high
	pb.Register(&attachedHandler{port: 0xFE, value: 0xFF, attached: 0x00}) // beeper: drives nothing

	if got, want := pb.Read(0xFE), uint8(0xFF); got != want {
		t.Errorf("merged read = 0x%02X, want 0x%02X", got, want)
	}

	// With the floating byte's D5/D7 clear, only the driven bits survive.
	pb.SetFloatingBus(&fakeFloat{0x00})
	if got, want := pb.Read(0xFE), uint8(0x5F); got != want {
		t.Errorf("merged read = 0x%02X, want 0x%02X", got, want)
	}
}

// A maskless handler (a joystick, the way Kempston is registered) followed by an
// attached one (the Beta Disk on port 0x1F/0xFF) must AND both values and leave
// the floating bus out of it: the maskless handler drives all eight bits, so
// nothing is left for the floating byte to fill.
func TestPortBusMasklessThenAttached(t *testing.T) {
	pb := NewPortBus()
	pb.SetFloatingBus(&fakeFloat{0x00}) // would clear everything if it were merged

	pb.Register(&stubHandler{port: 0x1F, value: 0xF3})                     // maskless: drives all 8
	pb.Register(&attachedHandler{port: 0x1F, value: 0x7F, attached: 0xC0}) // drives D6/D7 only

	// 0xF3 & 0x7F == 0x73, with no floating-bus merge.
	if got, want := pb.Read(0x1F), uint8(0x73); got != want {
		t.Errorf("read = 0x%02X, want 0x%02X", got, want)
	}
}
