package ula

import (
	"github.com/kiltum/zxgo-v3/pkg/state"
)

// SaveState and LoadState are the ULA's half of the native state format
// (STATE_DESIGN.md).
//
// The raster position is the reason the ULA has a chunk at all: the next
// instruction's contention and the floating bus both depend on where the beam
// is, so a machine restored anywhere except on a frame boundary needs it back
// exactly. Everything here is either that position or something derived from
// how long the machine has been running.
//
// Not stored, and why:
//
//   - The framebuffer. It is redrawn from RAM as the raster advances, so the one
//     visible artefact of a load is that the rows above the restored position
//     keep whatever the target machine last drew until the frame wraps - one
//     frame, at most 20 ms.
//   - The keyboard matrix. A key held when the state was written is not held
//     now; every key is released on load, which is what STATE_DESIGN section 8
//     specifies.
//   - snowEnabled and the geometry (width, height, clockPerLine, the color
//     table). They come from the model config, not from the file.
//   - horClock, which render() writes and nothing reads.
//
// The snow ring is written element by element rather than as a slice: its
// length is a compile-time constant of this file, so a length in the payload
// would be one more thing to validate for no gain.

// SaveState encodes the raster position, the border and the timing deadlines.
func (u *ULA) SaveState(e *state.Encoder) error {
	e.I64(u.absoluteClock) // the CPU-tick timeline every other deadline is on
	e.I64(int64(u.clock))  // position within the frame
	e.I64(int64(u.line))
	e.U8(uint8(u.borderColor))
	e.U8(uint8(u.pendingBorder))
	e.I64(u.pendingBorderTick)
	e.Bool(u.flash)
	e.U8(uint8(u.frameCnt))

	// The snow ring: a CPU write's byte lingers on the data bus for a few
	// T-states, so a save taken inside that window has to carry it or the
	// restored machine loses one collision it would have had.
	for i := 0; i < snowRing; i++ {
		e.I64(u.snowTicks[i])
		e.U8(u.snowValues[i])
	}
	e.U8(uint8(u.snowNext))

	e.I64(u.intAssertedUntil)
	e.I64(u.interruptFrameCount)
	e.Bool(u.audioState)
	return nil
}

// LoadState applies a payload written by SaveState.
//
// It parses into locals first, then assigns, so a payload that stops early
// leaves the machine as it was. Every key is released as part of the apply.
func (u *ULA) LoadState(d *state.Decoder) error {
	var (
		absoluteClock   int64
		clock, line     int
		borderColor     int
		pendingBorder   int
		pendingBorderTK int64
		flash           bool
		frameCnt        int
		snowTicks       [snowRing]int64
		snowValues      [snowRing]uint8
		snowNext        int
		intAssertedTill int64
		intFrameCount   int64
		audioState      bool
	)

	absoluteClock = d.I64()
	clock = int(d.I64())
	line = int(d.I64())
	borderColor = int(d.U8())
	pendingBorder = int(d.U8())
	pendingBorderTK = d.I64()
	flash = d.Bool()
	frameCnt = int(d.U8())

	for i := 0; i < snowRing; i++ {
		snowTicks[i] = d.I64()
		snowValues[i] = d.U8()
	}
	snowNext = int(d.U8())

	intAssertedTill = d.I64()
	intFrameCount = d.I64()
	audioState = d.Bool()

	if err := d.Err(); err != nil {
		return err
	}

	u.absoluteClock = absoluteClock
	u.clock = clock
	u.line = line
	u.borderColor = borderColor
	u.pendingBorder = pendingBorder
	u.pendingBorderTick = pendingBorderTK
	u.flash = flash
	u.frameCnt = frameCnt
	u.snowTicks = snowTicks
	u.snowValues = snowValues
	u.snowNext = snowNext % snowRing
	u.intAssertedUntil = intAssertedTill
	u.interruptFrameCount = intFrameCount
	u.audioState = audioState

	// A key held when the state was written is not held now.
	for i := range u.keyboard {
		u.keyboard[i] = 0xFF
	}
	return nil
}

var _ state.Component = (*ULA)(nil)
