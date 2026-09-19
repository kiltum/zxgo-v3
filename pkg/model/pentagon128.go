package model

// Pentagon128 defines the Pentagon 128K, a 1991 Russian clone.
//
// Timing (authoritative: ula_timing.txt, ticks): the frame is 71680 T-states
// (derived below) versus 69888 for the 48K, and the ULA inserts no contention
// wait states. After /INT there is a 3584-tick (16 line) flyback, then 64 top
// border lines, 192 screen lines, 48 bottom border lines and no overscan.
// Horizontally each line is 32 T-states of retrace before a 36 T-state left
// border, 128 T-states of screen and a 28 T-state right border, giving a 384-px
// window (36+128+28 = 192 T-states). /INT is at T=0 and held for 36 T-states.
// There is no doubled CPU clock -- the effective rate is 71680 * 50 = 3.584 MHz,
// derived from ClockEndFrame. Paging and the Beta Disk follow the standard 128K
// layout.
var Pentagon128 = Config{
	Key:         "pentagon",
	Name:        "Pentagon 128",
	Description: "1991 Russian Pentagon 128K clone, 128K RAM, AY-3-8912",

	CPUType: "nmos",

	PagingModel:     "128k",
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
	HasTurboSound:   true, // TurboSound is ubiquitous on the Pentagon
	HasDisk:         true,
	HasPrinter:      false,
	HasKempston:     true,
	AYPortMask:      0xC002,
	HasShadowScreen: true,

	ContentionModel: "none",
	HasFloatingBus:  false,
}
