package emulator

import (
	"bytes"
	"testing"

	"github.com/kiltum/zxgo-v3/pkg/snap"
)

// TestSnapshotRoundTrip saves the state after a boot, serializes it to the SNA
// byte format and back, loads it into a fresh emulator, and checks that the
// registers match. This pins the fixes for three round-trip bugs: the PC was
// never stored (SNA keeps it on the stack), the SP was off by 2, and the IM was
// corrupted by packing it into the same bits as the border colour.
func TestSnapshotRoundTrip(t *testing.T) {
	src := newTestEmu(t)
	for i := 0; i < 200; i++ {
		src.RunFrame()
	}

	s, err := src.CreateSNA()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := snap.SaveSNA(&buf, s); err != nil {
		t.Fatal(err)
	}
	loaded, err := snap.LoadSNA(&buf)
	if err != nil {
		t.Fatal(err)
	}

	dst := newTestEmu(t)
	if err := dst.LoadSNA(loaded); err != nil {
		t.Fatal(err)
	}

	a, b := src.CPU(), dst.CPU()
	checks := []struct {
		name string
		got  uint16
		want uint16
	}{
		{"PC", b.PC, a.PC},
		{"SP", b.SP, a.SP},
		{"AF", uint16(b.A)<<8 | uint16(b.F), uint16(a.A)<<8 | uint16(a.F)},
		{"BC", uint16(b.B)<<8 | uint16(b.C), uint16(a.B)<<8 | uint16(a.C)},
		{"DE", uint16(b.D)<<8 | uint16(b.E), uint16(a.D)<<8 | uint16(a.E)},
		{"HL", uint16(b.H)<<8 | uint16(b.L), uint16(a.H)<<8 | uint16(a.L)},
		{"IX", b.IX, a.IX},
		{"IY", b.IY, a.IY},
		{"IM", uint16(b.IM), uint16(a.IM)},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = 0x%04X, want 0x%04X", c.name, c.got, c.want)
		}
	}
	if b.IFF1 != a.IFF1 || b.IFF2 != a.IFF2 {
		t.Errorf("IFF1/IFF2 = %v/%v, want %v/%v", b.IFF1, b.IFF2, a.IFF1, a.IFF2)
	}
}
