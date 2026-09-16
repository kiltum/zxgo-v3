// Package gs implements the General Sound (GS) card for the ZX Spectrum: a
// second Z80 at 12 MHz with its own 32 KB ROM and 512 KB RAM, a 4-channel DAC,
// and a command protocol over host ports 0xBB (command/status) and 0xB3
// (data/output). The GS CPU runs independently of the Spectrum CPU; the only
// shared surface is the two handshake ports.
//
// The reference is ZEsarUX soundchips/gs.c (itself based on Xpeccy).
package gs

import (
	"fmt"

	"github.com/kiltum/zxgo-v3/pkg/bus"
	"github.com/kiltum/zxgo-v3/pkg/cpu"
	"github.com/kiltum/zxgo-v3/pkg/io_ports"
	"github.com/kiltum/zxgo-v3/pkg/sound"
)

const (
	romSize   = 32768  // 32 KB ROM (gs105a.rom)
	ramSize   = 524288 // 512 KB RAM
	blockSize = 16384  // 16 KB block (the paging granularity)
	ramBlocks = ramSize / blockSize

	// pageMask selects the 32 KB page in the upper 0x8000-0xFFFF window: page 0
	// is ROM, pages 1..15 are RAM (ZEsarUX gs_memory_mapping_mask_pages).
	pageMask = 15

	// gsInterruptPeriod is the GS Z80 T-states between maskable interrupts.
	gsInterruptPeriod = 334

	// gsClockHz is the GS CPU clock.
	gsClockHz = 12000000
)

// GS is a General Sound card.
type GS struct {
	cpu *cpu.Z80
	mem []byte // romSize ROM + ramSize RAM

	mapping uint8 // memory-mapping page (GS port 0x00 write)

	command, data, state, output uint8 // host <-> GS handshake registers

	dac [4]uint8 // volume-applied DAC channel levels (unsigned, centre 128)
	vol [4]uint8 // channel volumes (0..63)

	ratio        float64 // GS clock / Spectrum clock
	gsTicks      int64   // GS T-states elapsed since reset
	interruptCnt int64

	// DAC output as timestamped events, so the mixer can place each DAC change at
	// its exact GS T-state (a plain "current level" would sample-hold at frame
	// granularity and turn the music into noise). cur is the head index of the
	// first unconsumed event; consumed events are reclaimed lazily in recordMix
	// (not on every sample, which was O(n^2)).
	events       []gsEvent
	cur          int
	resolvedLeft  int16
	resolvedRight int16
}

// gsEvent is a DAC level change at a given GS T-state.
type gsEvent struct {
	gsTick int64
	left   int16
	right  int16
}

// New creates a GS card clocked relative to the given Spectrum CPU Hz.
func New(specHz int) *GS {
	g := &GS{
		mem:   make([]byte, romSize+ramSize),
		ratio: float64(gsClockHz) / float64(specHz),
		state: 0x7E,
	}
	g.cpu = cpu.New(g)
	g.cpu.SetCPUType(true) // NMOS
	return g
}

// LoadROM loads the 32 KB GS ROM image.
func (g *GS) LoadROM(data []byte) error {
	if len(data) != romSize {
		return fmt.Errorf("GS ROM must be %d bytes, got %d", romSize, len(data))
	}
	copy(g.mem[:romSize], data)
	return nil
}

// CPU returns the GS Z80 (debugger access).
func (g *GS) CPU() *cpu.Z80 { return g.cpu }

// Reset returns the GS to its power-on state.
func (g *GS) Reset() {
	g.cpu = cpu.New(g)
	g.cpu.SetCPUType(true)
	g.mapping = 0
	g.command, g.data, g.output = 0, 0, 0
	g.state = 0x7E
	g.dac = [4]uint8{}
	g.vol = [4]uint8{}
	g.gsTicks = 0
	g.interruptCnt = 0
	g.events = g.events[:0]
	g.cur = 0
	g.resolvedLeft, g.resolvedRight = 0, 0
}

// --- bus.BusReaderWriter (the GS Z80's view of memory and I/O) ---

