package sound

import (
	"math"

	"github.com/kiltum/zxgo-v3/pkg/io_ports"
	"github.com/kiltum/zxgo-v3/pkg/state"
)

// YM2203 (OPN) FM synthesizer -- the "TurboSound FM" sound chip. It has three
// 4-operator FM channels plus a 3-channel SSG (the AY-3-8910, reused via an
// embedded AY8912). The FM core is a port of the MAME fm.c implementation
// (Jarek Burczynski / Tatsuyuki Satoh); the register map follows ymfm's OPN.
//
// Accessed through the AY ports: 0xFFFD = register select, 0xBFFD = register
// data. Registers 0x00-0x0F address the SSG, 0x20-0x9F address the FM.

const (
	ymFreqShift = 16
	ymEnvBits   = 10
	ymEnvLen    = 1 << ymEnvBits
	ymEnvStep   = 128.0 / ymEnvLen
	ymSinLen    = 1 << 10 // 1024-entry sine table
	ymSinMask   = ymSinLen - 1
	ymTlResLen  = 256
	ymTlTabLen  = 13 * 2 * ymTlResLen
	ymEnvQuiet  = ymTlTabLen >> 3
	ymRateSteps = 8

	ymEgOff = 0
	ymEgAtt = 1
	ymEgDec = 2
	ymEgSus = 3
	ymEgRel = 4

	// ymEgTimerOverflow is MAME eg_timer_overflow = 3 * (1 << EG_SH); the EG
	// counter (egCnt) ticks once each time egTimer crosses it.
	ymEgTimerOverflow = int64(3 << 16)

	// connection-register indices for the algorithm graph
	ymRegOut = 0
	ymRegC1  = 1
	ymRegC2  = 2
	ymRegM2  = 3
	ymRegMem = 4

	// operator indices in the OPN connection order
	ymSlot1 = 0
	ymSlot2 = 2
	ymSlot3 = 1
	ymSlot4 = 3
)

var (
	ymSlTable [16]uint32
	ymEgInc   [19 * ymRateSteps]uint8
	ymEgSel   [128]uint8
	ymEgShift [128]uint8
	ymDtTab   [8 * 32]int32
	ymFkTable [16]uint8
	ymSinTab  [ymSinLen]int32
	ymTlTab   [ymTlTabLen]int32
)

