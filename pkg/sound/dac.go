package sound

// DAC is a generic digital-to-analog converter source. Its output level is
// whatever was last written, and it records {tick, level} events so the mixer
// places writes at sub-sample resolution -- the same contract the Beeper uses.
// Covox and SounDrive are thin PortHandler wrappers around one or more DACs.
//
// A DAC at rest outputs 0 (silence), so an unwritten DAC contributes nothing to
// the mix; this is why it is safe to register one unconditionally.
type DAC struct {
	events   []dacEvent
	cur      int   // mixer read cursor into events
	tick     int64 // current CPU tick, set by the emulator before each instruction
	resolved int16 // level in effect at the mixer's cursor
}

type dacEvent struct {
	tick  int64
	level int16
}

// NewDAC creates a DAC at rest (output 0).
func NewDAC() *DAC { return &DAC{resolved: 0} }

// SetTick tells the DAC the current CPU tick, so a write during the next
// instruction is timestamped at that instruction's start (same as Beeper.SetTick).
func (d *DAC) SetTick(t int64) { d.tick = t }

// SetLevel records a new output level at the current tick. Writes that do not
// change the level are ignored, keeping the event list proportional to changes.
func (d *DAC) SetLevel(level int16) {
	t := d.tick
	if n := len(d.events); n > 0 && d.events[n-1].tick > t {
		t = d.events[n-1].tick // events must not go backwards
	}
	if n := len(d.events); n > 0 && d.events[n-1].level == level {
		return // unchanged
	}
	d.events = append(d.events, dacEvent{tick: t, level: level})
}

// MeanLevel returns the time-weighted average output over [from, to).
//
// Averaging rather than point-sampling places a write that lands between two
// sample points correctly, the same anti-aliasing contract as the Beeper. The
// mixer calls this with strictly increasing windows, so consumed events are
// discarded as the cursor advances.
func (d *DAC) MeanLevel(from, to int64) int16 {
	if to <= from {
		return d.resolved
	}

	for d.cur < len(d.events) && d.events[d.cur].tick <= from {
		d.resolved = d.events[d.cur].level
		d.cur++
	}

	var acc int64
	t := from
	for d.cur < len(d.events) && d.events[d.cur].tick < to {
		e := d.events[d.cur]
		acc += int64(d.resolved) * (e.tick - t)
		t = e.tick
		d.resolved = e.level
		d.cur++
	}
	acc += int64(d.resolved) * (to - t)

	if d.cur > 0 {
		d.events = append(d.events[:0], d.events[d.cur:]...)
		d.cur = 0
	}
	return int16(acc / (to - from))
}

// Reset clears the DAC to silence and drops queued events.
func (d *DAC) Reset() {
	d.events = d.events[:0]
	d.cur = 0
	d.tick = 0
	d.resolved = 0
}

// ResetEvents clears the event list after the mixer has consumed them, so a
// burst of writes between Resolve calls cannot accumulate unboundedly.
func (d *DAC) ResetEvents() {
	if d.cur > 0 {
		d.events = append(d.events[:0], d.events[d.cur:]...)
		d.cur = 0
	}
}

var _ Source = (*DAC)(nil)
