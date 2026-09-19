// Package model defines ZX Spectrum model configurations.
// Models are pure data -- no logic. Components read their behavior from Config.
package model

// ScreenTStates is the screen bitmap width in T-states (256 px at 2 px/T-state).
const ScreenTStates = 128

// ScreenLines is the screen bitmap height in lines (256x192 bitmap).
const ScreenLines = 192

// Config is a pure-data description of a ZX Spectrum model.
// Components read this to configure their behaviour -- no if/else spaghetti.
type Config struct {
	// Key is this model's entry in AllModels ("pentagon512"). It is the
	// machine's identity, distinct from Name (the display string "Pentagon
	// 512"): a state file and a replay both record the key, and a key that does
	// not match the running machine is what makes a foreign session refuse to
	// load. A test in this package keeps every Config's Key equal to its
	// AllModels entry, so the two cannot drift apart.
	Key string

	Name        string // e.g. "ZX Spectrum 128K"
	Description string

	// CPU
	CPUType string // "nmos" or "cmos"

	// Memory
	PagingModel string // "none", "128k", "2a3"

	// ULA timing (all in T-states unless noted). The frame is laid out, after
	// /INT at T=0, as:
	//
	//	[Flyback] [TopBorder lines] [Screen 192 lines] [BottomBorder lines] [Overscan]
	//
	// and every scanline as:
	//
	//	[LeftBlank] [LeftBorder] [Screen 128] [RightBorder] [RightBlank]
	//
	// which sum to ClockPerLine (derived).
	FlybackTStates     int // T-states from /INT to the top border (vertical retrace)
	LeftBlankTStates   int // T-states of blank/retrace before the left border
	LeftBorderTStates  int // T-states of left border
	RightBorderTStates int // T-states of right border
	RightBlankTStates  int // T-states of blank/retrace after the right border
	TopBorderLines     int // top border lines
	BottomBorderLines  int // bottom border lines
	OverscanTStates    int // T-states the beam is idle at the end of the frame
	InterruptLength    int // T-states /INT is held low
	InterruptOffset    int // T-state within the frame where /INT is asserted (0
	// for every model; see the note in the model files)

	// Hardware features present
	HasAY           bool
	HasTurboSound   bool // two AY chips (TurboSound add-on), replaces the single AY
	HasTurboSoundFM bool // YM2203 (OPN) FM chip (TurboSound FM), replaces the AY
	HasGS           bool // General Sound card (second Z80 + 4-channel DAC)
	HasDisk         bool
	HasPrinter      bool
	HasKempston     bool
	AYPortMask      uint16 // port decoding mask for AY chip
	HasShadowScreen bool

	// Accuracy / behaviour
	ContentionModel string // "48k", "128k", "2a3", "none" - selects the contention delay pattern (see mem.ContentionKindFor)
	HasFloatingBus  bool
	// HasSnowEffect enables the 48K snow artefact: the ULA reads its screen
	// bytes off the data bus, so its fetch collides with a CPU write to
	// contended memory and shows the CPU's byte. Authentic, noisy, and off by
	// default - no model turns it on, `-snow` does.
	HasSnowEffect bool
	NoFDCTiming   bool // disable FDC seek/rotational latency (fast, non-protected loads)
}

// ClockPerLine derives the T-states per scanline from the horizontal timing.
func (c Config) ClockPerLine() int {
	return c.LeftBlankTStates + c.LeftBorderTStates + ScreenTStates + c.RightBorderTStates + c.RightBlankTStates
}

// ClockEndFrame derives the total T-states per frame from the vertical timing.
func (c Config) ClockEndFrame() int {
	return c.FlybackTStates + (c.TopBorderLines+ScreenLines+c.BottomBorderLines)*c.ClockPerLine() + c.OverscanTStates
}

// TopLeftPixel derives the T-state (after /INT) of the screen's top-left pixel.
func (c Config) TopLeftPixel() int {
	return c.FlybackTStates + c.TopBorderLines*c.ClockPerLine() + c.LeftBlankTStates + c.LeftBorderTStates
}

// FramebufferWidth derives the framebuffer width in pixels (2 px per T-state).
func (c Config) FramebufferWidth() int {
	return (c.LeftBorderTStates + ScreenTStates + c.RightBorderTStates) * 2
}

// FramebufferHeight derives the framebuffer height in lines.
func (c Config) FramebufferHeight() int {
	return c.TopBorderLines + ScreenLines + c.BottomBorderLines
}

// AllModels is the registry of supported ZX Spectrum models.
var AllModels = map[string]Config{
	"48k":         Spectrum48K,
	"128k":        Spectrum128K,
	"2a3":         Spectrum2A3,
	"pentagon":    Pentagon128,
	"pentagon512": Pentagon512,
}