func ymInitTables() {
	for i, v := range [16]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 31} {
		ymSlTable[i] = uint32(float64(v) * (4.0 / ymEnvStep))
	}

	egInc := [19 * 8]uint8{
		0, 1, 0, 1, 0, 1, 0, 1,
		0, 1, 0, 1, 1, 1, 0, 1,
		0, 1, 1, 1, 0, 1, 1, 1,
		0, 1, 1, 1, 1, 1, 1, 1,
		1, 1, 1, 1, 1, 1, 1, 1,
		1, 1, 1, 2, 1, 1, 1, 2,
		1, 2, 1, 2, 1, 2, 1, 2,
		1, 2, 2, 2, 1, 2, 2, 2,
		2, 2, 2, 2, 2, 2, 2, 2,
		2, 2, 2, 4, 2, 2, 2, 4,
		2, 4, 2, 4, 2, 4, 2, 4,
		2, 4, 4, 4, 2, 4, 4, 4,
		4, 4, 4, 4, 4, 4, 4, 4,
		4, 4, 4, 8, 4, 4, 4, 8,
		4, 8, 4, 8, 4, 8, 4, 8,
		4, 8, 8, 8, 4, 8, 8, 8,
		8, 8, 8, 8, 8, 8, 8, 8,
		16, 16, 16, 16, 16, 16, 16, 16,
		0, 0, 0, 0, 0, 0, 0, 0,
	}
	copy(ymEgInc[:], egInc[:])

	for i := 0; i < 32; i++ {
		ymEgSel[i] = 18 * ymRateSteps
		ymEgShift[i] = 0
	}
	// rates 00-11: 12 groups of {0,1,2,3} select, {11..0} shift
	for i := 0; i < 12; i++ {
		base := 32 + i*4
		ymEgSel[base+0], ymEgSel[base+1], ymEgSel[base+2], ymEgSel[base+3] = 0, 1*ymRateSteps, 2*ymRateSteps, 3*ymRateSteps
		sh := uint8(11 - i)
		ymEgShift[base+0], ymEgShift[base+1], ymEgShift[base+2], ymEgShift[base+3] = sh, sh, sh, sh
	}
	// rate 12..15 select values (each a distinct 4-value block)
	sel12 := []uint8{4, 5, 6, 7}
	sel13 := []uint8{8, 9, 10, 11}
	sel14 := []uint8{12, 13, 14, 15}
	sel15 := []uint8{16, 16, 16, 16}
	for i := 0; i < 4; i++ {
		ymEgSel[80+i] = sel12[i] * ymRateSteps
		ymEgSel[84+i] = sel13[i] * ymRateSteps
		ymEgSel[88+i] = sel14[i] * ymRateSteps
		ymEgSel[92+i] = sel15[i] * ymRateSteps
		ymEgShift[80+i] = 0
		ymEgShift[84+i] = 0
		ymEgShift[88+i] = 0
		ymEgShift[92+i] = 0
	}
	for i := 0; i < 32; i++ {
		ymEgSel[96+i] = 16 * ymRateSteps
		ymEgShift[96+i] = 0
	}

	dt := [4 * 32]uint8{
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 1, 1, 1, 1, 1, 1, 1, 1, 2, 2, 2, 2,
		2, 3, 3, 3, 4, 4, 4, 5, 5, 6, 6, 7, 8, 8, 8, 8,
		1, 1, 1, 1, 2, 2, 2, 2, 2, 3, 3, 3, 4, 4, 4, 5,
		5, 6, 6, 7, 8, 8, 9, 10, 11, 12, 13, 14, 16, 16, 16, 16,
		2, 2, 2, 2, 2, 3, 3, 3, 4, 4, 4, 5, 5, 6, 6, 7,
		8, 8, 9, 10, 11, 12, 13, 14, 16, 17, 19, 20, 22, 22, 22, 22,
	}
	// detune values are scaled by SIN_LEN * 2^FREQ_SH / 2^20 at runtime (they are
	// keycode-dependent), so store the raw base and scale in refreshFcEG.
	for d := 0; d < 4; d++ {
		for i := 0; i < 32; i++ {
			rate := int32(dt[d*32+i]) * ymSinLen * (1 << ymFreqShift) / (1 << 20)
			ymDtTab[d*32+i] = rate
			ymDtTab[(d+4)*32+i] = -rate
		}
	}
	ymFkTable = [16]uint8{0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 3, 3, 3, 3, 3, 3}

	for x := 0; x < ymTlResLen; x++ {
		m := math.Floor(float64(uint64(1)<<16) / math.Pow(2, float64(x+1)*(ymEnvStep/4.0)/8.0))
		n := int(m)
		n >>= 4
		if n&1 != 0 {
			n = (n >> 1) + 1
		} else {
			n >>= 1
		}
		n <<= 2
		ymTlTab[x*2] = int32(n)
		ymTlTab[x*2+1] = int32(-n)
		for i := 1; i < 13; i++ {
			ymTlTab[x*2+0+i*2*ymTlResLen] = int32(n >> i)
			ymTlTab[x*2+1+i*2*ymTlResLen] = int32(-(n >> i))
		}
	}

	for i := 0; i < ymSinLen; i++ {
		m := math.Sin(float64(i*2+1) * math.Pi / ymSinLen)
		o := 8.0 * math.Log(1.0/math.Abs(m)) / math.Log(2)
		o /= (ymEnvStep / 4.0)
		n := int(2.0 * o)
		if n&1 != 0 {
			n = (n >> 1) + 1
		} else {
			n >>= 1
		}
		ymSinTab[i] = int32(n*2) + boolInt(m >= 0.0)
	}
}

func boolInt(b bool) int32 {
	if b {
		return 1
	}
	return 0
}

type ymSlot struct {
	phase uint32
	incr  uint32
	dt    int32
	dtIdx uint8 // detune index (0-7)

	KSR uint8 // key scale rate register value (3 - R)
	ksr uint8 // computed key scale rate (kcode >> KSR)
	ar  uint32
	d1r uint32
	d2r uint32
	rr  uint32
	mul uint32
	tl  uint32
	sl  uint32
	ssg uint8
	// ssgn is the SSG-EG sign latch: bit 0 = "swapped once", bit 1 = "negated
	// output" (MAME FM_SLOT.ssgn). Updated by the sustain-phase alternate/hold
	// logic and by the 0x90 register write.
	ssgn uint8

	state  int
	volume int32
	key    bool

	egShAr, egSelAr   uint8
	egShD1r, egSelD1r uint8
	egShD2r, egSelD2r uint8
	egShRr, egSelRr   uint8
}

type ymChannel struct {
	slots [4]ymSlot
	algo  uint8
	fb    uint8
	fc    uint32

	op1Out [2]int32
	memVal int32

	connect1, connect2, connect3, connect4, memConnect int
}

