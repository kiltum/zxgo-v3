package sound

import (
	"github.com/kiltum/zxgo-v3/pkg/io_ports"
)

// AY8912 emulates the AY-3-8912 PSG fitted to the 128K / +2 / +3 Spectrums.
//
// # Timing model, and why it is not the one this file used to have
//
// Every divider in this chip is counted in *chip cycles* (AY clock = CPU/2).
// The generator is advanced by MeanLevel, which the mixer calls once per audio
// sample with the tick window that sample covers -- the same contract Beeper
// uses. Nothing outside this file ticks the chip.
//
// The previous implementation was a port of vgm_decoder's AY38910::getSample().
// That reference is correct, but it is a *per-audio-sample* generator: its step
// size is 2*fchip/fsample, and zxcpp calls it once per output sample from its
// audio thread. The port kept the step size but called it once per CPU T-state,
// so every counter advanced ~80x too fast -- a 110 Hz note came out at 8.8 kHz.
// Pitch, noise colour and envelope rate were all wrong by that factor, which is
// why tunes kept their tempo (register writes still landed on the right frame)
// but arrived on unrecognisable instruments. It also appended one level event
// per T-state -- 70908 per frame -- and MeanLevel rescanned the whole list for
// every sample, which made the audio path quadratic in frame length.
//
// # Divider ratios (AY-3-8910 datasheet, vgm_decoder/docs)
//
//	tone     section 3.1   f = fchip / (16 * TP)  -> output toggles every   8*TP cycles
//	noise    section 3.2   f = fchip / (16 * NP)  -> LFSR shifts every      16*NP cycles
//	envelope section 3.5.1 f = fchip / (256 * EP) -> one step every        256*EP cycles
//	         section 3.5.2 16 states per cycle, so a full shape is 4096*EP cycles
//
// The datasheet gives 1 as the lowest legal period ("divide by 1"), so a zero
// period register is read as 1 rather than freezing the generator.
//
// Implements io_ports.PortHandler (0xFFFD / 0xBFFD) and sound.Source.
type AY8912 struct {
	regs     [16]uint8 // shadow copy, updated synchronously so I/O reads are live
	selected uint8     // latched register address; persists across data writes

	chipFreq uint32 // AY clock in Hz (1.7734 MHz on a 128K)
	cpuFreq  uint32 // CPU clock in Hz, for the tick -> chip-cycle conversion

	// Divider state. Each counter counts *down* the cycles left until its next
	// event, so a window can be advanced in one jump per event rather than one
	// step per chip cycle.
	tonePeriod [3]uint32 // TP, 12 bits, 0 read as 1
	toneCount  [3]int64
	toneOut    [3]bool

	noisePeriod uint32 // NP, 5 bits, 0 read as 1
	noiseCount  int64
	noiseReg    uint32 // 17-bit LFSR
	noiseOut    bool

	envPeriod uint32 // EP, 16 bits, 0 read as 1
	envCount  int64
	envVolume uint8
	envShape  uint8
	envHold      bool
	envAlternate bool
	envAttack    bool
	envContinue  bool
	envHolding   bool

	mixer       uint8 // R7: a set bit *disables* that generator on that channel
	amplitude   [3]uint8
	useEnvelope [3]bool

	levelTable [16]int32
	userVolume uint16

	// Register writes are timestamped and applied as the generator reaches
	// them, so a write lands in the sample it actually happened in rather than
	// retroactively colouring a whole frame or slice.
	pending  []ayWrite
	tick     int64 // current CPU tick, set by the emulator before each instruction
	chipAcc  int64 // tick->cycle conversion remainder; keeps the ratio exact
	level    int32 // cached output, recomputed only when a generator changes
	levelSet bool
}

// ayWrite is a register write waiting for the generator to reach its tick.
type ayWrite struct {
	tick int64
	reg  uint8
	val  uint8
}

// Hardware-measured AY-3-8912 amplitude steps (vgm_decoder, from the
// normalized-voltage table in the datasheet). Only the *ratios* matter here;
// ayFullScale sets the absolute level.
var ay8910LevelTable = [16]uint16{
	0, 183, 408, 683, 1020, 1433, 1939, 2558,
	3317, 4246, 5385, 6779, 8487, 10579, 13142, 16281,
}

