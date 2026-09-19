package model

// Spectrum48K defines the original ZX Spectrum 48K.
var Spectrum48K = Config{
	Key:         "48k",
	Name:        "ZX Spectrum 48K",
	Description: "Original 1982 model, 48K RAM",

	CPUType: "nmos",

	PagingModel:     "none",
	InterruptLength: 32,
	// /INT at the frame wrap, T=0 - the same position on every model.
	// ula_timing.txt describes the interrupt 16 T into the scanline, but its
	// line origin is 16 T before the first pixel while ours is 24 T before it
	// (LeftBlank+LeftBorder), so that 16 is not ours. Measured on a 48K with a
	// raster demo: T=0 is the value that lines the bars up (ZXGO_INT_OFFSET=0).
	InterruptOffset: 0,

	// Flyback and overscan are a calibrated pair, not derived numbers: together
	// they set where the frame wrap falls relative to the visible picture, and
	// their sum must keep the frame at exactly 69888 T-states. The raw timing
	// model wants 3560/1816; 3562/1814 is what a raster demo shows as correct, so
	// do not "correct" it back from a reference.
	FlybackTStates:     3562,
	LeftBlankTStates:   0,
	LeftBorderTStates:  24,
	RightBorderTStates: 24,
	RightBlankTStates:  48,
	TopBorderLines:     48,
	BottomBorderLines:  48,
	OverscanTStates:    1814,

	HasAY:           false,
	HasDisk:         false,
	HasPrinter:      false,
	HasKempston:     true,
	HasShadowScreen: false,

	ContentionModel: "48k",
	HasFloatingBus:  true,
}
