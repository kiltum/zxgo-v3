package sound

import (
	"errors"

	"github.com/kiltum/zxgo-v3/pkg/state"
)

// errPendingQueue is a payload whose queued register writes exceed the chip's
// own bound. A real AY cannot hold that many, so the file is not one a machine
// wrote.
var errPendingQueue = errors.New("ay: more queued register writes than the chip can hold")

// Native state format for the sound devices (STATE_DESIGN.md).
//
// The chips are where "exact" stops being about the visible state and becomes
// about phase: a tone counter one cycle out, or a stored level that skips the
// generator's current position, produces audio that sounds right and drifts.
// So every divider, LFSR and accumulator the generator advances is stored, and
// the values that are pure functions of a register write are stored too, because
// reconstructing them would mean replaying the register file.
//
// # Deferred work, and why the AY is the odd one out
//
// The beeper and the DACs hold a list of {tick, level} events that the mixer
// consumes as it advances; the list up to the mixer's cursor is history and is
// dropped before saving, and what remains is what the restored mixer will
// consume. That is the normalisation.
//
// The AY holds something else: a queue of register writes waiting for their
// tick. It is not history - each entry still has to happen - and it is stamped
// on the same absolute tick clock the container restores, so it is stored as it
// stands rather than flushed. Flushing at save time would apply those writes
// early, moving a write up to one sample window ahead of where the saving
// machine would have placed it.
//
// The tick each component stamps a write with is stored for the same reason.
// It looks transient - step() sets it before every instruction - but a write can
// arrive between instructions (the debugger, the MCP worker's write_port), and
// such a write is stamped with whatever the saved tick was. Restoring a zero
// there puts the write at the start of the timeline, where it is applied a
// window early.

// --- Beeper ---------------------------------------------------------------

// SaveState encodes the speaker's levels and the events the mixer has not
// consumed yet.
func (b *Beeper) SaveState(e *state.Encoder) error {
	e.Bool(b.ear)
	e.Bool(b.mic)
	e.I16(b.resolved)
	e.I64(b.tick)
	e.Count(len(b.events))
	for _, ev := range b.events {
		e.I64(ev.tick)
		e.I16(ev.level)
	}
	return nil
}

// LoadState applies a payload written by SaveState.
func (b *Beeper) LoadState(d *state.Decoder) error {
	var (
		ear, mic bool
		resolved int16
		tick     int64
		events   []levelEvent
	)
	ear = d.Bool()
	mic = d.Bool()
	resolved = d.I16()
	tick = d.I64()
	n := d.Count(10) // one event is an int64 tick plus an int16 level
	if err := d.Err(); err != nil {
		return err
	}
	events = make([]levelEvent, 0, n)
	for i := 0; i < n; i++ {
		ev := levelEvent{tick: d.I64(), level: d.I16()}
		if err := d.Err(); err != nil {
			return err
		}
		events = append(events, ev)
	}

	b.ear, b.mic = ear, mic
	b.resolved = resolved
	b.tick = tick
	b.events = events
	b.cur = 0
	return nil
}

// --- DAC ------------------------------------------------------------------

// SaveState encodes a DAC's resolved level and the events still to come.
func (d0 *DAC) SaveState(e *state.Encoder) error {
	e.I16(d0.resolved)
	e.I64(d0.tick)
	e.Count(len(d0.events))
	for _, ev := range d0.events {
		e.I64(ev.tick)
		e.I16(ev.level)
	}
	return nil
}

// LoadState applies a payload written by SaveState.
func (d0 *DAC) LoadState(d *state.Decoder) error {
	resolved := d.I16()
	tick := d.I64()
	n := d.Count(10)
	if err := d.Err(); err != nil {
		return err
	}
	events := make([]dacEvent, 0, n)
	for i := 0; i < n; i++ {
		ev := dacEvent{tick: d.I64(), level: d.I16()}
		if err := d.Err(); err != nil {
			return err
		}
		events = append(events, ev)
	}

	d0.resolved = resolved
	d0.tick = tick
	d0.events = events
	d0.cur = 0
	return nil
}

// Covox is a DAC plus the port it decodes, which comes from the model.
func (c *Covox) SaveState(e *state.Encoder) error { return c.dac.SaveState(e) }
func (c *Covox) LoadState(d *state.Decoder) error { return c.dac.LoadState(d) }

// --- AY-3-8912 ------------------------------------------------------------

// ayState is everything the AY's generator advances plus its queued writes.
type ayState struct {
	regs     [16]uint8
	selected uint8

	tonePeriod [3]uint32
	toneCount  [3]int64
	toneOut    [3]bool

	noisePeriod uint32
	noiseCount  int64
	noiseReg    uint32
	noiseOut    bool

	envPeriod    uint32
	envCount     int64
	envVolume    uint8
	envShape     uint8
	envHold      bool
	envAlternate bool
	envAttack    bool
	envContinue  bool
	envHolding   bool

	mixer       uint8
	amplitude   [3]uint8
	useEnvelope [3]bool

	chipAcc int64
	tick    int64
	pending []ayWrite
}

