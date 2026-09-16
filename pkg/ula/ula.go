package ula

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/kiltum/zxgo-v3/pkg/mem"
	"github.com/kiltum/zxgo-v3/pkg/model"
)

// ulaLog is an optional logger for interrupt/border timing diagnostics.
// nil means no output (the default).
var ulaLog *slog.Logger

// SetULALogger wires a logger into the ULA for interrupt/border tracing.
func SetULALogger(l *slog.Logger) { ulaLog = l }

// borderWriteDelay is how many T-states after the start of an OUT (n),A the
// border register is latched. Hardware latches at T-state 7 of the 11 T-state
// instruction, but the emulator's instruction-level model applies the write at
// the instruction start, and 9 was verified against real hardware (many demos)
// to line the multicolor bars up correctly. Do not "correct" it back to 7.
const borderWriteDelay = 9

// ULA emulates the ZX Spectrum ULA chip -- video, border, keyboard, and tape I/O.
// It renders tick-by-tick into a variable-height ARGB8888 framebuffer whose
// geometry is derived from the model's timing config.
type ULA struct {
	mem    mem.MemoryMapper // for ULA screen reads (bank 5 or shadow bank 7)
	screen []uint32         // width * height ARGB8888 framebuffer
	name   string           // model name (for timing log)

	// Timing (derived from model.Config)
	// Contended memory writes the CPU has made recently, as (tick, value). The
	// 48K ULA reads the data bus while it is fetching screen bytes, so a CPU
	// access overlapping its fetch makes it read the CPU's byte instead of the
	// screen's - the "snow" artefact. Kept as a tiny ring; entries older than a
	// few T-states can never overlap a fetch and are ignored.
	snowTicks   [snowRing]int64
	snowValues  [snowRing]uint8
	snowNext    int
	snowEnabled bool

	width           int // framebuffer width in pixels ((borderLeftT + 128 + borderRightT) * 2)
	height          int // framebuffer height in lines (top border + 192 + bottom border)
	clock           int // T-state position within frame
	clockEndFrame   int // total T-states per frame
	clockPerLine    int // T-states per scanline
	interruptLength int // T-states /INT is held low
	interruptOffset int // T-state within frame where /INT is asserted

	// Geometry (derived from model.Config)
	flybackT      int // T-states of vertical retrace before the top border
	leftBlankT    int // T-states of blank/retrace before the left border
	borderLeftT   int // left border width in T-states
	borderRightT  int // right border width in T-states
	screenTopLine int // framebuffer row of the first visible screen line

	horClock      int   // horizontal position within current line
	line          int   // current scanline
	absoluteClock int64 // cumulative T-states since ULA reset (for absolute interrupt timing)

	// State
	borderColor int        // border color (0-7)
	flash       bool       // flash toggle state
	frameCnt    int        // frames since last flash toggle
	colors      [16]uint32 // pre-calculated ARGB color table

	// Deferred border write: the Z80 latches the border register near the end of
	// an OUT (n),A (T-state 7 of 11), not at its start. The emulator applies the
	// port write before the instruction's ticks, so we defer the visible border
	// change by a fixed number of T-states to align multicolor bars.
	pendingBorder     int   // border color waiting to be applied
	pendingBorderTick int64 // absolute tick at which to apply it (0 = none)

	// Keyboard (8 rows * 5 columns)
	keyboard [8]uint8 // all 1s = released, 0 = pressed

	// Tape EAR input
	audioState bool // fed by tape bitstream, read on port 0xFE bit 6

	// Interrupt
	// /INT is asserted for interruptLength T-states starting at interruptOffset.
	intAssertedUntil    int64 // Absolute tick when /INT deasserts
	interruptFrameCount int64 // Frame count at last interrupt for debugging
}

