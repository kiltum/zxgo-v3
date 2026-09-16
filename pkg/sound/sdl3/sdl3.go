// Package sdl3 is the SDL3 audio backend for zxgo-v3.
// It lives in its own package so pkg/sound builds without SDL3: the headless
// worker imports pkg/sound and uses NullOutput instead.
// Matches the approach from zxcpp and zxgo: CreateAudioStream + OpenAudioDevice + BindAudioStream.

package sdl3

/*
#cgo pkg-config: sdl3

#include <SDL3/SDL.h>

static inline int sdlAudioInit(void) { return SDL_InitSubSystem(SDL_INIT_AUDIO); }

static inline SDL_AudioStream* sdlCreateAudioStream(int freq, int channels) {
	SDL_AudioSpec spec;
	SDL_zero(spec);
	spec.format = SDL_AUDIO_S16LE;
	spec.freq = freq;
	spec.channels = channels;
	return SDL_CreateAudioStream(&spec, &spec);
}

static inline SDL_AudioDeviceID sdlOpenAudioDevice(int freq, int channels) {
	SDL_AudioSpec spec;
	SDL_zero(spec);
	spec.format = SDL_AUDIO_S16LE;
	spec.freq = freq;
	spec.channels = channels;
	return SDL_OpenAudioDevice(SDL_AUDIO_DEVICE_DEFAULT_PLAYBACK, &spec);
}

static inline int sdlBindAudioStream(SDL_AudioDeviceID dev, SDL_AudioStream *stream) {
	return SDL_BindAudioStream(dev, stream);
}

static inline int sdlPutAudioStreamData(SDL_AudioStream *stream, const void *buf, int len) {
	return SDL_PutAudioStreamData(stream, buf, len);
}

static inline void sdlCloseAudioDevice(SDL_AudioDeviceID dev) { SDL_CloseAudioDevice(dev); }

static inline void sdlDestroyAudioStream(SDL_AudioStream *stream) { SDL_DestroyAudioStream(stream); }

static inline int sdlResumeAudioDevice(SDL_AudioDeviceID dev) { return SDL_ResumeAudioDevice(dev); }

static inline int sdlGetAudioStreamQueued(SDL_AudioStream *stream) {
	return SDL_GetAudioStreamQueued(stream);
}
*/
import "C"
import (
	"log/slog"
	"unsafe"

	"github.com/kiltum/zxgo-v3/pkg/sound"
)

// audioLog is the logger for SDL audio subsystem debug output.
// nil means the caller has not wired a logger (audio operations are silent).
var audioLog *slog.Logger

// SetLogger stores the logger used by the SDL audio subsystem.
func SetLogger(log *slog.Logger) { audioLog = log }

type sdlaudio struct {
	device     C.SDL_AudioDeviceID
	stream     *C.SDL_AudioStream
	sampleRate int
	scratch    []byte // reused byte buffer for PushSamples (no per-call alloc)
}

func New(sampleRate int) sound.AudioOutput {
	return &sdlaudio{sampleRate: sampleRate}
}

func (s *sdlaudio) Init() error {
	if C.sdlAudioInit() == 0 {
		if audioLog != nil {
			audioLog.Warn("SDL audio init failed, running without audio")
		}
		return nil
	}

	s.stream = C.sdlCreateAudioStream(C.int(s.sampleRate), 2)
	if s.stream == nil {
		if audioLog != nil {
			audioLog.Warn("SDL_CreateAudioStream failed", "error", C.GoString(C.SDL_GetError()))
		}
		return nil
	}

	s.device = C.sdlOpenAudioDevice(C.int(s.sampleRate), 2)
	if s.device == 0 {
		if audioLog != nil {
			audioLog.Warn("SDL_OpenAudioDevice failed", "error", C.GoString(C.SDL_GetError()))
		}
		C.sdlDestroyAudioStream(s.stream)
		s.stream = nil
		return nil
	}

	if C.sdlBindAudioStream(s.device, s.stream) == 0 {
		if audioLog != nil {
			audioLog.Warn("SDL_BindAudioStream failed", "error", C.GoString(C.SDL_GetError()))
		}
		C.sdlCloseAudioDevice(s.device)
		C.sdlDestroyAudioStream(s.stream)
		s.device = 0
		s.stream = nil
		return nil
	}

	C.sdlResumeAudioDevice(s.device)
	if audioLog != nil {
		audioLog.Info("SDL3 audio initialized", "sampleRate", s.sampleRate, "channels", 2)
	}
	return nil
}

// PushSamples converts int16 stereo samples to interleaved bytes and pushes to SDL.
func (s *sdlaudio) PushSamples(left, right []int16) error {
	if s.stream == nil {
		return nil
	}
	n := len(left)
	if n == 0 {
		return nil
	}

	// Reuse a scratch buffer so the audio path does not allocate per drain
	// (this was the dominant allocation source: ~66% of all allocations).
	if cap(s.scratch) < n*4 {
		s.scratch = make([]byte, n*4)
	}
	buf := s.scratch[:n*4]
	for i := 0; i < n; i++ {
		l := uint16(left[i])
		r := uint16(right[i])
		buf[i*4+0] = byte(l & 0xFF)
		buf[i*4+1] = byte((l >> 8) & 0xFF)
		buf[i*4+2] = byte(r & 0xFF)
		buf[i*4+3] = byte((r >> 8) & 0xFF)
	}

	if C.sdlPutAudioStreamData(s.stream, unsafe.Pointer(&buf[0]), C.int(len(buf))) == 0 {
		if err := C.GoString(C.SDL_GetError()); err != "" {
			if audioLog != nil {
				audioLog.Warn("SDL_PutAudioStreamData failed", "error", err)
			}
		}
	}
	return nil
}

func (s *sdlaudio) SampleRate() int { return s.sampleRate }

// Queued returns the number of frames buffered in the SDL stream but not yet
// played by the device. SDL reports bytes; the stream is S16LE stereo, so one
// frame is 4 bytes. This is the signal the emulator throttles on: while it is
// above the high-water mark, production outruns the device and emulation sleeps.
func (s *sdlaudio) Queued() int {
	if s.stream == nil {
		return 0
	}
	return int(C.sdlGetAudioStreamQueued(s.stream)) / 4
}

// HasClock reports whether the backend has a live stream to pace against. A
// failed Init leaves stream nil, in which case the caller falls back to
// wall-clock pacing rather than running flat out.
func (s *sdlaudio) HasClock() bool { return s.stream != nil }

func (s *sdlaudio) Close() error {
	if s.device != 0 {
		C.sdlCloseAudioDevice(s.device)
		s.device = 0
	}
	if s.stream != nil {
		C.sdlDestroyAudioStream(s.stream)
		s.stream = nil
	}
	return nil
}

var _ sound.AudioOutput = (*sdlaudio)(nil)