const (
	// ayFullScale is the output with all three channels at maximum volume.
	//
	// The AY's three DACs sum into a single unipolar output, so the loudest
	// possible signal is 3x one channel's peak. Sizing *that* -- rather than one
	// channel -- is what keeps chords from clipping: the old table put a single
	// channel at 30526, so any two channels above about volume 12 saturated
	// int16 and every chord came out hard-clipped. Headroom below int16 max is
	// left for the beeper (+/-10000), which shares the mixer's accumulator.
	ayFullScale = 20000

	// ayLevelSum is sum(ay8910LevelTable) at full volume on all three channels.
	ayLevelSum = 3 * 16281

	// ayMaxPending bounds the deferred-write queue. Reached only when nothing
	// is draining it (turbo mode), where Flush applies the backlog instead.
	ayMaxPending = 4096
)

// NewAY8912 creates an AY-3-8912 clocked for a 128K Spectrum.
func NewAY8912() *AY8912 {
	ay := &AY8912{
		chipFreq:   1773450, // 3546900 / 2
		cpuFreq:    3546900,
		userVolume: 100,
		pending:    make([]ayWrite, 0, 256),
	}
	ay.calcVolumeTables()
	ay.Reset()
	return ay
}

// calcVolumeTables scales the hardware amplitude ratios to ayFullScale.
func (ay *AY8912) calcVolumeTables() {
	for i := range ay.levelTable {
		v := int64(ay8910LevelTable[i]) * int64(ay.userVolume) * ayFullScale /
			(100 * ayLevelSum)
		ay.levelTable[i] = int32(v)
	}
}

// Reset puts the chip in its power-on state.
func (ay *AY8912) Reset() {
	for i := range ay.regs {
		ay.regs[i] = 0
	}
	ay.selected = 0

	for i := 0; i < 3; i++ {
		ay.tonePeriod[i] = 0
		ay.toneCount[i] = 0
		ay.toneOut[i] = false
		ay.amplitude[i] = 0
		ay.useEnvelope[i] = false
	}

	ay.noisePeriod = 0
	ay.noiseCount = 0
	ay.noiseReg = 1
	ay.noiseOut = false

	ay.envPeriod = 0
	ay.envCount = 0
	ay.envVolume = 0
	ay.envShape = 0
	ay.envHold, ay.envAlternate = false, false
	ay.envAttack, ay.envContinue = false, false
	ay.envHolding = false

	// R7 powers up with every generator disabled (all bits set).
	ay.mixer = 0xFF

	ay.pending = ay.pending[:0]
	ay.tick = 0
	ay.chipAcc = 0
	ay.levelSet = false
	ay.level = 0
}

// SetClockFrequency sets the AY clock, which is half the CPU clock. The CPU
// clock is derived from it, so callers that use a non-standard ratio should
// call SetCPUClock afterwards.
func (ay *AY8912) SetClockFrequency(hz uint32) {
	if hz == 0 {
		return
	}
	ay.chipFreq = hz
	ay.cpuFreq = hz * 2
	ay.chipAcc = 0
}

// SetCPUClock sets the CPU clock the tick timestamps are measured in.
func (ay *AY8912) SetCPUClock(hz uint32) {
	if hz > 0 {
		ay.cpuFreq = hz
		ay.chipAcc = 0
	}
}

// SetVolume scales the output. 100 is the default.
func (ay *AY8912) SetVolume(vol uint16) {
	ay.userVolume = vol
	ay.calcVolumeTables()
	ay.levelSet = false
}

// SetTick tells the chip the current CPU tick, so register writes during the
// next instruction are timestamped at that instruction's start -- the same
// contract Beeper.SetTick has.
func (ay *AY8912) SetTick(t int64) { ay.tick = t }

// --- io_ports.PortHandler ---

// HandlesPort decodes the two AY ports the way the 128K hardware does, on A15,
// A14 and A1: register select is A15=1 A14=1 A1=0 (0xFFFD), data is A15=1
// A14=0 A1=0 (0xBFFD). The memory paging port is A15=0 A14=1 A1=0 (0x7FFD),
// so the three never collide.
func (ay *AY8912) HandlesPort(port uint16) bool {
	if (port & 0x0002) != 0 {
		return false
	}
	sel := port & 0xC000
	return sel == 0xC000 || sel == 0x8000
}