// YM2203 is the TurboSound FM chip.
type YM2203 struct {
	ay    *AY8912
	ch    [3]ymChannel
	egCnt uint32
	// egTimer/egTimerAdd advance egCnt at the FM clock rate (MAME eg_timer),
	// so the ADSR rates stay on the chip clock rather than the output rate.
	egTimer    int64
	egTimerAdd int64

	regAddr uint8
	// prescalerSel is the 2-bit YM2203 clock prescaler selector (MAME fm.c
	// OPNPrescaler_w). Reset -> 2. Register 0x2D sets bit 1, 0x2E sets bit 0,
	// 0x2F clears both. The FM clock divisor is {24, 24, 72, 36}[sel&3].
	prescalerSel uint8
	// mode is register 0x27; bit 6 enables channel 3's 3-slot mode, where its
	// operators take independent frequencies from 0xA8-0xAE. The low 6 bits are
	// the timer controls (reset/enable/load).
	mode uint8
	// ch3fc holds the three independent channel-3 block_freq values (block<<11|
	// fnum) used in 3-slot mode, indexed by the 0xA8-0xAA register's low 2 bits.
	ch3fc [3]uint32

	// Timer A (10-bit) and Timer B (8-bit) period registers and their countdown
	// counters, in FM-clock ticks (1 tick = 18 us at 4 MHz). 0 = stopped.
	ta, tb   uint32
	tac, tbc int64
	// timerFlags holds the timer overflow flags (bit 0 = A, bit 1 = B).
	timerFlags uint8
	// timerAcc/timerAdd advance the timers at the FM clock rate.
	timerAcc int64

	fmClock    float64
	cpuFreq    int
	sampleRate int

	fracAcc   int64 // tick->sample conversion remainder
	lastOut   int32
	curTick   int64 // CPU tick, refreshed by SetTick before each instruction
	busyUntil int64 // CPU tick until which the busy flag (status bit 7) is set
}

// YMClock is the YM2203 master clock in Hz. The TFM programmer manual
// (tfm-prg.pdf ch.4) gives the note table for a 4 MHz chip clock.
const YMClock = 4000000.0

// NewYM2203 creates a YM2203 clocked at fmClock Hz.
func NewYM2203(fmClock float64) *YM2203 {
	y := &YM2203{
		ay:         NewAY8912(),
		fmClock:    fmClock,
		cpuFreq:    3546900,
		sampleRate: 44100,
	}
	y.resetFM()
	y.updateSSGClock()
	y.updateEGTimer()
	return y
}

// ChunkID names the chunk a single YM2203 is written as. The emulator holds its
// FM chip as a TurboSound FM board, so this id is for the chip on its own; the
// board has its own.
func (y *YM2203) ChunkID() state.ID { return state.IDYM2203 }

func (y *YM2203) SetCPUClock(hz int) {
	if hz > 0 {
		y.cpuFreq = hz
		y.updateSSGClock()
	}
}

func (y *YM2203) SetSampleRate(rate int) {
	if rate > 0 {
		y.sampleRate = rate
		y.updateEGTimer()
	}
}

// SetTick forwards the tick to the embedded AY (the SSG part), so its register
// writes are timestamped, and records it for the busy-flag timing. The FM part
// is sample-based and ignores it.
func (y *YM2203) SetTick(tick int64) {
	y.curTick = tick
	y.ay.SetTick(tick)
}

// SetClockFrequency sets the FM clock (the AYChip interface).
func (y *YM2203) SetClockFrequency(hz uint32) {
	if hz > 0 {
		y.fmClock = float64(hz)
		y.updateSSGClock()
		y.updateEGTimer()
	}
}

// updateSSGClock clocks the embedded AY (the SSG section) from the master
// clock and the current prescaler. The YM2203 SSG runs at master*2/ssg_pres,
// where ssg_pres is {1,1,4,2} for selector {0,1,2,3} (MAME OPNSetPres); for
// the default selector 2 that is master/2.
func (y *YM2203) updateSSGClock() {
	ssgPres := [4]uint32{1, 1, 4, 2}[y.prescalerSel&3]
	ssgClock := y.fmClock * 2.0 / float64(ssgPres)
	y.ay.SetClockFrequency(uint32(ssgClock))
	y.ay.SetCPUClock(uint32(y.cpuFreq))
}

// updateEGTimer recomputes the EG counter advance step from the current clock
// and prescaler (MAME eg_timer_add = (1<<EG_SH) * freqbase).
func (y *YM2203) updateEGTimer() {
	div := float64(ymPrescaleDivisor(y.prescalerSel))
	freqbase := y.fmClock / (float64(y.sampleRate) * div)
	y.egTimerAdd = int64(freqbase * (1 << 16))
}