// segInfo returns the memory offset and writability of a 16 KB segment.
func (g *GS) segInfo(seg int) (base int, writable bool) {
	switch seg {
	case 0: // 0x0000-0x3FFF: ROM block 0 (fixed)
		return 0, false
	case 1: // 0x4000-0x7FFF: fixed RAM block (the sample buffer)
		return romSize + pageMask*blockSize, true
	case 2, 3: // 0x8000-0xFFFF: page-selected 32 KB window
		v := int(g.mapping) & pageMask
		if v == 0 {
			return (seg - 2) * blockSize, false // ROM blocks 0 and 1
		}
		block := 2*(v-1) + (seg - 2)
		return romSize + block*blockSize, true
	}
	return 0, false
}

func (g *GS) ReadMemory(addr uint16) uint8 {
	seg := int(addr) / blockSize
	base, _ := g.segInfo(seg)
	v := g.mem[base+int(addr)%blockSize]

	// Reading RAM at 0x6000-0x7FFF feeds the DAC channel selected by A8-A9.
	if addr >= 0x6000 && addr <= 0x7FFF {
		g.dacWrite(int((addr>>8)&3), v)
	}
	return v
}

func (g *GS) FetchOpcode(addr uint16) uint8 { return g.ReadMemory(addr) }

func (g *GS) WriteMemory(addr uint16, value uint8) {
	seg := int(addr) / blockSize
	base, writable := g.segInfo(seg)
	if !writable {
		return // ROM is read-only
	}
	g.mem[base+int(addr)%blockSize] = value
}

func (g *GS) ReadIO(port uint16) uint8  { return g.gsReadPort(byte(port) & 0x0F) }
func (g *GS) WriteIO(port uint16, value uint8) { g.gsWritePort(byte(port)&0x0F, value) }

// gsReadPort handles the GS Z80's own I/O ports (low 4 bits).
func (g *GS) gsReadPort(p uint8) uint8 {
	switch p {
	case 1:
		return g.command
	case 2:
		g.state &= 0x7F
		return g.data
	case 3:
		g.state |= 0x80
	case 4:
		return g.state
	case 5:
		g.state &= 0xFE
	case 10:
		if g.mapping&0x01 != 0 {
			g.state &= 0x7F
		} else {
			g.state |= 0x80
		}
	case 11:
		if g.vol[0]&0x20 != 0 {
			g.state |= 1
		} else {
			g.state &= 0xFE
		}
	}
	return 0xFF
}

// gsWritePort handles the GS Z80's own I/O ports (low 4 bits).
func (g *GS) gsWritePort(p uint8, value uint8) {
	switch p {
	case 0:
		g.mapping = value
	case 2:
		g.state &= 0x7F
	case 3:
		g.state |= 0x80
		g.output = value
	case 5:
		g.state &= 0xFE
	case 6:
		g.vol[0] = value & 0x3F
	case 7:
		g.vol[1] = value & 0x3F
	case 8:
		g.vol[2] = value & 0x3F
	case 9:
		g.vol[3] = value & 0x3F
	case 10:
		if g.mapping&0x01 != 0 {
			g.state &= 0x7F
		} else {
			g.state |= 0x80
		}
	case 11:
		if g.vol[0]&0x20 != 0 {
			g.state |= 1
		} else {
			g.state &= 0x7F
		}
	}
}

// dacWrite applies the channel volume and stores a DAC sample.
func (g *GS) dacWrite(ch int, value uint8) {
	signed := int(value) - 128
	vol := int(g.vol[ch]) & 0x3F
	g.dac[ch] = uint8(128 + (signed*vol)/0x3F)
	g.recordMix()
}

// recordMix appends a DAC level-change event at the current GS T-state.
func (g *GS) recordMix() {
	left, right := g.dacMix()
	if n := len(g.events); n > 0 {
		last := &g.events[n-1]
		if last.left == left && last.right == right {
			return // unchanged since the last event
		}
	}
	// Reclaim consumed events lazily: only compact once over half the buffer is
	// spent. Compacting on every audio sample was O(n^2) and killed the whole
	// emulator at the GS's ~144 kHz sample rate (4 channels x 35.9 kHz).
	if g.cur > 0 && g.cur*2 >= len(g.events) {
		g.events = append(g.events[:0], g.events[g.cur:]...)
		g.cur = 0
	}
	g.events = append(g.events, gsEvent{gsTick: g.gsTicks, left: left, right: right})
}

