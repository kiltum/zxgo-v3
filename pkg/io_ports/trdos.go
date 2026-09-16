package io_ports

// TRDosState carries the TR-DOS ROM paging state that arbitrates port 0x1F
// between the Beta Disk controller and the Kempston joystick.
//
// It lives in io_ports rather than in pkg/media because it arbitrates the
// I/O bus, and two peripherals in different packages need to see it:
//
//   - the Beta Disk controller (pkg/media), which owns ports 0x1F/0x3F/0x5F/
//     0x7F/0xFF, and
//   - the Kempston joystick, which decodes port 0x1F as well.
//
// On real hardware the Beta 128 interface only drives the bus while its ROM
// is paged in, so exactly one of the two answers any given IN. Without a
// shared flag both handlers claim port 0x1F, PortBus ANDs their results, and
// an idle Kempston (0x00) masks the whole WD1793 status register to zero.
//
// The memory mapper is the writer: it sets the state from the M1 fetch trap in
// 0x3D00-0x3DFF and clears it when an opcode is fetched from 0x4000 or above.
// The state is a per-emulator instance (not a package global) so two machines
// in one process (the MCP worker) do not interfere.
type TRDosState struct {
	active bool
}

// SetActive records whether the TR-DOS ROM is currently paged in.
func (s *TRDosState) SetActive(active bool) { s.active = active }

// Active reports whether the TR-DOS ROM is currently paged in.
func (s *TRDosState) Active() bool { return s.active }
