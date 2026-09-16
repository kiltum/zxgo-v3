package mem

import "testing"

// Fuse pins its own contention tables with a checksum over the frame,
// sum(table[i] * (i+1)) over the first 80000 T-states (ref/fuse-1.9.0/
// unittests/unittests.c:63-145). Matching those numbers means our tables are
// Fuse's tables, which is far stronger than spot-checking a few T-states: the
// +2A/+3 value in particular is the one this emulator could not produce while
// the +2A/+3 was aliased onto the 128K pattern.
func TestContentionTableChecksums(t *testing.T) {
	const fuseTableSize = 80000
	cases := []struct {
		name string
		kind ContentionKind
		want uint64
	}{
		{"48K", Contention48k, 2308862976},
		{"128K", Contention128k, 2335183872},
		{"+2A/+3", Contention2A3, 3113754624},
	}
	for _, c := range cases {
		var sum uint64
		for i := 0; i < fuseTableSize; i++ {
			sum += uint64(GetContentionDelay(c.kind, i)) * uint64(i+1)
		}
		if sum != c.want {
			t.Errorf("%s contention checksum = %d, want %d", c.name, sum, c.want)
		}
	}
}

// The +2A/+3 has no contended ports (Fuse specplus3.c installs "none" for the
// no-MREQ table), so its I/O delay is zero at every T-state.
func Test2A3HasNoIOContention(t *testing.T) {
	for i := 0; i < 70908; i++ {
		if d := GetIOContentionDelay(Contention2A3, i); d != 0 {
			t.Fatalf("+2A/+3 I/O contention at T=%d = %d, want 0", i, d)
		}
	}
}

// A T-state outside the frame is a miss, not a panic - the harness config has
// no frame at all, and 0 must stay a usable answer there.
func TestContentionDelayOutOfRange(t *testing.T) {
	kinds := []ContentionKind{ContentionNone, Contention48k, Contention128k, Contention2A3}
	for _, kind := range kinds {
		for _, ts := range []int{-1, -100000, 1 << 20} {
			if d := GetContentionDelay(kind, ts); d != 0 {
				t.Errorf("kind %d at T=%d = %d, want 0", kind, ts, d)
			}
			if d := GetIOContentionDelay(kind, ts); d != 0 {
				t.Errorf("I/O kind %d at T=%d = %d, want 0", kind, ts, d)
			}
		}
	}
}

// ContentionKindFor is the single place the model string becomes a pattern, and
// an unknown name must fall back to no contention rather than to a wrong one.
func TestContentionKindFor(t *testing.T) {
	cases := map[string]ContentionKind{
		"":      ContentionNone,
		"none":  ContentionNone,
		"48k":   Contention48k,
		"128k":  Contention128k,
		"2a3":   Contention2A3,
		"bogus": ContentionNone,
		"128K":  ContentionNone, // names are exact, not case-folded
	}
	for name, want := range cases {
		if got := ContentionKindFor(name); got != want {
			t.Errorf("ContentionKindFor(%q) = %d, want %d", name, got, want)
		}
	}
}
