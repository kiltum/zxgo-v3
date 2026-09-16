// Package sound implements ZX Spectrum audio generation.
package sound

// AudioOutput abstracts the audio backend (SDL3, null, ...).
type AudioOutput interface {
	Init() error
	PushSamples(left, right []int16) error
	SampleRate() int
	Close() error

	// Queued returns the number of frames buffered for playback but not yet
	// consumed by the device. This is the emulator's master clock: while it
	// exceeds the high-water mark, the emulator sleeps and lets the device
	// drain. A backend without a real device returns 0.
	Queued() int

	// HasClock reports whether Queued is meaningful. A real device with a live
	// stream returns true; NullOutput and a failed audio init return false, so
	// callers fall back to wall-clock pacing.
	HasClock() bool
}

// NullOutput discards all samples. For tests and for running without audio.
type NullOutput struct{}

func (n *NullOutput) Init() error                           { return nil }
func (n *NullOutput) PushSamples(left, right []int16) error { return nil }
func (n *NullOutput) SampleRate() int                       { return 44100 }
func (n *NullOutput) Close() error                          { return nil }
func (n *NullOutput) Queued() int                           { return 0 }
func (n *NullOutput) HasClock() bool                        { return false }
