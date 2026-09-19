package gs

import (
	"errors"

	"github.com/kiltum/zxgo-v3/pkg/state"
)

// errRAMSize is a payload whose GS RAM is not the size this card has: the card
// is 512K in 16K blocks, and a block of another size would be truncated into it.
var errRAMSize = errors.New("gs: RAM block is not the size this card has")

// Native state format for the General Sound card (STATE_DESIGN.md).
//
// The GS is a machine inside the machine, so its chunk is a machine's worth of
// state: its own Z80, the RAM that Z80 sees, the paging register that decides
// which 32K page is in the window, the host handshake registers and the DAC
// levels with their volumes.
//
// The 32K ROM is not stored. Writes to segment 0 are refused by segInfo, so the
// ROM never changes and the machine a state is restored into already has it.
// RAM is stored as one block from the ROM's end, which is where it lives in the
// card's flat memory.

// gsState is the GS's own state, minus the nested CPU.
type gsState struct {
	ram     []byte
	mapping uint8

	command, data, state, output uint8

	dac [4]uint8
	vol [4]uint8

	gsTicks      int64
	interruptCnt int64

	events        []gsEvent
	resolvedLeft  int16
	resolvedRight int16
}

// SaveState encodes the card.
//
// The DAC event ring is written without the events the mixer has already
// consumed: those are history, and the ring the mixer reads next begins at its
// cursor.
func (g *GS) SaveState(e *state.Encoder) error {
	if err := nest(e, g.cpu); err != nil {
		return err
	}

	// RAM only: [romSize:] of the flat memory. The ROM is the machine's.
	e.Bytes(g.mem[romSize:])
	e.U8(g.mapping)
	e.U8(g.command)
	e.U8(g.data)
	e.U8(g.state)
	e.U8(g.output)
	for _, v := range g.dac {
		e.U8(v)
	}
	for _, v := range g.vol {
		e.U8(v)
	}
	e.I64(g.gsTicks)
	e.I64(g.interruptCnt)

	// The level in effect at the mixer's cursor, so the first window after a
	// restore starts from where the card's output actually was rather than from
	// silence.
	e.I16(g.resolvedLeft)
	e.I16(g.resolvedRight)

	live := g.events[g.cur:]
	e.Count(len(live))
	for _, ev := range live {
		e.I64(ev.gsTick)
		e.I16(ev.left)
		e.I16(ev.right)
	}
	return nil
}

// LoadState applies a payload written by SaveState.
func (g *GS) LoadState(d *state.Decoder) error {
	cpuBlob := d.Bytes()
	ram := d.Bytes()
	mapping := d.U8()
	command := d.U8()
	data := d.U8()
	st := d.U8()
	output := d.U8()
	var dac, vol [4]uint8
	for i := range dac {
		dac[i] = d.U8()
	}
	for i := range vol {
		vol[i] = d.U8()
	}
	gsTicks := d.I64()
	interruptCnt := d.I64()
	resolvedLeft := d.I16()
	resolvedRight := d.I16()

	n := d.Count(12) // one event is a tick and two levels
	if err := d.Err(); err != nil {
		return err
	}
	events := make([]gsEvent, 0, n)
	for i := 0; i < n; i++ {
		ev := gsEvent{gsTick: d.I64(), left: d.I16(), right: d.I16()}
		if err := d.Err(); err != nil {
			return err
		}
		events = append(events, ev)
	}

	// The RAM has to be exactly the RAM this card has before anything is
	// written: a short block would otherwise leave the card half-restored.
	if len(ram) != ramSize {
		return errRAMSize
	}

	// The CPU first, so a refusal does not leave the memory changed.
	if err := g.cpu.LoadState(state.NewDecoder(cpuBlob, d.Version())); err != nil {
		return err
	}

	copy(g.mem[romSize:], ram)
	g.mapping = mapping
	g.command, g.data, g.state, g.output = command, data, st, output
	g.dac = dac
	g.vol = vol
	g.gsTicks = gsTicks
	g.interruptCnt = interruptCnt
	g.events = events
	g.cur = 0
	g.resolvedLeft, g.resolvedRight = resolvedLeft, resolvedRight
	return nil
}

// ResetEvents drops the DAC events the mixer has already consumed. The state
// writer calls it so the ring it stores is the ring the mixer will read next
// rather than a log of the whole run.
func (g *GS) ResetEvents() {
	if g.cur == 0 {
		return
	}
	g.events = append(g.events[:0], g.events[g.cur:]...)
	g.cur = 0
}

// nest writes a nested component's payload as a length-prefixed blob.
func nest(e *state.Encoder, c state.Component) error {
	sub := state.NewEncoder()
	if err := c.SaveState(sub); err != nil {
		return err
	}
	e.Bytes(sub.Payload())
	return nil
}

var _ state.Component = (*GS)(nil)
