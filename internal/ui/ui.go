// Package ui defines the pluggable UI backend interface for zxgo-v3.
// Implementations: internal/ui/sdl (interactive) and the null backend in this
// package (headless/CI). This package carries no cgo, so importing it does not
// pull in SDL3.
package ui

// UI is the interface that any rendering+input backend must implement.
type UI interface {
	// Init initializes the UI (creates window, renderer, texture).
	Init() error

	// ProcessEvents processes pending input events without blocking.
	// Returns false if the user requested quit (close window, Cmd+Q).
	// onKey handles ZX Spectrum keyboard matrix keys.
	// onCommand handles emulator commands (tape control, etc).
	ProcessEvents(onKey func(row, col int, pressed bool), onCommand func(command string, pressed bool)) bool

	// RenderFrame uploads the ARGB8888 framebuffer (width*height pixels) and
	// presents it. Dimensions are fixed at construction.
	RenderFrame(screen []uint32)

	// SetTitle sets the window title (e.g. for FPS display).
	SetTitle(title string)

	// Destroy cleans up SDL resources.
	Destroy()
}
