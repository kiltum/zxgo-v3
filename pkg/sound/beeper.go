package sound

// Beeper models the ZX Spectrum's 1-bit speaker, driven by bits 3 (MIC) and
// 4 (EAR) of port 0xFE.
//
// It records level *events* against the CPU tick clock and generates no samples
// itself -- the mixer resolves those events onto its sample grid. That split is
// what makes total sample count a pure function of elapsed T-states (AUDIT S1).
// The previous design generated samples per transition, rounding each delta
// independently: it drifted -20% at 17.5 kHz and fell to total silence below
// ~40 T-states per toggle, where every delta rounded to zero samples.
//
// Implements io_ports.PortHandler (port 0xFE) and sound.Source.
type Beeper struct {
	events []levelEvent
	cur    int   // mixer read cursor into events
	tick   int64 // current CPU tick, set by the emulator before each instruction

	ear, mic bool
	resolved int16 // level in effect at the mixer's cursor
}

type levelEvent struct {
	tick  int64
	level int16
}

// EAR dominates the speaker; MIC contributes a smaller amount, as on real
// hardware. Total swing matches the previous implementation's amplitude.
const (
	earAmp = 9000
	micAmp = 1000
)

func beeperLevel(ear, mic bool) int16 {
	var v int32
	if ear {
		v += earAmp
	} else {
		v -= earAmp
	}
	if mic {
		v += micAmp
	} else {
		v -= micAmp
	}
	return int16(v)
}

// NewBeeper creates a beeper at rest. It has no audio output of its own --
// register it with a Mixer via AddSource.
//
// Rest is ear=false, mic=true: the ULA port latch holds 0x00 after reset, and
// MIC is active low, so bit 3 clear means MIC asserted. Getting this wrong
// makes a program's first write of 0x00 look like a transition.
func NewBeeper() *Beeper {
	return &Beeper{mic: true, resolved: beeperLevel(false, true)}
}

// SetTick tells the beeper the current CPU tick. The emulator calls this before
// each instruction, so an OUT during that instruction is timestamped at the
// instruction's start. Sub-instruction placement needs the per-M-cycle hook
// (AUDIT D1/T6).
func (b *Beeper) SetTick(t int64) { b.tick = t }

// --- io_ports.PortHandler ---

func (b *Beeper) HandlesPort(port uint16) bool { return (port & 0xFF) == 0xFE }

// Read returns 0xFF: several handlers claim port 0xFE, and the port bus ANDs
// their results, so the beeper must not pull down the ULA's keyboard bits.
func (b *Beeper) Read(port uint16) uint8 { return 0xFF }

// ReadAttached implements io_ports.AttachedHandler. The beeper only listens on
// port 0xFE; it drives nothing, so it reports an empty mask and lets the ULA's
// keyboard/EAR bits and the floating bus through the open-collector AND.
func (b *Beeper) ReadAttached(port uint16) (uint8, uint8) { return 0xFF, 0x00 }

// Write records a level change. Writes that don't move either bit are ignored,
// which keeps the event list proportional to actual transitions.
func (b *Beeper) Write(port uint16, value uint8) {
	ear := (value & 0x10) != 0
	mic := (value & 0x08) == 0 // MIC is active low
	if ear == b.ear && mic == b.mic {
		return
	}
	b.ear, b.mic = ear, mic

	t := b.tick
	if n := len(b.events); n > 0 && b.events[n-1].tick > t {
		t = b.events[n-1].tick // events must not go backwards
	}
	b.events = append(b.events, levelEvent{tick: t, level: beeperLevel(ear, mic)})
}

// --- sound.Source ---

// MeanLevel returns the time-weighted average level over [from, to).
//
// Averaging rather than point-sampling is what captures transitions that fall
// between two sample points -- the anti-aliasing in this design is correct
// resampling, not a tone filter.
//
// The mixer calls this with strictly increasing, non-overlapping windows, so
// consumed events are discarded as the cursor advances.
func (b *Beeper) MeanLevel(from, to int64) int16 {
	if to <= from {
		return b.resolved
	}

	for b.cur < len(b.events) && b.events[b.cur].tick <= from {
		b.resolved = b.events[b.cur].level
		b.cur++
	}

	var acc int64
	t := from
	for b.cur < len(b.events) && b.events[b.cur].tick < to {
		e := b.events[b.cur]
		acc += int64(b.resolved) * (e.tick - t)
		t = e.tick
		b.resolved = e.level
		b.cur++
	}
	acc += int64(b.resolved) * (to - t)

	// Compact consumed events. Only resetting when the list is fully drained
	// would let it grow for the lifetime of the session, since a sample window
	// routinely ends part-way through the queue.
	if b.cur > 0 {
		b.events = append(b.events[:0], b.events[b.cur:]...)
		b.cur = 0
	}
	return int16(acc / (to - from))
}

// Reset returns the beeper to its rest state and drops queued events.
func (b *Beeper) Reset() {
	b.events = b.events[:0]
	b.cur = 0
	b.tick = 0
	b.ear, b.mic = false, true // see NewBeeper: 0x00 latch, MIC active low
	b.resolved = beeperLevel(false, true)
}

// Pending reports queued, unresolved events -- diagnostics only.
func (b *Beeper) Pending() int { return len(b.events) - b.cur }

// ResetEvents clears the event list after the mixer has consumed them.
// Called by pumpAudio to prevent unbounded event accumulation.
func (b *Beeper) ResetEvents() {
	if b.cur > 0 {
		b.events = append(b.events[:0], b.events[b.cur:]...)
		b.cur = 0
	}
}

var _ Source = (*Beeper)(nil)