// New creates a new ULA attached to the given memory mapper and model configuration.
func New(mm mem.MemoryMapper, cfg model.Config) *ULA {
	u := &ULA{
		mem:             mm,
		name:            cfg.Name,
		flybackT:        cfg.FlybackTStates,
		leftBlankT:      cfg.LeftBlankTStates,
		borderLeftT:     cfg.LeftBorderTStates,
		borderRightT:    cfg.RightBorderTStates,
		clockPerLine:    cfg.ClockPerLine(),
		clockEndFrame:   cfg.ClockEndFrame(),
		interruptLength: cfg.InterruptLength,
		interruptOffset: cfg.InterruptOffset,
		width:           cfg.FramebufferWidth(),
		height:          cfg.FramebufferHeight(),
		screenTopLine:   cfg.TopBorderLines,
	}
	u.screen = make([]uint32, u.width*u.height)

	// Init palette
	u.colors[0] = 0xFF000000  // Black
	u.colors[1] = 0xFF0000C0  // Blue
	u.colors[2] = 0xFFC00000  // Red
	u.colors[3] = 0xFFC000C0  // Magenta
	u.colors[4] = 0xFF00C000  // Green
	u.colors[5] = 0xFF00C0C0  // Cyan
	u.colors[6] = 0xFFC0C000  // Yellow
	u.colors[7] = 0xFFC0C0C0  // White
	u.colors[8] = 0xFF000000  // Black (bright)
	u.colors[9] = 0xFF0000FF  // Bright Blue
	u.colors[10] = 0xFFFF0000 // Bright Red
	u.colors[11] = 0xFFFF00FF // Bright Magenta
	u.colors[12] = 0xFF00FF00 // Bright Green
	u.colors[13] = 0xFFFFFF00 // Bright Cyan
	u.colors[14] = 0xFF00FFFF // Bright Yellow
	u.colors[15] = 0xFFFFFFFF // Bright White

	// Init keyboard (all released)
	for i := 0; i < 8; i++ {
		u.keyboard[i] = 0xFF
	}

	u.clock = 0
	u.line = 0

	if os.Getenv("ZXGO_ULA_TIMING") == "1" {
		u.logTiming(cfg)
	}
	return u
}

// GetScreen returns the ARGB framebuffer (width*height pixels).
func (u *ULA) GetScreen() []uint32 { return u.screen }

// Width returns the framebuffer width in pixels.
func (u *ULA) Width() int { return u.width }

// Height returns the framebuffer height in pixels (lines).
func (u *ULA) Height() int { return u.height }

// Reset clears the ULA state.
func (u *ULA) Reset() {
	u.clock = 0
	u.absoluteClock = 0
	u.horClock = 0
	u.line = 0
	u.flash = false
	u.frameCnt = 0
	u.borderColor = 0
	u.pendingBorder = 0
	u.pendingBorderTick = 0
	u.interruptFrameCount = 0
	u.intAssertedUntil = 0
	for i := range u.screen {
		u.screen[i] = u.colors[0]
	}
	for i := 0; i < 8; i++ {
		u.keyboard[i] = 0xFF
	}
}

// SetAudioState sets the EAR input state (from tape bitstream).
func (u *ULA) SetAudioState(on bool) {
	u.audioState = on
}

// --- I/O ports ---

// HandlesPort implements io_ports.PortHandler. ULA responds to port 0xFE.
func (u *ULA) HandlesPort(port uint16) bool {
	return (port & 0xFF) == 0xFE
}

// Read handles port reads. Returns keyboard state + EAR bit.
// Real hardware ANDs all selected half-rows onto shared open-collector lines.
func (u *ULA) Read(port uint16) uint8 {
	halfRowSelect := (port >> 8) & 0xFF
	result := uint8(0xFF) // Start with all released (high means unpressed)

	for i := 0; i < 8; i++ {
		if (halfRowSelect & (1 << i)) == 0 {
			result &= u.keyboard[i]
		}
	}

	// Apply EAR bit (bit 6) - audioState is fed by tape bitstream
	if u.audioState {
		result |= 0x40
	} else {
		result &^= 0x40
	}

	// Bits 7,5-0 are unused and float high on real hardware
	// Bits 4-0 are the keyboard lines (already included from keyboard[])
	// Bit 6 is EAR tape input (set above)
	return result
}

// Write handles port writes. Sets border color and EAR/MIC bits.
// ReadAttached implements io_ports.AttachedHandler. On a read of the ULA port
// (any port whose low byte is 0xFE) the chip drives D0-D4 (the selected
// keyboard half-rows) and D6 (EAR); D5 and D7 are undriven and take the
// floating bus, so Read leaves them high and the mask excludes them. Note this
// is a deliberate divergence from Fuse, which declares the ULA as driving all
// eight bits (ref/fuse-1.9.0/peripherals/ula.c:185) - the hardware floats those
// two, and software that samples the screen through 0xFE depends on it.
func (u *ULA) ReadAttached(port uint16) (uint8, uint8) {
	return u.Read(port), 0x5F // 0x1F keyboard | 0x40 EAR
}

func (u *ULA) Write(port uint16, value uint8) {
	// The border register is latched near the end of the OUT instruction
	// (T-state 7 of an 11 T-state OUT (n),A). Defer the visible change so the
	// multicolor bars line up instead of appearing ~7 T-states too early.
	u.pendingBorder = int(value & 0x07)
	u.pendingBorderTick = u.absoluteClock + borderWriteDelay
	u.audioState = (value & 0x10) != 0 // EAR bit (active high)
}

