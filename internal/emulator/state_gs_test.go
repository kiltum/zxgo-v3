package emulator

import (
	"bytes"
	"testing"

	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/state"
)

// General Sound. The card is a machine inside the machine - its own Z80, its own
// 512K, its own DAC - so its state is a machine's worth of state, and a session
// has to carry all of it.
//
// The adapter is driven the way the hardware is: the card's DAC channels are
// memory-mapped, and a write to the low page updates a channel as well as the
// RAM, so writing to the card's memory produces real audio from it. Its Z80 runs
// the card's ROM from reset, so its registers and RAM move on their own as well.

func gsMachine(t *testing.T) (*Emulator, *digestOutput) {
	t.Helper()
	cfg := variant("128k", func(c *model.Config) { c.HasGS = true })
	out := newDigestOutput()
	e := newModelWithAudio(t, cfg, out)
	if e.GS() == nil {
		t.Fatal("a machine with HasGS has no General Sound card")
	}
	return e, out
}

// driveGS writes a moving waveform into the card's first DAC channel and pokes
// its host ports, so the card's Z80, its RAM and its DAC event ring are all
// doing something when the state is taken.
func driveGS(e *Emulator, frame int) {
	g := e.GS()
	for i := 0; i < 16; i++ {
		// Channel 1 is selected by bits 8 and 9 of the address.
		g.WriteMemory(uint16(0x0100|(i*8)), uint8(128+int(100*sinApprox(frame*8+i))))
	}
	// Host handshake: a command byte and a data byte, which is how a player
	// talks to the card.
	e.WritePort(0x00BB, uint8(0x80|(frame&0x0F)))
	e.WritePort(0x00B3, uint8(frame*7))
}

// sinApprox is a cheap triangle wave, so the test does not need math and the
// values are reproducible everywhere.
func sinApprox(x int) float64 {
	p := x % 64
	if p < 0 {
		p += 64
	}
	v := float64(p) / 32
	if v > 1 {
		v = 2 - v
	}
	return 2*v - 1
}

func TestGeneralSoundSurvivesRestore(t *testing.T) {
	original, outA := gsMachine(t)
	for i := 0; i < 30; i++ {
		driveGS(original, i)
		original.RunFrame()
	}
	wantPC := original.GS().CPU().PC
	saved := saveToBytes(t, original)
	outA.take()

	restored, outB := gsMachine(t)
	if _, err := restored.LoadState(bytes.NewReader(saved)); err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if got := restored.GS().CPU().PC; got != wantPC {
		t.Errorf("GS Z80 PC = %04x, want %04x", got, wantPC)
	}
	outB.take()

	// Run both and compare what the Spectrum sees and what the card produces.
	for i := 30; i < 60; i++ {
		driveGS(original, i)
		driveGS(restored, i)
		original.RunFrame()
		restored.RunFrame()

		if a, b := original.TotalTicks(), restored.TotalTicks(); a != b {
			t.Fatalf("frame %d: ticks %d, want %d", i, b, a)
		}
		if a, b := screenDigest(original), screenDigest(restored); a != b {
			t.Fatalf("frame %d: framebuffer differs (%08x, want %08x)", i, b, a)
		}
		ha, na := outA.take()
		hb, nb := outB.take()
		if na != nb {
			t.Fatalf("frame %d: %d audio samples, want %d", i, nb, na)
		}
		if ha != hb {
			t.Fatalf("frame %d: audio differs (%08x, want %08x) - the card's DAC or its Z80 did not come back",
				i, hb, ha)
		}
		if a, b := original.GS().CPU().PC, restored.GS().CPU().PC; a != b {
			t.Fatalf("frame %d: GS Z80 PC %04x, want %04x", i, b, a)
		}
	}
}

// TestGeneralSoundIsInTheTableOnlyWhenFitted: the card's chunk is there when the
// machine has the card and not otherwise, so a file says which it was.
func TestGeneralSoundIsInTheTableOnlyWhenFitted(t *testing.T) {
	plain := newModel(t, "128k")
	if ids := sectionIDs(plain); ids[state.IDGeneralSound] || ids[state.IDCPUGS] {
		t.Error("a machine with no General Sound lists its chunks")
	}

	withGS, _ := gsMachine(t)
	ids := sectionIDs(withGS)
	if !ids[state.IDGeneralSound] {
		t.Error("a machine with a General Sound has no section for it")
	}
}

// TestGeneralSoundRAMIsRestoredDirectly checks the card's RAM block itself,
// rather than inferring it from the audio: a byte written to a high channel
// address lands in a mapped page, and the restored card has to see it there.
func TestGeneralSoundRAMIsRestoredDirectly(t *testing.T) {
	original, _ := gsMachine(t)
	runFrames(original, 5)

	// A marker through the card's own memory. 0x4000 is the fixed RAM block:
	// the low page is the card's ROM, where a write is refused (it only pokes a
	// DAC channel, which is what the hardware does).
	g := original.GS()
	g.WriteMemory(0x4000, 0x5A)
	if got := g.ReadMemory(0x4000); got != 0x5A {
		t.Fatalf("reading back the marker gave %02x", got)
	}

	saved := saveToBytes(t, original)

	restored, _ := gsMachine(t)
	if _, err := restored.LoadState(bytes.NewReader(saved)); err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if got := restored.GS().ReadMemory(0x4000); got != 0x5A {
		t.Errorf("the card's RAM byte came back as %02x, want 5a", got)
	}
}