// --- io_ports.PortHandler (the host Spectrum's view: 0xBB and 0xB3) ---

func (g *GS) HandlesPort(port uint16) bool {
	p := byte(port)
	return p == 0xBB || p == 0xB3
}

func (g *GS) Read(port uint16) uint8 {
	switch byte(port) {
	case 0xBB:
		return g.state
	case 0xB3:
		g.state &= 0x7F
		return g.output
	}
	return 0xFF
}

func (g *GS) Write(port uint16, value uint8) {
	switch byte(port) {
	case 0xBB:
		g.command = value
		g.state |= 1
	case 0xB3:
		g.data = value
		g.state |= 0x80
	}
}

// --- clocking ---

// Tick advances the GS Z80 in lockstep with the Spectrum CPU: for deltaMainTicks
// Spectrum T-states, the GS runs deltaMainTicks * (12MHz/SpectrumHz) GS
// T-states. The maskable interrupt fires every 334 GS T-states when IFF1 is set.
func (g *GS) Tick(deltaMainTicks int64) {
	if deltaMainTicks <= 0 {
		return
	}
	target := g.gsTicks + int64(float64(deltaMainTicks)*g.ratio)
	for g.gsTicks < target {
		if g.gsTicks/gsInterruptPeriod > g.interruptCnt {
			g.interruptCnt = g.gsTicks / gsInterruptPeriod
			if g.cpu.IFF1 {
				g.cpu.SetInterruptLevel(true)
			}
		}
		g.gsTicks += int64(g.cpu.ExecuteOneInstruction())
	}
}

// --- sound.Source ---

// dacMix returns the stereo DAC output, signed and scaled to int16.
func (g *GS) dacMix() (int16, int16) {
	left := (int(g.dac[0]) + int(g.dac[1])) / 2
	right := (int(g.dac[2]) + int(g.dac[3])) / 2
	return int16((left - 128) * 128), int16((right - 128) * 128)
}

// MeanLevel returns the mono downmix.
func (g *GS) MeanLevel(from, to int64) int16 {
	l, r := g.MeanLevelStereo(from, to)
	return int16((int32(l) + int32(r)) / 2)
}

// MeanLevelStereo returns the time-weighted average DAC output over [from, to),
// where from/to are Spectrum T-states. Each DAC change is placed at its exact
// GS T-state (converted back to the Spectrum grid), so the resampled output is
// the correct mean level rather than a frame-granularity sample-and-hold.
func (g *GS) MeanLevelStereo(from, to int64) (int16, int16) {
	fromGS := int64(float64(from) * g.ratio)
	toGS := int64(float64(to) * g.ratio)
	if toGS <= fromGS {
		return g.resolvedLeft, g.resolvedRight
	}

	for g.cur < len(g.events) && g.events[g.cur].gsTick <= fromGS {
		g.resolvedLeft = g.events[g.cur].left
		g.resolvedRight = g.events[g.cur].right
		g.cur++
	}

	var accL, accR int64
	t := fromGS
	for g.cur < len(g.events) && g.events[g.cur].gsTick < toGS {
		e := g.events[g.cur]
		accL += int64(g.resolvedLeft) * (e.gsTick - t)
		accR += int64(g.resolvedRight) * (e.gsTick - t)
		t = e.gsTick
		g.resolvedLeft = e.left
		g.resolvedRight = e.right
		g.cur++
	}
	accL += int64(g.resolvedLeft) * (toGS - t)
	accR += int64(g.resolvedRight) * (toGS - t)
	return int16(accL / (toGS - fromGS)), int16(accR / (toGS - fromGS))
}

var _ bus.BusReaderWriter = (*GS)(nil)
var _ io_ports.PortHandler = (*GS)(nil)
var _ sound.Source = (*GS)(nil)
var _ sound.StereoSource = (*GS)(nil)
