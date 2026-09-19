package model

// Spectrum2A3 defines the ZX Spectrum +2A/+3.
// Supports 4 ROM banks: +2A uses 2 ROMs, +3 uses 4 ROMs
// Uses dual-port paging (0x7FFD + 0x1FFD) vs single-port 128K (0x7FFD only)
var Spectrum2A3 = Config{
	Key:         "2a3",
	Name:        "ZX Spectrum +2A/+3",
	Description: "1987+2A model, 128K RAM, dual-port paging, extended disk support",

	CPUType: "nmos",

	PagingModel:     "2a3",
	InterruptLength: 32,
	// /INT at the frame wrap, T=0 - the same position on every model.
	// ula_timing.txt describes the interrupt 16 T into the scanline, but its
	// line origin is 16 T before the first pixel while ours is 24 T before it
	// (LeftBlank+LeftBorder), so that 16 is not ours. Measured on a 48K with a
	// raster demo: T=0 is the value that lines the bars up (ZXGO_INT_OFFSET=0).
	InterruptOffset: 0,

	// Flyback and overscan are a calibrated pair, not derived numbers: together
	// they set where the frame wrap falls relative to the visible picture, and
	// their sum must keep the frame at exactly 70908 T-states. The raw timing
	// model wants 3368/1876; 3396/1848 is what a raster demo shows as correct on
	// the 128K, and the +2A/+3 shares its ULA timing - check it visually, but do
	// not "correct" it back from a reference.
	FlybackTStates:     3396 + 1,
	LeftBlankTStates:   0,
	LeftBorderTStates:  24,
	RightBorderTStates: 24,
	RightBlankTStates:  52,
	TopBorderLines:     48,
	BottomBorderLines:  48,
	OverscanTStates:    1848 - 1,

	HasAY:           true,
	HasDisk:         true,
	HasPrinter:      true,
	HasKempston:     true,
	AYPortMask:      0xC002,
	HasShadowScreen: true,

	ContentionModel: "2a3",
	HasFloatingBus:  false,
}