// setBusy marks the chip busy for 32*clock_prescale master cycles after a
// register write (ymfm ymfm_set_busy_end), converted to CPU ticks. clock_prescale
// is the divisor/12: 72 -> 6, 36 -> 3, 24 -> 2.
func (y *YM2203) setBusy() {
	prescale := ymPrescaleDivisor(y.prescalerSel) / 12
	cycles := int64(32 * prescale)
	y.busyUntil = y.curTick + cycles*int64(y.cpuFreq)/int64(y.fmClock) + 1
}

// busy reports the status-register busy flag: true while a write is settling.
func (y *YM2203) busy() bool {
	return y.curTick < y.busyUntil
}

// status returns the timer overflow flags (bit 0 = A, bit 1 = B). The busy flag
// (bit 7) is merged in by the caller.
func (y *YM2203) status() uint8 {
	return y.timerFlags
}

// tickTimers advances both timers by one FM-clock tick: each running timer
// counts down and, on reaching zero, sets its overflow flag (if enabled) and
// reloads (MAME TimerAOver/TimerBOver).
func (y *YM2203) tickTimers() {
	if y.tac > 0 {
		y.tac--
		if y.tac == 0 {
			if y.mode&0x04 != 0 {
				y.timerFlags |= 0x01
			}
			y.tac = int64(1024 - y.ta)
		}
	}
	if y.tbc > 0 {
		y.tbc--
		if y.tbc == 0 {
			if y.mode&0x08 != 0 {
				y.timerFlags |= 0x02
			}
			y.tbc = int64(256-y.tb) * 16
		}
	}
}

func (y *YM2203) resetFM() {
	for c := 0; c < 3; c++ {
		ch := &y.ch[c]
		ch.algo = 0
		ch.fb = 0
		ch.fc = 0
		ch.op1Out = [2]int32{}
		ch.memVal = 0
		for s := 0; s < 4; s++ {
			ch.slots[s] = ymSlot{state: ymEgOff, volume: ymEnvLen - 1, mul: 2, KSR: 3}
		}
		y.setupConnection(c)
	}
	y.egCnt = 0
	y.egTimer = 0
	y.prescalerSel = 2
	y.mode = 0
	y.ch3fc = [3]uint32{}
	y.ta, y.tb = 0, 0
	y.tac, y.tbc = 0, 0
	y.timerFlags = 0
	y.timerAcc = 0
	y.fracAcc = 0
	y.lastOut = 0
}

func (y *YM2203) Reset() {
	y.ay.Reset()
	y.resetFM()
	y.regAddr = 0
	y.busyUntil = 0
	y.curTick = 0
	y.updateSSGClock()
}

// setupConnection wires the algorithm's operator connection graph.
func (y *YM2203) setupConnection(c int) {
	ch := &y.ch[c]
	switch ch.algo {
	case 0:
		ch.connect1, ch.connect2, ch.connect3, ch.memConnect = ymRegC1, ymRegMem, ymRegC2, ymRegM2
	case 1:
		ch.connect1, ch.connect2, ch.connect3, ch.memConnect = ymRegMem, ymRegMem, ymRegC2, ymRegM2
	case 2:
		ch.connect1, ch.connect2, ch.connect3, ch.memConnect = ymRegC2, ymRegMem, ymRegC2, ymRegM2
	case 3:
		ch.connect1, ch.connect2, ch.connect3, ch.memConnect = ymRegC1, ymRegMem, ymRegC2, ymRegC2
	case 4:
		ch.connect1, ch.connect2, ch.connect3, ch.memConnect = ymRegC1, ymRegOut, ymRegC2, ymRegMem
	case 5:
		ch.connect1, ch.connect2, ch.connect3, ch.memConnect = -1, ymRegOut, ymRegOut, ymRegM2
	case 6:
		ch.connect1, ch.connect2, ch.connect3, ch.memConnect = ymRegC1, ymRegOut, ymRegOut, ymRegMem
	case 7:
		ch.connect1, ch.connect2, ch.connect3, ch.memConnect = ymRegOut, ymRegOut, ymRegOut, ymRegMem
	}
	ch.connect4 = ymRegOut
}

// --- io_ports.PortHandler ---

func (y *YM2203) HandlesPort(port uint16) bool {
	if (port & 0x0002) != 0 {
		return false
	}
	sel := port & 0xC000
	return sel == 0xC000 || sel == 0x8000
}

func (y *YM2203) Read(port uint16) uint8 {
	if (port&0x0002) == 0 && (port&0xC000) == 0xC000 {
		// Read back the selected register. The SSG registers (0x00-0x0F) are the
		// AY shadow registers; the FM registers have no read-back data.
		if y.regAddr <= 0x0F {
			return y.ay.ReadRegister(y.regAddr)
		}
		return 0x00
	}
	return 0xFF
}