// Read returns the selected register on the select port. The port bus ANDs
// every handler's result, so anything this chip does not drive must read as
// 0xFF rather than 0 -- returning 0 would pull down bits other handlers own.
func (ay *AY8912) Read(port uint16) uint8 {
	if (port&0x0002) == 0 && (port&0xC000) == 0xC000 && ay.selected <= 13 {
		return ay.regs[ay.selected]
	}
	return 0xFF
}

// Write latches a register address or writes data to the latched register.
//
// The address latch persists across data writes, as it does on the chip: a
// player may select R8 once and then write it repeatedly for digi-drums. The
// previous code cleared the latch after each data write, so only the first of
// those writes was accepted.
func (ay *AY8912) Write(port uint16, data uint8) {
	if (port & 0x0002) != 0 {
		return
	}
	switch port & 0xC000 {
	case 0xC000: // 0xFFFD -- register select
		ay.selected = data & 0x0F
	case 0x8000: // 0xBFFD -- register data
		ay.writeRegister(ay.selected, data)
	}
}

// writeRegister updates the shadow register immediately, so an I/O read sees
// it, and queues the synthesis-visible effect at the current tick.
func (ay *AY8912) writeRegister(reg uint8, data uint8) {
	if reg > 13 { // R14/R15 are the I/O ports; the 8912 has no writable ones
		return
	}
	ay.regs[reg] = data

	if len(ay.pending) >= ayMaxPending {
		ay.applyWrite(reg, data) // backstop: nothing is draining the queue
		return
	}
	ay.pending = append(ay.pending, ayWrite{tick: ay.tick, reg: reg, val: data})
}

// ReadRegister returns a shadow register. R14/R15 read as 0xFF (nothing is
// attached to the 8912's single I/O port).
func (ay *AY8912) ReadRegister(reg uint8) uint8 {
	if reg > 13 {
		return 0xFF
	}
	return ay.regs[reg]
}

// LastWriteReg returns the latched register address.
func (ay *AY8912) LastWriteReg() uint8 { return ay.selected }

var _ io_ports.PortHandler = (*AY8912)(nil)

// --- register decode -------------------------------------------------------

// applyWrite moves a register value into the generator state.
func (ay *AY8912) applyWrite(reg, data uint8) {
	switch reg {
	case 0, 2, 4: // tone fine
		ch := reg / 2
		ay.setTonePeriod(int(ch), (ay.tonePeriod[ch]&0x0F00)|uint32(data))
	case 1, 3, 5: // tone coarse
		ch := (reg - 1) / 2
		ay.setTonePeriod(int(ch), (uint32(data&0x0F)<<8)|(ay.tonePeriod[ch]&0x00FF))
	case 6: // noise period
		ay.noisePeriod = uint32(data & 0x1F)
		if r := ay.noiseReload(); ay.noiseCount > r {
			ay.noiseCount = r
		}
	case 7: // mixer
		ay.mixer = data
		ay.levelSet = false
	case 8, 9, 10: // amplitude
		ch := reg - 8
		ay.amplitude[ch] = data & 0x0F
		ay.useEnvelope[ch] = (data & 0x10) != 0
		ay.levelSet = false
	case 11: // envelope fine
		ay.setEnvPeriod((ay.envPeriod & 0xFF00) | uint32(data))
	case 12: // envelope coarse
		ay.setEnvPeriod((uint32(data) << 8) | (ay.envPeriod & 0x00FF))
	case 13: // envelope shape -- writing it restarts the envelope
		ay.envShape = data
		ay.envHold = (data & 0x01) != 0
		ay.envAlternate = (data & 0x02) != 0
		ay.envAttack = (data & 0x04) != 0
		ay.envContinue = (data & 0x08) != 0
		if !ay.envContinue {
			ay.envHold = true // a one-shot shape holds at its end
		}
		ay.envHolding = false
		ay.envVolume = 0
		if !ay.envAttack {
			ay.envVolume = 0x0F
		}
		ay.envCount = ay.envReload()
		ay.levelSet = false
	}
}

