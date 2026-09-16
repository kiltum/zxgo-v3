// Package io_ports implements the ZX Spectrum I/O port system.
//
// The PortBus dispatches I/O reads and writes to registered PortHandlers.
// The ZX Spectrum uses partially-decoded I/O addressing (only the low byte
// of the port address matters for most peripherals, but some use bit 15-14).
//
// Multiple handlers can respond to the same port. On reads, results are
// ANDed together (open-collector bus behaviour). On writes, all handlers
// receive the write.
package io_ports

import (
	"log/slog"
)

// PortHandler is implemented by every I/O peripheral.
// The port parameter is the FULL 16-bit address as written by the Z80.
type PortHandler interface {
	// HandlesPort returns true if this peripheral responds to the given port.
	HandlesPort(port uint16) bool

	// Read returns the value this peripheral drives on the data bus.
	Read(port uint16) uint8

	// Write handles a write to this peripheral.
	Write(port uint16, value uint8)
}

// FloatingBusProvider supplies the data-bus value for the bits of a port read
// that no handler drives (the ZX "floating bus": the ULA's screen/attribute
// byte during the contended screen area). When set, those bits take its value
// instead of 0xFF.
type FloatingBusProvider interface {
	FloatingBus() uint8
}

// FloatingBusAtProvider is the optional refinement: the floating byte a given
// number of T-states ahead of the provider's current position. A provider that
// implements it is sampled at the I/O cycle's T-state; one that does not is
// sampled at its current position, which is the old instruction-granularity
// behaviour. The ULA implements this so a port read mid-instruction returns the
// byte the chip is fetching at that instant rather than at the instruction start.
type FloatingBusAtProvider interface {
	FloatingBusAt(delta int) uint8
}

// AttachedHandler is implemented by a PortHandler that drives only some of the
// eight data-bus bits on a read. attached has a 1 in every bit the handler
// drives; undriven bits must be left HIGH in the returned value, because the
// merge below only ANDs (open-collector bus, exactly like the ANDing between
// handlers). Handlers that do not implement it are treated as driving all eight
// bits, which is the common case and keeps every existing handler unchanged.
//
// This is Fuse's callback_info.attached mask (ref/fuse-1.9.0/periph.c:293-305,
// :340-364): result &= value | attached, with the floating byte filling the
// bits that are still undriven.
type AttachedHandler interface {
	ReadAttached(port uint16) (value, attached uint8)
}

// PortBus is the central I/O dispatcher.
// All PortHandlers register here. The CPU's I/O operations go through the bus,
// which fans out to matching handlers.
type PortBus struct {
	handlers    []PortHandler
	floatingBus FloatingBusProvider
	log         *slog.Logger // nil means silent

	// floatingOffset is how far ahead of the provider's current position a read
	// should be sampled. The bus sets it per I/O cycle when the CPU reports
	// machine cycles; it stays 0 for reads that happen outside an instruction
	// (the debugger and MCP ReadPort path).
	floatingOffset int
}

// NewPortBus creates an empty PortBus.
func NewPortBus() *PortBus {
	return &PortBus{
		handlers: make([]PortHandler, 0, 8),
	}
}

// SetLogger sets the logger for unclaimed port warnings. Called from emulator setup.
func (pb *PortBus) SetLogger(l *slog.Logger) { pb.log = l }

// SetFloatingBus installs the floating-bus provider (the ULA, for models that
// have one). Called from emulator setup; when nil the behaviour is unchanged.
func (pb *PortBus) SetFloatingBus(p FloatingBusProvider) { pb.floatingBus = p }

// SetFloatingOffset sets how many T-states ahead of the provider's current
// position the next read should sample. The bus sets this for each I/O cycle;
// Read consumes it, so a read from outside an instruction (the debugger and the
// MCP ReadPort path) samples at the provider's current position, never at
// whatever offset the last I/O cycle happened to use.
func (pb *PortBus) SetFloatingOffset(delta int) { pb.floatingOffset = delta }

// floatingByte reads the provider at the requested offset when it can, and at
// its current position otherwise.
func (pb *PortBus) floatingByte() uint8 {
	if pb.floatingBus == nil {
		return 0xFF
	}
	if at, ok := pb.floatingBus.(FloatingBusAtProvider); ok {
		return at.FloatingBusAt(pb.floatingOffset)
	}
	return pb.floatingBus.FloatingBus()
}

// Register adds a PortHandler. Handlers are checked in registration order.
func (pb *PortBus) Register(h PortHandler) {
	pb.handlers = append(pb.handlers, h)
}

// Read returns the logical AND of all handlers that respond to this port.
// This simulates open-collector bus behaviour. Handlers that report an
// AttachedHandler mask drive only those bits; every bit left undriven by all of
// them is taken from the floating bus (or 0xFF when no provider is installed),
// which is what makes an undecoded port read and a read of the ULA port itself
// return the byte the ULA is fetching.
func (pb *PortBus) Read(port uint16) uint8 {
	result := uint8(0xFF)
	attached := uint8(0)
	// allDriven is separate from attached on purpose: a maskless handler drives
	// all eight bits, but it must not stop the *next* attached handler from
	// contributing its own bits (result &= value | 0xFF would swallow it). The
	// Kempston joystick is registered maskless and the Beta Disk after it, so
	// this ordering is real on port 0x1F.
	allDriven := false
	for _, h := range pb.handlers {
		if !h.HandlesPort(port) {
			continue
		}
		if ah, ok := h.(AttachedHandler); ok {
			value, mask := ah.ReadAttached(port)
			result &= value | attached
			attached |= mask
			continue
		}
		// No mask: the handler drives all eight bits (Fuse: *attached = 0xFF).
		result &= h.Read(port)
		allDriven = true
	}
	// Nothing driven at all means the port is unclaimed: the whole byte floats,
	// which is the same arithmetic as the merge below with attached == 0.
	if !allDriven {
		result &= pb.floatingByte() | attached
		pb.floatingOffset = 0
	}
	if attached == 0 && !allDriven && pb.log != nil {
		pb.log.Debug("unclaimed port read", "port", port, "val", result)
	}
	return result
}

// Write fans out to all handlers that respond to this port.
func (pb *PortBus) Write(port uint16, value uint8) {
	claimed := false
	for _, h := range pb.handlers {
		if h.HandlesPort(port) {
			claimed = true
			h.Write(port, value)
		}
	}
	if !claimed && pb.log != nil {
		pb.log.Debug("unclaimed port write", "port", port, "val", value)
	}
}
