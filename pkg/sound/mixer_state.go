package sound

import "github.com/kiltum/zxgo-v3/pkg/state"

// SaveState and LoadState are the mixer's half of the native state format
// (STATE_DESIGN.md).
//
// The grid position is machine state, not host state: every sample's timestamp
// is derived from totalTicks, so a restored machine has to resume the grid
// where it stopped or the next Resolve would generate from tick 0 (the resync
// path would jump it forward, but that is a repair, not a restore, and it is
// audible). The DC-block history is the rest of it: the filter is one pole per
// ear with a one-sample memory, and starting it from zero after a load puts a
// step into the first samples that follow.
//
// Not stored: the ring, the scratch buffers and the output device. They are
// host buffers - the ring is staging, and the emulator discards whatever is in
// it after a load.
type mixerState struct {
	nextSample    int64
	generated     int64
	dcInL, dcOutL int32
	dcInR, dcOutR int32
}

// SaveState encodes the grid position and the filter history.
func (m *Mixer) SaveState(e *state.Encoder) error {
	e.I64(m.nextSample)
	e.I64(m.generated)
	e.I32(m.dcInL)
	e.I32(m.dcOutL)
	e.I32(m.dcInR)
	e.I32(m.dcOutR)
	return nil
}

// LoadState applies a payload written by SaveState. The ring is reset, so the
// device is fed the restored machine's samples and nothing else.
func (m *Mixer) LoadState(d *state.Decoder) error {
	var s mixerState
	s.nextSample = d.I64()
	s.generated = d.I64()
	s.dcInL = d.I32()
	s.dcOutL = d.I32()
	s.dcInR = d.I32()
	s.dcOutR = d.I32()
	if err := d.Err(); err != nil {
		return err
	}

	m.nextSample = s.nextSample
	m.generated = s.generated
	m.dcInL, m.dcOutL = s.dcInL, s.dcOutL
	m.dcInR, m.dcOutR = s.dcInR, s.dcOutR
	m.ring.Reset()
	return nil
}

var _ state.Component = (*Mixer)(nil)