func (y *YM2203) Write(port uint16, value uint8) {
	if (port & 0x0002) != 0 {
		return
	}
	switch port & 0xC000 {
	case 0xC000:
		y.regAddr = value
		y.setBusy()
	case 0x8000:
		y.writeReg(y.regAddr, value)
		y.setBusy()
	}
}

func (y *YM2203) writeReg(reg, val uint8) {
	if reg <= 0x0F {
		y.ay.writeRegister(reg, val)
		return
	}
	switch {
	case reg >= 0x30 && reg < 0xA0:
		// TFM register layout (tfm-prg.pdf ch.4): register = base + (operator<<2) + channel,
		// where channel is bits 0-1 and operator is bits 2-3. Channel 3 is the unused slot.
		ch := int(reg & 0x03)
		op := int((reg >> 2) & 0x03)
		if ch == 3 {
			return // unused channel slot
		}
		y.setOperatorReg(ch, op, reg&0xF0, val)
	case reg >= 0xA0 && reg <= 0xA2:
		ch := int(reg - 0xA0)
		y.ch[ch].fc = (y.ch[ch].fc & 0x3F00) | uint32(val)
		y.refreshFcEG(ch)
	case reg >= 0xA4 && reg <= 0xA6:
		ch := int(reg - 0xA4)
		y.ch[ch].fc = (y.ch[ch].fc & 0x00FF) | (uint32(val&0x3F) << 8)
		y.refreshFcEG(ch)
	case reg >= 0xA8 && reg <= 0xAA:
		// 3-slot mode: channel 3 operator frequency, low 8 bits of fnum.
		c := int(reg - 0xA8)
		y.ch3fc[c] = (y.ch3fc[c] & 0x3F00) | uint32(val)
		y.refreshFcEG(2)
	case reg >= 0xAC && reg <= 0xAE:
		// 3-slot mode: channel 3 operator frequency, high 3 bits + block.
		c := int(reg - 0xAC)
		y.ch3fc[c] = (y.ch3fc[c] & 0x00FF) | (uint32(val&0x3F) << 8)
		y.refreshFcEG(2)
	case reg >= 0xB0 && reg <= 0xB2:
		ch := int(reg - 0xB0)
		y.ch[ch].algo = val & 0x07
		y.ch[ch].fb = (val >> 3) & 0x07
		y.setupConnection(ch)
	case reg == 0x24:
		// Timer A high 8 bits (bits 2-9).
		y.ta = (y.ta & 0x03) | (uint32(val) << 2)
	case reg == 0x25:
		// Timer A low 2 bits (bits 0-1).
		y.ta = (y.ta & 0x3FC) | uint32(val&0x03)
	case reg == 0x26:
		// Timer B (8-bit).
		y.tb = uint32(val)
	case reg == 0x27:
		// Timer/mode register (MAME set_timers): bits 4/5 reset the overflow
		// flags, bits 0/1 load-or-stop the timers, bit 6 is the 3-slot mode.
		y.mode = val
		if val&0x20 != 0 {
			y.timerFlags &^= 0x02 // reset timer B flag
		}
		if val&0x10 != 0 {
			y.timerFlags &^= 0x01 // reset timer A flag
		}
		if val&0x02 != 0 { // load B
			if y.tbc == 0 {
				y.tbc = int64(256-y.tb) * 16
			}
		} else {
			y.tbc = 0 // stop B
		}
		if val&0x01 != 0 { // load A
			if y.tac == 0 {
				y.tac = int64(1024 - y.ta)
			}
		} else {
			y.tac = 0 // stop A
		}
		y.refreshFcEG(2) // bit 6 may have toggled the 3-slot mode
	case reg == 0x28:
		ch := int(val & 0x03)
		if ch == 3 {
			return
		}
		mask := val >> 4
		for op := 0; op < 4; op++ {
			if mask&(1<<uint(op)) != 0 {
				y.keyOn(ch, op)
			} else {
				y.keyOff(ch, op)
			}
		}
	case reg == 0x2D:
		y.prescalerSel |= 0x02
		y.refreshAllFcEG()
		y.updateSSGClock()
		y.updateEGTimer()
	case reg == 0x2E:
		y.prescalerSel |= 0x01
		y.refreshAllFcEG()
		y.updateSSGClock()
		y.updateEGTimer()
	case reg == 0x2F:
		y.prescalerSel = 0
		y.refreshAllFcEG()
		y.updateSSGClock()
		y.updateEGTimer()
	}
}

