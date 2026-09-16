package model

// Pentagon512 defines the Pentagon 512K: the same ULA timing and paging base as
// the Pentagon 128, but with 32 RAM banks (512K) selected by a 5-bit page via
// port 0x7FFD (see pkg/mem/banking.Pentagon512).
var Pentagon512 = Config{
	Name:        "Pentagon 512",
	Description: "Pentagon 512K clone, 512K RAM, AY-3-8912",

	CPUType: "nmos",

	PagingModel:     "pentagon512",
	InterruptLength: 36,
	InterruptOffset: 0, // /INT at the start of the flyback, T=0 - the Pentagon
	// differs from the Sinclair models here (ula_timing.txt)

	FlybackTStates:     3584,
	LeftBlankTStates:   32,
	LeftBorderTStates:  36,
	RightBorderTStates: 28,
	RightBlankTStates:  0,
	TopBorderLines:     64,
	BottomBorderLines:  48,
	OverscanTStates:    0,

	HasAY:           true,
	HasTurboSound:   true,
	HasDisk:         true,
	HasPrinter:      false,
	HasKempston:     true,
	AYPortMask:      0xC002,
	HasShadowScreen: true,

	ContentionModel: "none",
	HasFloatingBus:  false,
}
