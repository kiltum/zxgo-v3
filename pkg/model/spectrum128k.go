package model

// Spectrum128K defines the ZX Spectrum 128K "Toastrack".
// Supports 5 ROM banks for TR-DOS (UnrealSpeccy compatible):
//
//	ROM_128_0 (bank 0), ROM_128_1/ROM_SOS (bank 1), ROM_48 (bank 2), ROM_SYS (bank 3), ROM_DOS (bank 4)
var Spectrum128K = Config{
	Key:         "128k",
	Name:        "ZX Spectrum 128K",
	Description: "1985 Toastrack model, 128K RAM, AY-3-8912",

	CPUType: "nmos",

	PagingModel:     "128k",
	InterruptLength: 32,
	// /INT at the frame wrap, T=0 - the same position on every model.
	// ula_timing.txt describes the interrupt 16 T into the scanline, but its
	// line origin is 16 T before the first pixel while ours is 24 T before it
	// (LeftBlank+LeftBorder), so that 16 is not ours. Measured on a 48K with a
	// raster demo: T=0 is the value that lines the bars up (ZXGO_INT_OFFSET=0).
	InterruptOffset: 0,

	// Flyback and overscan are a calibrated pair, not derived numbers: together
	// they set where the frame wrap falls relative to the visible picture, and
	// their sum must keep the frame at exactly 70908 T-states. The raw timing model
	// wants 3368/1876; 3396/1848 is what a raster demo shows as correct, so do not
	// "correct" it back from a reference (Fuse's floating-bus checksum, for
	// instance, implies 3394/1850 for the 128K - close, and visibly not the same).
	FlybackTStates:     3396,
	LeftBlankTStates:   0,
	LeftBorderTStates:  24,
	RightBorderTStates: 24,
	RightBlankTStates:  52,
	TopBorderLines:     48,
	BottomBorderLines:  48,
	OverscanTStates:    1848,

	HasAY:           true,
	HasDisk:         false,
	HasPrinter:      false,
	HasKempston:     true,
	AYPortMask:      0xC002,
	HasShadowScreen: true,

	ContentionModel: "128k",
	HasFloatingBus:  true,
}