func (y *YM2203) setOperatorReg(ch, op int, group, val uint8) {
	sl := &y.ch[ch].slots[op]
	switch group {
	case 0x30:
		mul := val & 0x0F
		if mul == 0 {
			sl.mul = 1 // MUL=0 is half frequency
		} else {
			sl.mul = uint32(mul) * 2
		}
		sl.dtIdx = (val >> 4) & 0x07
		y.refreshFcEG(ch)
	case 0x40:
		sl.tl = uint32(val&0x7F) << (ymEnvBits - 7)
	case 0x50:
		sl.KSR = 3 - (val >> 6)
		sl.ar = ymRateOf(val & 0x1F)
		y.refreshFcEG(ch)
	case 0x60:
		sl.d1r = ymRateOf(val & 0x1F)
		sl.refreshEGRates()
	case 0x70:
		sl.d2r = ymRateOf(val & 0x1F)
		sl.refreshEGRates()
	case 0x80:
		sl.sl = ymSlTable[val>>4]
		sl.rr = 34 + uint32(val&0x0F)*4
		sl.refreshEGRates()
	case 0x90:
		sl.ssg = val & 0x0F
		sl.ssgn = (val & 0x04) >> 1
	}
}

func ymRateOf(r uint8) uint32 {
	if r == 0 {
		return 0
	}
	return 32 + uint32(r)*2
}

// ymPrescaleDivisor maps the 2-bit prescaler selector to the FM clock divisor
// (MAME fm.c opn_pres = {24, 24, 72, 36}).
func ymPrescaleDivisor(sel uint8) uint32 {
	switch sel & 3 {
	case 0, 1:
		return 24
	case 3:
		return 36
	default:
		return 72
	}
}

// refreshAllFcEG recomputes the phase step of every channel after a change to
// the clock prescaler (which scales the master clock, and hence every note).
func (y *YM2203) refreshAllFcEG() {
	for c := 0; c < 3; c++ {
		y.refreshFcEG(c)
	}
}

func (sl *ymSlot) refreshEGRates() {
	ar := sl.ar + uint32(sl.ksr)
	if ar < 32+62 {
		sl.egShAr = ymEgShift[ar]
		sl.egSelAr = ymEgSel[ar]
	} else {
		sl.egShAr = 0
		sl.egSelAr = 17 * ymRateSteps
	}
	sl.egShD1r = ymEgShift[sl.d1r+uint32(sl.ksr)]
	sl.egSelD1r = ymEgSel[sl.d1r+uint32(sl.ksr)]
	sl.egShD2r = ymEgShift[sl.d2r+uint32(sl.ksr)]
	sl.egSelD2r = ymEgSel[sl.d2r+uint32(sl.ksr)]
	sl.egShRr = ymEgShift[sl.rr+uint32(sl.ksr)]
	sl.egSelRr = ymEgSel[sl.rr+uint32(sl.ksr)]
}

// refreshSlot sets one operator's phase step and key-scaled EG rates from a
// block_freq value (block<<11 | fnum).
func (y *YM2203) refreshSlot(ch *ymChannel, sl *ymSlot, blockFnum uint32) {
	// key code = (block << 2) | opn_fktable[fnum >> 7] (MAME refresh_fc_eg_chan)
	fnum := blockFnum & 0x7FF
	block := (blockFnum >> 11) & 0x07
	kc := (block << 2) | uint32(ymFkTable[fnum>>7])
	div := float64(ymPrescaleDivisor(y.prescalerSel))
	// MAME: Incr = ((fc + DT[kc]) * mul) >> 1, where
	//   fc  = fnum * freqbase * 2^(block+5)
	//   DT  = dt_tab[dtIdx][kc&31] * freqbase
	//   freqbase = fmClock / (sampleRate * div)
	freqbase := y.fmClock / (float64(y.sampleRate) * div)
	baseFC := float64(fnum) * freqbase * float64(uint64(1)<<(block+5))
	sl.dt = ymDtTab[int(sl.dtIdx)*32+int(kc&31)]
	sl.incr = uint32((baseFC + float64(sl.dt)*freqbase) * float64(sl.mul) / 2.0)
	sl.ksr = uint8(kc >> sl.KSR)
	sl.refreshEGRates()
}

func (y *YM2203) refreshFcEG(c int) {
	ch := &y.ch[c]
	if c == 2 && y.mode&0x40 != 0 {
		// 3-slot mode: channel 3's operators use independent frequencies from
		// 0xA8-0xAE (MAME SL3). SLOT1<-ch3fc[1], SLOT2<-ch3fc[2],
		// SLOT3<-ch3fc[0], SLOT4<-normal channel fc.
		y.refreshSlot(ch, &ch.slots[0], y.ch3fc[1])
		y.refreshSlot(ch, &ch.slots[2], y.ch3fc[2])
		y.refreshSlot(ch, &ch.slots[1], y.ch3fc[0])
		y.refreshSlot(ch, &ch.slots[3], ch.fc)
		return
	}
	for s := 0; s < 4; s++ {
		y.refreshSlot(ch, &ch.slots[s], ch.fc)
	}
}