// --- Keyboard ---

// SetKeyDown sets a key as pressed (0 = pressed, 1 = released).
func (u *ULA) SetKeyDown(halfRow, keyBit int) {
	if halfRow >= 0 && halfRow < 8 && keyBit >= 0 && keyBit < 5 {
		u.keyboard[halfRow] &^= (1 << keyBit)
	}
}

// SetKeyUp sets a key as released.
func (u *ULA) SetKeyUp(halfRow, keyBit int) {
	if halfRow >= 0 && halfRow < 8 && keyBit >= 0 && keyBit < 5 {
		u.keyboard[halfRow] |= (1 << keyBit)
	}
}

// --- Tick ---

// OneTick advances the ULA by 1 T-state, drawing one column of the current
// scanline into the framebuffer. /INT is asserted for interruptLength T-states
// starting at interruptOffset (T=0 for Sinclair models, near the end of the
// frame for the Pentagon).
func (u *ULA) OneTick() {
	// Apply a deferred border-colour change at its latched T-state.
	if u.pendingBorderTick != 0 && u.absoluteClock >= u.pendingBorderTick {
		u.borderColor = u.pendingBorder
		u.pendingBorderTick = 0
	}

	u.render(u.clock)

	u.clock++
	u.absoluteClock++

	// Frame complete: reset the raster counters and advance the flash toggle.
	if u.clock >= u.clockEndFrame {
		u.clock = 0
		u.line = 0
		u.horClock = 0
		u.frameCnt++
		if u.frameCnt >= 16 {
			u.flash = !u.flash
			u.frameCnt = 0
		}
	}

	// Assert /INT at the model's interrupt position (T=0 for every model; the
	// Pentagon uses T=0 too, not a mid-frame offset - see PENTAGON_INT_TIMING.md).
	// The pulse is held for interruptLength T-states.
	if u.clock == u.interruptOffset {
		u.intAssertedUntil = u.absoluteClock + int64(u.interruptLength)
		u.interruptFrameCount++
		if ulaLog != nil {
			ulaLog.Debug("int_assert", "clock", u.clock, "abs", u.absoluteClock, "until", u.intAssertedUntil)
		}
	}
}

// render draws the column at frame T-state clock. clock is the position before
// the tick (0-based), so all ClockEndFrame positions are rendered once per frame.
func (u *ULA) render(clock int) {
	// Vertical retrace (flyback) before the top border.
	if clock < u.flybackT {
		return
	}
	rel := clock - u.flybackT
	line := rel / u.clockPerLine
	if line >= u.height {
		return // overscan past the bottom border
	}
	col := rel % u.clockPerLine
	u.line = line
	u.horClock = col

	// Left blank (retrace) and right blank after the right border.
	if col < u.leftBlankT || col >= u.leftBlankT+u.borderLeftT+model.ScreenTStates+u.borderRightT {
		return
	}
	col -= u.leftBlankT

	// Left/right border columns.
	if col < u.borderLeftT || col >= u.borderLeftT+model.ScreenTStates {
		u.drawBorder(line, col)
		return
	}

	// Top/bottom border lines.
	if line < u.screenTopLine || line >= u.screenTopLine+model.ScreenLines {
		u.drawBorder(line, col)
		return
	}

	x := (col - u.borderLeftT) * 2
	y := line - u.screenTopLine
	c := col * 2
	u.screen[line*u.width+c] = u.getPixelColorFast(x, y)
	u.screen[line*u.width+c+1] = u.getPixelColorFast(x+1, y)
}

// IntAssertedUntil returns the tick when /INT deasserts.
// This enables the CPU to sample interrupt levels instead of responding to single events (AUDIT T4).
func (u *ULA) IntAssertedUntil() int64 { return u.intAssertedUntil }

// FloatingBus returns the byte the ULA places on the data bus during a port
// read inside the visible screen area: the screen bitmap or attribute byte at
// the current raster position, or 0xFF outside it. This is the ZX "floating
// bus" that Arkanoid, Sidewize and snow-effect programs read (Fuse
// spectrum_unattached_port). It is approximate at instruction granularity --
// the exact byte depends on the I/O cycle's T-state, which the per-M-cycle
// hook would pin down -- but a tight read loop samples it at the right rate.
// snowRing is how many recent contended writes are remembered. An instruction
// can make two or three, and the ULA renders the whole instruction afterwards.
const snowRing = 16

