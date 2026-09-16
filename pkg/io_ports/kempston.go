package io_ports

// Kempston implements a Kempston joystick on port 0x1F.
// Bits: D0=Right, D1=Left, D2=Down, D3=Up, D4=Fire
type Kempston struct {
	state uint8
	trdos *TRDosState // shared TR-DOS arbitration state (per emulator)
}

func NewKempston() *Kempston {
	return &Kempston{}
}

// SetTRDosState wires the shared TR-DOS arbitration state so HandlesPort stands
// down while TR-DOS is paged in.
func (k *Kempston) SetTRDosState(s *TRDosState) { k.trdos = s }

// HandlesPort claims port 0x1F, which the WD1793 status register also occupies.
//
// The Beta Disk interface only drives the bus while its ROM is paged in, so the
// joystick must stand down for exactly that window. Without this the port bus
// ANDs both handlers' results and an idle Kempston (0x00) masks the entire
// WD1793 status register to zero -- TR-DOS then sees no index pulse, no BUSY
// and no error bits, and reports "no disk" for every command.
func (k *Kempston) HandlesPort(port uint16) bool {
	if k.trdos != nil && k.trdos.Active() {
		return false
	}
	return (port & 0xFF) == 0x1F
}

func (k *Kempston) Read(port uint16) uint8         { return k.state }
func (k *Kempston) Write(port uint16, value uint8) {}

func (k *Kempston) SetState(right, left, down, up, fire bool) {
	k.state = 0
	if right {
		k.state |= 0x01
	}
	if left {
		k.state |= 0x02
	}
	if down {
		k.state |= 0x04
	}
	if up {
		k.state |= 0x08
	}
	if fire {
		k.state |= 0x10
	}
}

func (k *Kempston) Reset() { k.state = 0 }