func (y *YM2203) keyOn(c, op int) {
	sl := &y.ch[c].slots[op]
	if !sl.key {
		sl.key = true
		sl.phase = 0
		sl.state = ymEgAtt
		if sl.volume >= ymEnvLen-1 {
			sl.volume = 511
		}
	}
}

// keyOff releases an operator: from attack/decay/sustain it enters the release
// phase; an operator already off or releasing is left alone (MAME FM_KEYOFF).
func (y *YM2203) keyOff(c, op int) {
	sl := &y.ch[c].slots[op]
	if sl.key {
		sl.key = false
		if sl.state == ymEgAtt || sl.state == ymEgDec || sl.state == ymEgSus {
			sl.state = ymEgRel
		}
	}
}

// --- synthesis ---

func opCalc(phase uint32, env uint32, pm int32) int32 {
	// phase & ~FREQ_MASK (the high 16 bits), plus pm<<15 phase modulation.
	idx := (int32(phase&0xFFFF0000) + (pm << 15)) >> ymFreqShift
	p := int32(env<<3) + ymSinTab[int(idx)&ymSinMask]
	if p >= int32(ymTlTabLen) {
		return 0
	}
	return ymTlTab[p]
}

// opCalc1 is opCalc without the pm<<15 scaling, used for the SLOT1 feedback path.
func opCalc1(phase uint32, env uint32, pm int32) int32 {
	idx := (int32(phase&0xFFFF0000) + pm) >> ymFreqShift
	p := int32(env<<3) + ymSinTab[int(idx)&ymSinMask]
	if p >= int32(ymTlTabLen) {
		return 0
	}
	return ymTlTab[p]
}

func (y *YM2203) advanceEG(sl *ymSlot) {
	var swapFlag uint8
	switch sl.state {
	case ymEgAtt:
		if y.egCnt&((1<<sl.egShAr)-1) == 0 {
			sl.volume += (int32(^sl.volume) * int32(ymEgInc[sl.egSelAr+uint8((y.egCnt>>sl.egShAr)&7)])) >> 4
			if sl.volume <= 0 {
				sl.volume = 0
				sl.state = ymEgDec
			}
		}
	case ymEgDec:
		if y.egCnt&((1<<sl.egShD1r)-1) == 0 {
			step := int32(ymEgInc[sl.egSelD1r+uint8((y.egCnt>>sl.egShD1r)&7)])
			if sl.ssg&0x08 != 0 {
				step *= 4 // SSG-EG decay has 1/4 the resolution
			}
			sl.volume += step
			if uint32(sl.volume) >= sl.sl {
				sl.state = ymEgSus
			}
		}
	case ymEgSus:
		if y.egCnt&((1<<sl.egShD2r)-1) == 0 {
			step := int32(ymEgInc[sl.egSelD2r+uint8((y.egCnt>>sl.egShD2r)&7)])
			if sl.ssg&0x08 != 0 {
				step *= 4 // SSG-EG sustain has 1/4 the resolution
			}
			sl.volume += step
			if sl.ssg&0x08 != 0 {
				if sl.volume >= 512 {
					sl.volume = ymEnvLen - 1
					if sl.ssg&0x01 != 0 { // bit 0 = hold
						if sl.ssgn&1 == 0 {
							swapFlag = (sl.ssg & 0x02) | 1 // bit 1 = alternate
						}
					} else {
						// restart attack, as on key-on
						sl.phase = 0
						sl.volume = 511
						sl.state = ymEgAtt
						swapFlag = sl.ssg & 0x02
					}
				}
			} else if sl.volume >= int32(ymEnvLen-1) {
				sl.volume = ymEnvLen - 1
			}
		}
	case ymEgRel:
		if y.egCnt&((1<<sl.egShRr)-1) == 0 {
			sl.volume += int32(ymEgInc[sl.egSelRr+uint8((y.egCnt>>sl.egShRr)&7)])
			if sl.volume >= int32(ymEnvLen-1) {
				sl.volume = ymEnvLen - 1
				sl.state = ymEgOff
			}
		}
	}
	sl.ssgn ^= swapFlag
}

// egLevel returns the effective envelope level for this operator (total level
// plus envelope volume), with the SSG-EG output negation applied when active.
func (sl *ymSlot) egLevel() uint32 {
	eg := sl.tl + uint32(sl.volume)
	if sl.ssg&0x08 != 0 && sl.ssgn&0x02 != 0 && sl.state != ymEgOff {
		eg ^= 511
	}
	return eg
}