// SnowWrite records a contended memory write: the CPU drives this value on the
// data bus for the next few T-states, and a ULA screen fetch overlapping that
// window reads it instead of memory. Only contended writes are interesting - an
// uncontended access does not clash with the ULA - and the caller (the emulator)
// decides that, because it is the part that knows the model.
func (u *ULA) SnowWrite(tick int64, value uint8) {
	u.snowTicks[u.snowNext] = tick
	u.snowValues[u.snowNext] = value
	u.snowNext = (u.snowNext + 1) % snowRing
}

// snowAt returns the byte a CPU write has on the bus at the given absolute tick,
// if one does. A write occupies 3 T-states and the ULA reads the byte as it is
// driven, so the window is [tick, tick+3).
func (u *ULA) snowAt(tick int64) (uint8, bool) {
	const writeTStates = 3
	for i := 0; i < snowRing; i++ {
		t := u.snowTicks[i]
		if t != 0 && tick >= t && tick < t+writeTStates {
			return u.snowValues[i], true
		}
	}
	return 0, false
}

// screenByte reads a screen byte for the ULA. During a contended CPU write the
// bus belongs to the CPU, so the ULA sees that byte: this is the approximation
// of the snow effect. It models the conflict, not the analogue behaviour of the
// chip, and it is judged by eye rather than derived.
func (u *ULA) screenByte(addr uint16) uint8 {
	if u.snowEnabled {
		if v, ok := u.snowAt(u.absoluteClock); ok {
			return v
		}
	}
	return u.mem.ULAReadByte(addr)
}

// AbsoluteClock returns the cumulative T-state count since reset, the same clock
// the emulator stamps every machine cycle with.
func (u *ULA) AbsoluteClock() int64 { return u.absoluteClock }

// ScreenByteForTest exposes screenByte so the snow substitution can be checked
// from outside the package.
func (u *ULA) ScreenByteForTest(addr uint16) uint8 { return u.screenByte(addr) }

// SetSnowEffect enables the 48K snow-effect approximation. The 128K and later
// machines keep their I register in contended RAM, so the same rule would grain
// their whole picture; the caller enables it only where it is documented.
func (u *ULA) SetSnowEffect(on bool) { u.snowEnabled = on }

func (u *ULA) FloatingBus() uint8 { return u.FloatingBusAt(0) }

// FloatingBusAt returns the floating-bus byte `delta` T-states ahead of the
// ULA's current position. The emulator's ULA is ticked once per T-state after
// each instruction, so its clock sits at the instruction's start; a port read
// inside an instruction asks for the byte at the I/O cycle's T-state instead,
// which is what an IN A,(0xFE) mid-screen actually samples.
func (u *ULA) FloatingBusAt(delta int) uint8 {
	clock := u.clock
	if delta != 0 {
		clock = (clock + delta) % u.clockEndFrame
	}
	rel := clock - u.flybackT
	if rel < 0 {
		return 0xFF
	}
	line := rel / u.clockPerLine
	if line < u.screenTopLine || line >= u.screenTopLine+model.ScreenLines {
		return 0xFF
	}
	col := rel % u.clockPerLine
	screenT := col - u.leftBlankT - u.borderLeftT
	if screenT < 0 || screenT >= model.ScreenTStates {
		return 0xFF
	}

	y := line - u.screenTopLine
	column := (screenT / 8) * 2
	switch screenT % 8 {
	case 5:
		column++
		fallthrough
	case 3:
		return u.screenByte(uint16(attrByteAddr(y, column)))
	case 4:
		column++
		fallthrough
	case 2:
		return u.screenByte(uint16(bitmapByteAddr(y, column)))
	default:
		return 0xFF
	}
}

// drawBorder writes a border-colored column at framebuffer (row, col).
func (u *ULA) drawBorder(row, col int) {
	c := col * 2
	u.screen[row*u.width+c] = u.colors[u.borderColor]
	u.screen[row*u.width+c+1] = u.colors[u.borderColor]
}

// bitmapByteAddr returns the screen bitmap byte address for character cell
// (col, y): col is 0..31, y is 0..191.
func bitmapByteAddr(y, col int) int {
	return 0x4000 + ((y & 0xC0) << 5) + ((y & 0x07) << 8) + ((y & 0x38) << 2) + col
}

// attrByteAddr returns the attribute byte address for character cell (col, y).
func attrByteAddr(y, col int) int {
	return 0x5800 + ((y >> 3) * 32) + col
}