// SaveState encodes the AY's registers, generators and pending writes.
//
// level and levelSet are not stored: they are a cache of the current output,
// recomputed from the generators the first time it is asked for.
func (ay *AY8912) SaveState(e *state.Encoder) error {
	// The write timestamp, then the registers they are stamped from.
	e.I64(ay.tick)
	for _, r := range ay.regs {
		e.U8(r)
	}
	e.U8(ay.selected)

	for i := 0; i < 3; i++ {
		e.U32(ay.tonePeriod[i])
		e.I64(ay.toneCount[i])
		e.Bool(ay.toneOut[i])
	}

	e.U32(ay.noisePeriod)
	e.I64(ay.noiseCount)
	e.U32(ay.noiseReg)
	e.Bool(ay.noiseOut)

	e.U32(ay.envPeriod)
	e.I64(ay.envCount)
	e.U8(ay.envVolume)
	e.U8(ay.envShape)
	e.Bool(ay.envHold)
	e.Bool(ay.envAlternate)
	e.Bool(ay.envAttack)
	e.Bool(ay.envContinue)
	e.Bool(ay.envHolding)

	e.U8(ay.mixer)
	for i := 0; i < 3; i++ {
		e.U8(ay.amplitude[i])
		e.Bool(ay.useEnvelope[i])
	}

	// The tick-to-chip-cycle remainder: dropped, the chip would restart its
	// phase mid-window and the pitch would wobble by a cycle per call.
	e.I64(ay.chipAcc)

	// Queued register writes, with the tick each one is waiting for.
	e.Count(len(ay.pending))
	for _, w := range ay.pending {
		e.I64(w.tick)
		e.U8(w.reg)
		e.U8(w.val)
	}
	return nil
}

// LoadState applies a payload written by SaveState.
func (ay *AY8912) LoadState(d *state.Decoder) error {
	var s ayState

	s.tick = d.I64()
	for i := range s.regs {
		s.regs[i] = d.U8()
	}
	s.selected = d.U8()

	for i := 0; i < 3; i++ {
		s.tonePeriod[i] = d.U32()
		s.toneCount[i] = d.I64()
		s.toneOut[i] = d.Bool()
	}

	s.noisePeriod = d.U32()
	s.noiseCount = d.I64()
	s.noiseReg = d.U32()
	s.noiseOut = d.Bool()

	s.envPeriod = d.U32()
	s.envCount = d.I64()
	s.envVolume = d.U8()
	s.envShape = d.U8()
	s.envHold = d.Bool()
	s.envAlternate = d.Bool()
	s.envAttack = d.Bool()
	s.envContinue = d.Bool()
	s.envHolding = d.Bool()

	s.mixer = d.U8()
	for i := 0; i < 3; i++ {
		s.amplitude[i] = d.U8()
		s.useEnvelope[i] = d.Bool()
	}

	s.chipAcc = d.I64()

	n := d.Count(10) // one write is an int64 tick, a register and a value
	if err := d.Err(); err != nil {
		return err
	}
	// The queue is bounded by the chip's own limit, not by the count in the
	// file: a file claiming a million pending writes is not a file this chip
	// could have written.
	if n > ayMaxPending {
		return errPendingQueue
	}
	s.pending = make([]ayWrite, 0, n)
	for i := 0; i < n; i++ {
		w := ayWrite{tick: d.I64(), reg: d.U8(), val: d.U8()}
		if err := d.Err(); err != nil {
			return err
		}
		s.pending = append(s.pending, w)
	}

	ay.tick = s.tick
	ay.regs = s.regs
	ay.selected = s.selected
	ay.tonePeriod = s.tonePeriod
	ay.toneCount = s.toneCount
	ay.toneOut = s.toneOut
	ay.noisePeriod = s.noisePeriod
	ay.noiseCount = s.noiseCount
	ay.noiseReg = s.noiseReg
	ay.noiseOut = s.noiseOut
	ay.envPeriod = s.envPeriod
	ay.envCount = s.envCount
	ay.envVolume = s.envVolume
	ay.envShape = s.envShape
	ay.envHold = s.envHold
	ay.envAlternate = s.envAlternate
	ay.envAttack = s.envAttack
	ay.envContinue = s.envContinue
	ay.envHolding = s.envHolding
	ay.mixer = s.mixer
	ay.amplitude = s.amplitude
	ay.useEnvelope = s.useEnvelope
	ay.chipAcc = s.chipAcc
	ay.pending = s.pending

	// The output cache is derived; drop it so the next read recomputes it from
	// the generators that were just restored.
	ay.levelSet = false
	ay.level = 0
	return nil
}