// chanCalc computes one channel's FM output for the current sample.
func (y *YM2203) chanCalc(c int) int32 {
	ch := &y.ch[c]
	var regs [5]int32 // out, c1, c2, m2, mem

	regs[ch.memConnect] = ch.memVal

	// operator 1 (with feedback)
	eg := ch.slots[ymSlot1].egLevel()
	{
		out := ch.op1Out[0] + ch.op1Out[1]
		ch.op1Out[0] = ch.op1Out[1]
		if ch.connect1 == -1 {
			// algorithm 5: op1 output feeds the memory and both carriers directly
			regs[ymRegMem], regs[ymRegC1], regs[ymRegC2] = out, out, out
		} else {
			regs[ch.connect1] += out
		}
		ch.op1Out[1] = 0
		if eg < ymEnvQuiet {
			if ch.fb == 0 {
				out = 0
			}
			ch.op1Out[1] = opCalc1(ch.slots[ymSlot1].phase, eg, out<<ch.fb)
		}
	}

	eg = ch.slots[ymSlot3].egLevel()
	if eg < ymEnvQuiet {
		regs[ch.connect3] += opCalc(ch.slots[ymSlot3].phase, eg, regs[ymRegM2])
	}

	eg = ch.slots[ymSlot2].egLevel()
	if eg < ymEnvQuiet {
		regs[ch.connect2] += opCalc(ch.slots[ymSlot2].phase, eg, regs[ymRegC1])
	}

	eg = ch.slots[ymSlot4].egLevel()
	if eg < ymEnvQuiet {
		regs[ch.connect4] += opCalc(ch.slots[ymSlot4].phase, eg, regs[ymRegC2])
	}

	ch.memVal = regs[ymRegMem]

	ch.slots[ymSlot1].phase += ch.slots[ymSlot1].incr
	ch.slots[ymSlot2].phase += ch.slots[ymSlot2].incr
	ch.slots[ymSlot3].phase += ch.slots[ymSlot3].incr
	ch.slots[ymSlot4].phase += ch.slots[ymSlot4].incr

	return regs[ymRegOut]
}

// generateOne produces one FM output sample (mono FM, no SSG).
func (y *YM2203) generateOne() int32 {
	// Advance the envelope counter on the FM clock (MAME eg_timer), so the ADSR
	// rates track the chip clock rather than the output sample rate. The EG is
	// stepped only when egTimer crosses egTimerOverflow, ~freqbase/3 per sample.
	y.egTimer += y.egTimerAdd
	for y.egTimer >= ymEgTimerOverflow {
		y.egTimer -= ymEgTimerOverflow
		y.egCnt++
		for c := 0; c < 3; c++ {
			ch := &y.ch[c]
			y.advanceEG(&ch.slots[0])
			y.advanceEG(&ch.slots[1])
			y.advanceEG(&ch.slots[2])
			y.advanceEG(&ch.slots[3])
		}
	}
	// Advance the timers one FM-clock tick at a time (freqbase ticks/sample).
	y.timerAcc += y.egTimerAdd
	for y.timerAcc >= (1 << 16) {
		y.timerAcc -= (1 << 16)
		y.tickTimers()
	}
	var sum int32
	for c := 0; c < 3; c++ {
		sum += y.chanCalc(c)
	}
	return sum
}

// --- sound.Source ---

func (y *YM2203) MeanLevel(from, to int64) int16 {
	return y.meanLevel(from, to)
}

func (y *YM2203) MeanLevelStereo(from, to int64) (int16, int16) {
	v := y.meanLevel(from, to)
	return v, v
}

func (y *YM2203) meanLevel(from, to int64) int16 {
	// number of FM samples in [from, to), with the fractional remainder carried
	y.fracAcc += (to - from) * int64(y.sampleRate)
	nSamples := y.fracAcc / int64(y.cpuFreq)
	y.fracAcc %= int64(y.cpuFreq)

	var acc int64
	for i := int64(0); i < nSamples; i++ {
		acc += int64(y.generateOne())
	}
	var fm int32
	if nSamples == 0 {
		fm = y.lastOut
	} else {
		fm = int32(acc / nSamples)
		y.lastOut = fm
	}

	// Mix in the SSG (the embedded AY). The YM2203 sums the FM and SSG outputs,
	// so both must reach the mixer.
	ssg := int32(y.ay.MeanLevel(from, to))
	return int16((fm + ssg) / 2)
}

var _ io_ports.PortHandler = (*YM2203)(nil)
var _ Source = (*YM2203)(nil)
var _ StereoSource = (*YM2203)(nil)
var _ AYChip = (*YM2203)(nil)

func init() {
	ymInitTables()
}