// getPixelColorFast reads screen memory and returns the ARGB pixel color.
func (u *ULA) getPixelColorFast(x, y int) uint32 {
	bitmap := u.screenByte(uint16(bitmapByteAddr(y, x>>3)))
	attr := u.screenByte(uint16(attrByteAddr(y, x>>3)))

	bit := uint8(0x80 >> (x & 0x07))

	var ink, paper uint8
	if (attr&0x80) != 0 && u.flash {
		ink = (attr >> 3) & 0x07
		paper = attr & 0x07
	} else {
		ink = attr & 0x07
		paper = (attr >> 3) & 0x07
	}

	if attr&0x40 != 0 { // bright
		ink |= 0x08
		paper |= 0x08
	}

	if (bitmap & bit) != 0 {
		return u.colors[ink]
	}
	return u.colors[paper]
}

// Line returns the current scanline.
func (u *ULA) Line() int { return u.line }

// Clock returns the current T-state position within the frame.
func (u *ULA) Clock() int { return u.clock }

// BorderColor returns the current border color.
func (u *ULA) BorderColor() int { return u.borderColor }

// SetBorderColor sets the border color.
func (u *ULA) SetBorderColor(color int) {
	if color >= 0 && color <= 7 {
		u.borderColor = color
	}
}

// logTiming prints the derived ULA timing to stdout as a plain ASCII-art block.
func (u *ULA) logTiming(cfg model.Config) {
	fmt.Print(describeTiming(cfg))
}

// describeTiming renders the per-model timing as ASCII art with all values,
// dimensions and calculations.
func describeTiming(cfg model.Config) string {
	cpl := cfg.ClockPerLine()
	cef := cfg.ClockEndFrame()
	top := cfg.TopLeftPixel()
	width := cfg.FramebufferWidth()
	height := cfg.FramebufferHeight()

	var b strings.Builder
	p := func(format string, args ...any) {
		fmt.Fprintf(&b, format+"\n", args...)
	}

	p("==================================================================================")
	p(" %s  --  ULA timing", cfg.Name)
	p("==================================================================================")
	p("")
	p(" frame (from /INT at T=0):")
	p("   (INT)")
	p("    |  flyback        %d T   (%.1f lines)", cfg.FlybackTStates, float64(cfg.FlybackTStates)/float64(cpl))
	p("    +---------------------------------------+")
	p("    |  top border     %d lines   %d T", cfg.TopBorderLines, cfg.TopBorderLines*cpl)
	p("    +---------------------------------------+")
	p("    |  screen         %d lines   %d T", model.ScreenLines, model.ScreenLines*cpl)
	p("    +---------------------------------------+")
	p("    |  bottom border  %d lines   %d T", cfg.BottomBorderLines, cfg.BottomBorderLines*cpl)
	p("    +---------------------------------------+")
	p("    |  overscan       %d T", cfg.OverscanTStates)
	p("   (INT)  total %d T", cef)
	p("")
	p(" scanline (%d T):", cpl)
	p("   [blank %d][border %d][screen %d][border %d][blank %d]",
		cfg.LeftBlankTStates, cfg.LeftBorderTStates, model.ScreenTStates,
		cfg.RightBorderTStates, cfg.RightBlankTStates)
	p("")
	p(" derived:")
	p("   clockPerLine  = %d + %d + %d + %d + %d = %d",
		cfg.LeftBlankTStates, cfg.LeftBorderTStates, model.ScreenTStates,
		cfg.RightBorderTStates, cfg.RightBlankTStates, cpl)
	p("   clockEndFrame = %d + (%d+%d+%d)*%d + %d = %d",
		cfg.FlybackTStates, cfg.TopBorderLines, model.ScreenLines, cfg.BottomBorderLines,
		cpl, cfg.OverscanTStates, cef)
	p("   topLeftPixel  = %d + %d*%d + %d + %d = %d T",
		cfg.FlybackTStates, cfg.TopBorderLines, cpl, cfg.LeftBlankTStates, cfg.LeftBorderTStates, top)
	p("   /INT pulse    = %d T", cfg.InterruptLength)
	p("")
	p(" dimensions:")
	p("   framebuffer    = %d x %d px  (width x height)", width, height)
	p("   screen bitmap  = %d x %d px  (%d T x %d lines)",
		model.ScreenTStates*2, model.ScreenLines, model.ScreenTStates, model.ScreenLines)
	p("   screen origin  = x %d px, y %d lines  (top-left in framebuffer)",
		cfg.LeftBorderTStates*2, cfg.TopBorderLines)
	p("   borders        = left %d px (%d T), right %d px (%d T), top %d lines, bottom %d lines",
		cfg.LeftBorderTStates*2, cfg.LeftBorderTStates,
		cfg.RightBorderTStates*2, cfg.RightBorderTStates,
		cfg.TopBorderLines, cfg.BottomBorderLines)
	p("==================================================================================")

	return b.String()
}