// --- TurboSound -----------------------------------------------------------

// SaveState encodes both chips and which one the select port last chose.
func (t *TurboSound) SaveState(e *state.Encoder) error {
	e.Bool(t.current == t.ay2)
	if err := nest(e, t.ay1); err != nil {
		return err
	}
	return nest(e, t.ay2)
}

// LoadState applies a payload written by SaveState.
func (t *TurboSound) LoadState(d *state.Decoder) error {
	second := d.Bool()
	blob1, blob2 := d.Bytes(), d.Bytes()
	if err := d.Err(); err != nil {
		return err
	}
	if err := t.ay1.LoadState(state.NewDecoder(blob1, d.Version())); err != nil {
		return err
	}
	if err := t.ay2.LoadState(state.NewDecoder(blob2, d.Version())); err != nil {
		return err
	}
	if second {
		t.current = t.ay2
	} else {
		t.current = t.ay1
	}
	return nil
}

// --- TurboSound FM (YM2203 pair) ------------------------------------------

// SaveState encodes both FM chips, which one is selected, and the ready-flag
// poll state of the pseudo-register.
func (t *TSFM) SaveState(e *state.Encoder) error {
	e.Bool(t.current == t.chip1)
	e.Bool(t.readiness)
	if err := nest(e, t.chip0); err != nil {
		return err
	}
	return nest(e, t.chip1)
}

// LoadState applies a payload written by SaveState.
func (t *TSFM) LoadState(d *state.Decoder) error {
	second := d.Bool()
	readiness := d.Bool()
	blob0, blob1 := d.Bytes(), d.Bytes()
	if err := d.Err(); err != nil {
		return err
	}
	if err := t.chip0.LoadState(state.NewDecoder(blob0, d.Version())); err != nil {
		return err
	}
	if err := t.chip1.LoadState(state.NewDecoder(blob1, d.Version())); err != nil {
		return err
	}
	if second {
		t.current = t.chip1
	} else {
		t.current = t.chip0
	}
	t.readiness = readiness
	return nil
}

// --- YM2203 ---------------------------------------------------------------

// SaveState encodes the FM chip: the SSG it embeds, the twelve operators, the
// timers and the prescaler.
//
// egTimerAdd and timerAdd are not stored: they are derived from the chip clock
// and the sample rate, and the machine a state is restored into was built with
// the same pair, so the values it already holds are the right ones.
func (y *YM2203) SaveState(e *state.Encoder) error {
	if err := nest(e, y.ay); err != nil {
		return err
	}
	e.U32(y.egCnt)
	e.I64(y.egTimer)
	e.I64(y.curTick)
	e.U8(y.regAddr)
	e.U8(y.prescalerSel)
	e.U8(y.mode)
	for _, f := range y.ch3fc {
		e.U32(f)
	}
	e.U32(y.ta)
	e.U32(y.tb)
	e.I64(y.tac)
	e.I64(y.tbc)
	e.U8(y.timerFlags)
	e.I64(y.timerAcc)
	e.I64(y.fracAcc)
	e.I32(y.lastOut)
	e.I64(y.busyUntil)
	for i := range y.ch {
		saveYMChannel(e, &y.ch[i])
	}
	return nil
}

// LoadState applies a payload written by SaveState.
func (y *YM2203) LoadState(d *state.Decoder) error {
	blob := d.Bytes()
	if err := d.Err(); err != nil {
		return err
	}
	var s struct {
		egCnt        uint32
		egTimer      int64
		curTick      int64
		regAddr      uint8
		prescalerSel uint8
		mode         uint8
		ch3fc        [3]uint32
		ta, tb       uint32
		tac, tbc     int64
		timerFlags   uint8
		timerAcc     int64
		fracAcc      int64
		lastOut      int32
		busyUntil    int64
		ch           [3]ymChannel
	}
	s.egCnt = d.U32()
	s.egTimer = d.I64()
	s.curTick = d.I64()
	s.regAddr = d.U8()
	s.prescalerSel = d.U8()
	s.mode = d.U8()
	for i := range s.ch3fc {
		s.ch3fc[i] = d.U32()
	}
	s.ta = d.U32()
	s.tb = d.U32()
	s.tac = d.I64()
	s.tbc = d.I64()
	s.timerFlags = d.U8()
	s.timerAcc = d.I64()
	s.fracAcc = d.I64()
	s.lastOut = d.I32()
	s.busyUntil = d.I64()
	for i := range s.ch {
		loadYMChannel(d, &s.ch[i])
	}
	if err := d.Err(); err != nil {
		return err
	}

	// The SSG goes in first: if it refuses, nothing else has been touched.
	if err := y.ay.LoadState(state.NewDecoder(blob, d.Version())); err != nil {
		return err
	}
	y.egCnt = s.egCnt
	y.egTimer = s.egTimer
	y.curTick = s.curTick
	y.regAddr = s.regAddr
	y.prescalerSel = s.prescalerSel
	y.mode = s.mode
	y.ch3fc = s.ch3fc
	y.ta, y.tb = s.ta, s.tb
	y.tac, y.tbc = s.tac, s.tbc
	y.timerFlags = s.timerFlags
	y.timerAcc = s.timerAcc
	y.fracAcc = s.fracAcc
	y.lastOut = s.lastOut
	y.busyUntil = s.busyUntil
	y.ch = s.ch
	return nil
}

