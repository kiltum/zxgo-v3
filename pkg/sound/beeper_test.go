package sound

import "testing"

// Rest state must match the ULA port latch after reset (0x00 written), so that
// a program's first write of 0x00 is correctly a non-event.
func TestBeeperRestState(t *testing.T) {
	b := NewBeeper()
	rest := b.MeanLevel(0, 100)
	if want := beeperLevel(false, true); rest != want {
		t.Errorf("rest level = %d, want %d", rest, want)
	}

	b2 := NewBeeper()
	b2.SetTick(50)
	b2.Write(0xFE, 0x00) // matches the reset latch -- must not create an event
	if n := b2.Pending(); n != 0 {
		t.Errorf("writing the rest value queued %d events, want 0", n)
	}
}

// A single transition followed by silence must hold the new level indefinitely.
// The old event-driven design emitted nothing until the *next* transition, so a
// program that set a level and stopped toggling produced silence (AUDIT S5).
func TestBeeperHoldsLevelAfterSingleTransition(t *testing.T) {
	b := NewBeeper()
	b.SetTick(1000)
	b.Write(0xFE, 0x10) // EAR high
	high := beeperLevel(true, true)

	if got := b.MeanLevel(2000, 2079); got != high {
		t.Errorf("level just after transition = %d, want %d", got, high)
	}
	// Far later, with no further events at all.
	if got := b.MeanLevel(5000000, 5000079); got != high {
		t.Errorf("level not held over time = %d, want %d", got, high)
	}
}

// The mean must be time-weighted across the window, which is what captures a
// transition landing between two sample points.
func TestBeeperMeanIsTimeWeighted(t *testing.T) {
	b := NewBeeper()
	b.SetTick(25)
	b.Write(0xFE, 0x10)

	rest := int64(beeperLevel(false, true))
	high := int64(beeperLevel(true, true))
	want := int16((rest*25 + high*75) / 100)

	if got := b.MeanLevel(0, 100); got != want {
		t.Errorf("time-weighted mean = %d, want %d", got, want)
	}
}

// Border-colour writes share port 0xFE. They must not queue audio events, or
// the event list grows with every border change rather than every transition.
func TestBeeperIgnoresNonAudioBits(t *testing.T) {
	b := NewBeeper()
	b.SetTick(100)
	b.Write(0xFE, 0x10) // EAR high -- one real transition
	if n := b.Pending(); n != 1 {
		t.Fatalf("after first transition Pending = %d, want 1", n)
	}
	for i, v := range []uint8{0x11, 0x12, 0x17, 0x10} { // border bits only
		b.SetTick(int64(200 + i*100))
		b.Write(0xFE, v)
	}
	if n := b.Pending(); n != 1 {
		t.Errorf("border-only writes queued extra events: Pending = %d, want 1", n)
	}
}

// EAR and MIC both drive the speaker on real hardware; engines use the pair to
// get more than two output levels. Each must move the output independently.
func TestBeeperEarAndMicAreDistinctLevels(t *testing.T) {
	seen := map[int16]string{}
	for _, c := range []struct {
		name string
		val  uint8
	}{
		{"ear=0 mic=0", 0x08},
		{"ear=0 mic=1", 0x00},
		{"ear=1 mic=0", 0x18},
		{"ear=1 mic=1", 0x10},
	} {
		b := NewBeeper()
		b.SetTick(0)
		b.Write(0xFE, c.val)
		got := b.MeanLevel(10, 110)
		if prev, dup := seen[got]; dup {
			t.Errorf("%s and %s both produce level %d; bits are not independent",
				c.name, prev, got)
		}
		seen[got] = c.name
	}
}

func TestBeeperResetReturnsToRest(t *testing.T) {
	b := NewBeeper()
	b.SetTick(1000)
	b.Write(0xFE, 0x10)
	b.SetTick(2000)
	b.Write(0xFE, 0x00)

	b.Reset()
	if n := b.Pending(); n != 0 {
		t.Errorf("Pending after Reset = %d, want 0", n)
	}
	if got, want := b.MeanLevel(0, 100), beeperLevel(false, true); got != want {
		t.Errorf("level after Reset = %d, want rest %d", got, want)
	}
}

// Events must never travel backwards in time, even if a caller rewinds the
// tick clock -- the mixer's cursor assumes a monotonic event list.
func TestBeeperEventsAreMonotonic(t *testing.T) {
	b := NewBeeper()
	b.SetTick(1000)
	b.Write(0xFE, 0x10)
	b.SetTick(500) // rewind
	b.Write(0xFE, 0x00)

	last := int64(-1)
	for _, e := range b.events {
		if e.tick < last {
			t.Fatalf("event at tick %d follows tick %d", e.tick, last)
		}
		last = e.tick
	}
}
