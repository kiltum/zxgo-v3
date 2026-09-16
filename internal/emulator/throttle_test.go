package emulator

import (
	"testing"
	"time"
)

// TestAudioClockSleep pins the pure sleep-computation that the queue-depth
// throttle loops over. It is the deterministic core of A2: the wall-clock
// deadline is gone, and these thresholds are what keep the audio device the
// master clock.
func TestAudioClockSleep(t *testing.T) {
	const rate = 44100
	highWater := int64(rate) * audioHighWaterMS / 1000 // 2205 frames

	cases := []struct {
		name   string
		queued int64
		want   time.Duration
	}{
		{"below high water", highWater - 1, 0},
		{"at high water", highWater, 0},
		{"10ms excess", highWater + rate/100, 10 * time.Millisecond},
		{"sub-millisecond excess", highWater + 10, time.Millisecond},
		{"deep queue clamps", highWater + rate, 20 * time.Millisecond},
	}

	for _, c := range cases {
		if got := audioClockSleep(c.queued, highWater, rate); got != c.want {
			t.Errorf("%s: audioClockSleep(%d) = %v, want %v", c.name, c.queued, got, c.want)
		}
	}
}