// saveYMChannel and loadYMChannel carry one channel's four operators plus the
// algorithm graph's memo of the previous sample, which feedback reads.
func saveYMChannel(e *state.Encoder, c *ymChannel) {
	for i := range c.slots {
		saveYMSlot(e, &c.slots[i])
	}
	e.U8(c.algo)
	e.U8(c.fb)
	e.U32(c.fc)
	e.I32(c.op1Out[0])
	e.I32(c.op1Out[1])
	e.I32(c.memVal)
	e.I64(int64(c.connect1))
	e.I64(int64(c.connect2))
	e.I64(int64(c.connect3))
	e.I64(int64(c.connect4))
	e.I64(int64(c.memConnect))
}

func loadYMChannel(d *state.Decoder, c *ymChannel) {
	for i := range c.slots {
		loadYMSlot(d, &c.slots[i])
	}
	c.algo = d.U8()
	c.fb = d.U8()
	c.fc = d.U32()
	c.op1Out[0] = d.I32()
	c.op1Out[1] = d.I32()
	c.memVal = d.I32()
	c.connect1 = int(d.I64())
	c.connect2 = int(d.I64())
	c.connect3 = int(d.I64())
	c.connect4 = int(d.I64())
	c.memConnect = int(d.I64())
}

// saveYMSlot writes one operator: its phase and increment, its key-scale and
// envelope-rate tables, the envelope generator's state and volume, and the SSG
// mode's latches. The rate-table entries are decoded from registers rather than
// recomputed here, because recomputing them would mean replaying register writes
// in the right order.
func saveYMSlot(e *state.Encoder, s *ymSlot) {
	e.U32(s.phase)
	e.U32(s.incr)
	e.I32(s.dt)
	e.U8(s.dtIdx)
	e.U8(s.KSR)
	e.U8(s.ksr)
	e.U32(s.ar)
	e.U32(s.d1r)
	e.U32(s.d2r)
	e.U32(s.rr)
	e.U32(s.mul)
	e.U32(s.tl)
	e.U32(s.sl)
	e.U8(s.ssg)
	e.U8(s.ssgn)
	e.I64(int64(s.state))
	e.I32(s.volume)
	e.Bool(s.key)
	e.U8(s.egShAr)
	e.U8(s.egSelAr)
	e.U8(s.egShD1r)
	e.U8(s.egSelD1r)
	e.U8(s.egShD2r)
	e.U8(s.egSelD2r)
	e.U8(s.egShRr)
	e.U8(s.egSelRr)
}

func loadYMSlot(d *state.Decoder, s *ymSlot) {
	s.phase = d.U32()
	s.incr = d.U32()
	s.dt = d.I32()
	s.dtIdx = d.U8()
	s.KSR = d.U8()
	s.ksr = d.U8()
	s.ar = d.U32()
	s.d1r = d.U32()
	s.d2r = d.U32()
	s.rr = d.U32()
	s.mul = d.U32()
	s.tl = d.U32()
	s.sl = d.U32()
	s.ssg = d.U8()
	s.ssgn = d.U8()
	s.state = int(d.I64())
	s.volume = d.I32()
	s.key = d.Bool()
	s.egShAr = d.U8()
	s.egSelAr = d.U8()
	s.egShD1r = d.U8()
	s.egSelD1r = d.U8()
	s.egShD2r = d.U8()
	s.egSelD2r = d.U8()
	s.egShRr = d.U8()
	s.egSelRr = d.U8()
}

// nest writes one component's payload as a length-prefixed blob, so a device
// made of other devices can carry them without the reader having to know how
// long each one is.
func nest(e *state.Encoder, c state.Component) error {
	sub := state.NewEncoder()
	if err := c.SaveState(sub); err != nil {
		return err
	}
	e.Bytes(sub.Payload())
	return nil
}

var _ state.Component = (*Beeper)(nil)
var _ state.Component = (*DAC)(nil)
var _ state.Component = (*AY8912)(nil)
var _ state.Component = (*TurboSound)(nil)
var _ state.Component = (*YM2203)(nil)
var _ state.Component = (*TSFM)(nil)
var _ state.Component = (*Covox)(nil)