// setTonePeriod stores TP and clamps a running countdown that the new, shorter
// period has already overtaken -- otherwise a downward pitch sweep would hold
// the old note until the long countdown expired.
func (ay *AY8912) setTonePeriod(ch int, tp uint32) {
	ay.tonePeriod[ch] = tp & 0x0FFF
	if r := ay.toneReload(ch); ay.toneCount[ch] > r {
		ay.toneCount[ch] = r
	}
}

func (ay *AY8912) setEnvPeriod(ep uint32) {
	ay.envPeriod = ep & 0xFFFF
	if r := ay.envReload(); ay.envCount > r {
		ay.envCount = r
	}
}

// Reload values, in chip cycles, with the datasheet's "lowest period is 1".
func (ay *AY8912) toneReload(ch int) int64 {
	tp := int64(ay.tonePeriod[ch])
	if tp == 0 {
		tp = 1
	}
	return 8 * tp
}

func (ay *AY8912) noiseReload() int64 {
	np := int64(ay.noisePeriod)
	if np == 0 {
		np = 1
	}
	return 16 * np
}

func (ay *AY8912) envReload() int64 {
	ep := int64(ay.envPeriod)
	if ep == 0 {
		ep = 1
	}
	return 256 * ep
}

// --- sound.Source ----------------------------------------------------------

// MeanLevel advances the generator across [from, to) and returns the
// time-weighted average output over that window.
//
// Averaging rather than point-sampling is what keeps tones above the Nyquist
// limit from aliasing down into the audible band, and it is the same
// resampling contract Beeper implements. The window is walked one *event* at a
// time, not one chip cycle at a time, so cost tracks how often the generators
// actually change rather than the clock rate.
func (ay *AY8912) MeanLevel(from, to int64) int16 {
	if to <= from {
		return int16(ay.output())
	}

	// Register writes inside this window take effect at its start. At 44.1 kHz
	// a window is ~40 chip cycles, far below anything audible, and it keeps a
	// write from being heard a sample before the CPU made it.
	ay.applyUntil(to)

	cycles := ay.cyclesFor(to - from)
	if cycles <= 0 {
		return int16(ay.output())
	}

	var acc int64
	remaining := cycles
	for remaining > 0 {
		step := ay.nextStep(remaining)
		acc += int64(ay.output()) * step
		ay.advance(step)
		remaining -= step
	}
	return int16(acc / cycles)
}

// nextStep returns the largest number of chip cycles that can be advanced
// without any generator firing twice: the nearest pending tone, noise or
// envelope event, at least 1.
func (ay *AY8912) nextStep(remaining int64) int64 {
	step := remaining
	for i := 0; i < 3; i++ {
		if ay.toneCount[i] < step {
			step = ay.toneCount[i]
		}
	}
	if ay.noiseCount < step {
		step = ay.noiseCount
	}
	if !ay.envHolding && ay.envCount < step {
		step = ay.envCount
	}
	if step < 1 {
		step = 1
	}
	return step
}

// cyclesFor converts a CPU-tick span to whole chip cycles, carrying the
// remainder so the ratio stays exact over any number of calls.
func (ay *AY8912) cyclesFor(ticks int64) int64 {
	if ticks <= 0 || ay.cpuFreq == 0 {
		return 0
	}
	ay.chipAcc += ticks * int64(ay.chipFreq)
	cycles := ay.chipAcc / int64(ay.cpuFreq)
	ay.chipAcc -= cycles * int64(ay.cpuFreq)
	return cycles
}

// advance runs every divider forward by step chip cycles. step is never larger
// than the nearest pending event, so each counter fires at most once.
func (ay *AY8912) advance(step int64) {
	for i := 0; i < 3; i++ {
		ay.toneCount[i] -= step
		if ay.toneCount[i] <= 0 {
			ay.toneCount[i] += ay.toneReload(i)
			ay.toneOut[i] = !ay.toneOut[i]
			ay.levelSet = false
		}
	}

	ay.noiseCount -= step
	if ay.noiseCount <= 0 {
		ay.noiseCount += ay.noiseReload()
		// 17-bit LFSR, input is bit0 XOR bit3, bit0 is the output.
		bit := (ay.noiseReg ^ (ay.noiseReg >> 3)) & 1
		ay.noiseReg = (ay.noiseReg >> 1) | (bit << 16)
		out := (ay.noiseReg & 1) != 0
		if out != ay.noiseOut {
			ay.noiseOut = out
			ay.levelSet = false
		}
	}

	if !ay.envHolding {
		ay.envCount -= step
		if ay.envCount <= 0 {
			ay.envCount += ay.envReload()
			ay.stepEnvelope()
			ay.levelSet = false
		}
	}
}

