package emulator

import (
	"bytes"
	"testing"

	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/snap"
	"github.com/kiltum/zxgo-v3/pkg/sound"
)

// banked fills every RAM bank of a machine with a pattern unique to that bank,
// so a snapshot that loses or misplaces one shows up as a mismatch rather than
// as plausible-looking bytes.
func banked(t *testing.T, e *Emulator) {
	t.Helper()
	for b := 0; b < 8; b++ {
		bank := e.Mapper().GetBank(b)
		if bank == nil {
			continue
		}
		for i := range bank {
			bank[i] = uint8(b<<4 | (i & 0x0F))
		}
	}
}

func paging(t *testing.T, e *Emulator, value uint8) {
	t.Helper()
	w, ok := e.Mapper().(interface{ WritePort(uint16, uint8) })
	if !ok {
		t.Fatal("mapper does not accept paging writes")
	}
	w.WritePort(0x7FFD, value)
}

// roundTrip saves src, loads the file into a fresh machine and compares the
// two SNA images byte for byte. Comparing the *files* rather than the banks
// sidesteps the format's own convention of parking the program counter on the
// stack, which a loader legitimately leaves in memory - a second save writes it
// again, in the same place, with the same value, so an identical image means
// nothing was lost: not a bank, not the paging, not the port.
func roundTrip(t *testing.T, cfg model.Config, p7ffd uint8) {
	t.Helper()

	src := New(cfg, &sound.NullOutput{})
	banked(t, src)
	if p7ffd != 0 {
		paging(t, src, p7ffd)
	}
	first := saveBytes(t, src)

	loaded, err := snap.LoadSNA(bytes.NewReader(first))
	if err != nil {
		t.Fatalf("LoadSNA: %v", err)
	}
	dst := New(cfg, &sound.NullOutput{})
	if err := dst.LoadSNA(loaded); err != nil {
		t.Fatalf("apply: %v", err)
	}

	second := saveBytes(t, dst)
	if !bytes.Equal(first, second) {
		n := len(first)
		if len(second) < n {
			n = len(second)
		}
		for i := 0; i < n; i++ {
			if first[i] != second[i] {
				t.Fatalf("SNA differs at byte %d (0x%02X vs 0x%02X); %d vs %d bytes",
					i, first[i], second[i], len(first), len(second))
			}
		}
		t.Fatalf("SNA length differs: %d vs %d bytes", len(first), len(second))
	}
	if got := dst.Mapper().(interface{ PagingValue() uint8 }).PagingValue(); got != p7ffd {
		t.Errorf("paging = 0x%02X after load, want 0x%02X", got, p7ffd)
	}
}

func saveBytes(t *testing.T, e *Emulator) []byte {
	t.Helper()
	s, err := e.CreateSNA()
	if err != nil {
		t.Fatalf("CreateSNA: %v", err)
	}
	var buf bytes.Buffer
	if err := snap.SaveSNA(&buf, s); err != nil {
		t.Fatalf("SaveSNA: %v", err)
	}
	return buf.Bytes()
}

// A 128K machine must survive a round trip through an SNA file with its paging
// and every RAM bank intact. It could not before: CreateSNA wrote the 48K shape
// whatever the machine was, and LoadSNA ignored the extended fields, so the
// paging register and four banks were silently lost.
func TestSNARoundTrip128K(t *testing.T) {
	// Bank 7 at 0xC000: the window then shows banks 5, 2 and 7, leaving exactly
	// the five the format has room for.
	roundTrip(t, model.Spectrum128K, 0x07)
}

// The 48K stays on the 48K form: a file the same size as it always was.
func TestSNARoundTrip48K(t *testing.T) {
	roundTrip(t, model.Spectrum48K, 0)

	e := New(model.Spectrum48K, &sound.NullOutput{})
	banked(t, e)
	if got := len(saveBytes(t, e)); got != 49179 {
		t.Errorf("48K SNA is %d bytes, want 49179", got)
	}
}

// A snapshot for a banked machine must not be applied to a 48K: the banks and
// the paging register have nowhere to go, and silently loading the 48K window
// would look like it worked. The other direction stays allowed - a 48K program
// runs on a 128K, which is the common case.
func TestSnapshotClassMismatchRefused(t *testing.T) {
	src := New(model.Spectrum128K, &sound.NullOutput{})
	banked(t, src)
	paging(t, src, 0x07)
	file := saveBytes(t, src)

	s48 := New(model.Spectrum48K, &sound.NullOutput{})
	loaded, err := snap.LoadSNA(bytes.NewReader(file))
	if err != nil {
		t.Fatalf("LoadSNA: %v", err)
	}
	if err := s48.LoadSNA(loaded); err == nil {
		t.Error("loading a 128K snapshot into a 48K should be refused")
	}

	// The same file into a banked machine is fine.
	if err := New(model.Spectrum128K, &sound.NullOutput{}).LoadSNA(loaded); err != nil {
		t.Errorf("loading a 128K snapshot into a 128K failed: %v", err)
	}

	// And a 48K snapshot into a 128K is allowed, not refused.
	small := New(model.Spectrum48K, &sound.NullOutput{})
	banked(t, small)
	smallFile, err := snap.LoadSNA(bytes.NewReader(saveBytes(t, small)))
	if err != nil {
		t.Fatalf("LoadSNA: %v", err)
	}
	if err := New(model.Spectrum128K, &sound.NullOutput{}).LoadSNA(smallFile); err != nil {
		t.Errorf("a 48K snapshot should load on a 128K: %v", err)
	}
}