// stepEnvelope advances the 16-state envelope counter and applies the
// shape/cycle rules of R13.
//
// The overflow test relies on envVolume being unsigned: counting down past 0
// wraps to 255, which trips the same `> 0x0F` check as counting up past 15.
// That is deliberate, and matches vgm_decoder.
func (ay *AY8912) stepEnvelope() {
	if ay.envAttack {
		ay.envVolume++
	} else {
		ay.envVolume--
	}
	if ay.envVolume <= 0x0F {
		return
	}

	// A boundary was crossed. Step back onto it, then decide what happens next.
	ay.envHolding = ay.envHold
	if ay.envAttack {
		ay.envVolume--
	} else {
		ay.envVolume++
	}

	switch {
	case !ay.envContinue:
		ay.envVolume = 0
	case ay.envAlternate && ay.envHold:
		ay.envVolume ^= 0x0F
	case !ay.envHold && !ay.envAlternate:
		ay.envVolume ^= 0x0F
	case !ay.envHold && ay.envAlternate:
		ay.envAttack = !ay.envAttack
	}
}

// output returns the summed level of all three channels, cached until a
// generator or register changes it.
//
// The AY-3-8912 mixes its three channels into a single mono output; there is no
// per-channel stereo on the chip. The mixer centres that mono sum across both
// ears (a StereoSource like TurboSound adds its own panning at a higher level).
func (ay *AY8912) output() int32 {
	if ay.levelSet {
		return ay.level
	}
	var sum int32
	for ch := 0; ch < 3; ch++ {
		sum += ay.channelLevel(ch)
	}
	ay.level = sum
	ay.levelSet = true
	return sum
}

// channelLevel gates one channel through the tone/noise mixer.
//
// R7 holds *disable* bits, and the two generators are combined as
//
//	out = (tone | toneDisabled) & (noise | noiseDisabled)
//
// so a channel with both disabled passes DC at its programmed amplitude -- the
// datasheet's "neither" case (section 3.3), and how envelope-only percussion is made.
// vgm_decoder uses an OR form here, which silences that case and turns
// tone-plus-noise into a logical OR; this is the form the hardware implements.
func (ay *AY8912) channelLevel(ch int) int32 {
	toneDisabled := (ay.mixer & (1 << uint(ch))) != 0
	noiseDisabled := (ay.mixer & (1 << uint(ch+3))) != 0

	if !((ay.toneOut[ch] || toneDisabled) && (ay.noiseOut || noiseDisabled)) {
		return 0
	}

	vol := ay.amplitude[ch]
	if ay.useEnvelope[ch] {
		vol = ay.envVolume
	}
	return ay.levelTable[vol&0x0F]
}

// --- deferred writes -------------------------------------------------------

// applyUntil applies every queued write timestamped before tick.
func (ay *AY8912) applyUntil(tick int64) {
	if len(ay.pending) == 0 {
		return
	}
	n := 0
	for _, w := range ay.pending {
		if w.tick >= tick {
			break
		}
		ay.applyWrite(w.reg, w.val)
		n++
	}
	if n > 0 {
		ay.pending = append(ay.pending[:0], ay.pending[n:]...)
	}
}

// Flush applies every queued write up to tick without generating audio. Turbo
// mode uses it to keep the chip's state coherent while the mixer is idle, so
// sound resumes correctly rather than replaying a backlog.
func (ay *AY8912) Flush(tick int64) {
	ay.applyUntil(tick + 1)
}

// Pending reports queued, unapplied register writes -- diagnostics only.
func (ay *AY8912) Pending() int { return len(ay.pending) }

var _ Source = (*AY8912)(nil)
