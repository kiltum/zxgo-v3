package emulator

import (
	"bytes"
	"hash/crc32"
	"testing"

	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/sound"
	"github.com/kiltum/zxgo-v3/pkg/state"
)

// Audio exactness. The framebuffer comparison in state_test.go cannot see a
// sound chip at all, so a chip whose generator phase was dropped would pass it
// and drift for the whole session - which is the failure STATE_DESIGN.md section
// 4 calls out and section 12 asks to be tested per device.
//
// The oracle is the sample stream itself: an AudioOutput that hashes what the
// mixer pushed during one frame. Two machines with the same chip state push the
// same samples, so a per-frame digest is to the mixer what the framebuffer
// digest is to the ULA.
//
// A silent machine would prove nothing, so each case drives its chips as it
// runs: notes on the AY (or the pairs), a tone on the beeper, a level into the
// Covox. The state has to be carried, not merely be present.

// digestOutput is a sound.AudioOutput that hashes the samples it is given.
type digestOutput struct {
	h        uint32
	pushed   int64
	sampleHz int
}

func newDigestOutput() *digestOutput { return &digestOutput{sampleHz: 44100} }

func (o *digestOutput) Init() error { return nil }
func (o *digestOutput) PushSamples(left, right []int16) error {
	var buf [4]byte
	for i := range left {
		if i < len(right) {
			putI16(buf[0:2], right[i])
		}
		putI16(buf[2:4], left[i])
		o.h = crc32.Update(o.h, crc32.IEEETable, buf[:])
		o.pushed++
	}
	return nil
}
func putI16(b []byte, v int16) {
	b[0] = byte(v)
	b[1] = byte(uint16(v) >> 8)
}
func (o *digestOutput) SampleRate() int { return o.sampleHz }
func (o *digestOutput) Close() error    { return nil }
func (o *digestOutput) Queued() int     { return 0 }
func (o *digestOutput) HasClock() bool  { return false }

// take returns the digest of the samples pushed since the last call, so a
// frame's audio can be compared the way its picture is.
func (o *digestOutput) take() (uint32, int64) {
	h, n := o.h, o.pushed
	o.h, o.pushed = 0, 0
	return h, n
}

// variant builds a Config that is a copy of a shipped model with one or two
// switches changed, so a device no model registers yet can still be exercised
// end to end (the ROM layout is chosen by name, which the copy keeps).
func variant(base string, change func(*model.Config)) model.Config {
	cfg := model.AllModels[base]
	change(&cfg)
	return cfg
}

// tickle drives every sound device the machine has, so the state being saved is
// a chip in the middle of making sound rather than a chip at rest.
func tickle(e *Emulator, frame int) {
	// AY (or the TurboSound / TurboSound FM pair): enable all three tone
	// channels, then sweep the periods. The ports are the same for all three
	// chips, which is exactly why they are mutually exclusive.
	e.WritePort(0xFFFD, 0x07)
	e.WritePort(0xBFFD, 0x38) // tone A, B and C enabled, noise off
	e.WritePort(0xFFFD, 0x08)
	e.WritePort(0xBFFD, 0x0F) // channel A at full volume
	e.WritePort(0xFFFD, 0x00)
	e.WritePort(0xBFFD, uint8(frame*7))
	e.WritePort(0xFFFD, 0x01)
	e.WritePort(0xBFFD, 0x02)
	// Envelope, so the envelope counter is somewhere other than its start.
	e.WritePort(0xFFFD, 0x0B)
	e.WritePort(0xBFFD, uint8(frame))
	e.WritePort(0xFFFD, 0x0D)
	e.WritePort(0xBFFD, 0x0E)

	// Beeper: toggle EAR every frame.
	e.WritePort(0x00FE, uint8(frame&1)<<4)

	// Covox on its Pentagon/ATM port: a level that moves.
	e.WritePort(0x00FB, uint8(frame*13))
}

func newModelWithAudio(t *testing.T, cfg model.Config, out sound.AudioOutput) *Emulator {
	t.Helper()
	e, err := NewFromModel(cfg, "roms", out)
	if err != nil {
		t.Fatalf("building %s: %v", cfg.Name, err)
	}
	return e
}

// TestRunForwardEqualityAudio is the per-device exactness test: save a machine
// with its chips running, restore it, and require the same sample stream from
// there on.
func TestRunForwardEqualityAudio(t *testing.T) {
	cases := []struct {
		name string
		key  string
	}{
		{"single-ay-48k", "48k"},
		{"single-ay-128k", "128k"},
		{"turbosound-pentagon", "pentagon512"},
	}
	for _, key := range []string{"48k", "128k"} {
		cases = append(cases, struct {
			name string
			key  string
		}{"turbosound-fm-" + key, key})
	}
	cases = append(cases, struct {
		name string
		key  string
	}{"covox-and-fm-2a3", "2a3"})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := model.AllModels[tc.key]
			switch tc.name {
			case "turbosound-fm-48k", "turbosound-fm-128k":
				cfg = variant(tc.key, func(c *model.Config) { c.HasTurboSoundFM = true })
			}

			outA := newDigestOutput()
			a := newModelWithAudio(t, cfg, outA)
			for i := 0; i < 40; i++ {
				tickle(a, i)
				a.RunFrame()
			}

			saved := saveToBytesFor(t, a, cfg.Key)
			outA.take() // the run up to the save is not compared

			outB := newDigestOutput()
			b := newModelWithAudio(t, cfg, outB)
			if _, err := b.LoadState(bytes.NewReader(saved)); err != nil {
				t.Fatalf("LoadState: %v", err)
			}
			outB.take()

			for i := 40; i < 70; i++ {
				tickle(a, i)
				tickle(b, i)
				a.RunFrame()
				b.RunFrame()

				ha, na := outA.take()
				hb, nb := outB.take()
				if na != nb {
					t.Fatalf("frame %d: %d samples, want %d", i, nb, na)
				}
				if ha != hb {
					t.Fatalf("frame %d: audio differs (%08x, want %08x) - a chip's phase was not restored", i, hb, ha)
				}
				if ga, gb := a.Mixer().Generated(), b.Mixer().Generated(); ga != gb {
					t.Fatalf("frame %d: mixer generated %d, want %d", i, gb, ga)
				}
			}
		})
	}
}

// saveToBytesFor saves a machine built from an arbitrary Config, which is the
// only difference from saveToBytes: the header carries that config's key.
func saveToBytesFor(t *testing.T, e *Emulator, key string) []byte {
	t.Helper()
	if e.cfg.Key == "" {
		t.Fatalf("config has no key")
	}
	var buf bytes.Buffer
	if err := e.SaveState(&buf, state.Options{}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	return buf.Bytes()
}

// TestSoundDeviceSectionIsPerModel: the table lists the chip the model has and
// only that one, so a file's chunk id says which chip wrote it.
func TestSoundDeviceSectionIsPerModel(t *testing.T) {
	t.Run("single AY", func(t *testing.T) {
		e := newModel(t, "128k")
		ids := sectionIDs(e)
		if !ids[state.IDAY] {
			t.Error("a single-AY machine has no IDAY section")
		}
		if ids[state.IDTurboSound] || ids[state.IDTurboSoundFM] {
			t.Error("a single-AY machine lists another chip's section")
		}
	})
	t.Run("TurboSound", func(t *testing.T) {
		e := newModel(t, "pentagon512")
		ids := sectionIDs(e)
		if !ids[state.IDTurboSound] {
			t.Error("the Pentagon has no IDTurboSound section")
		}
		if ids[state.IDAY] || ids[state.IDTurboSoundFM] {
			t.Error("the Pentagon lists another chip's section")
		}
	})
	t.Run("TurboSound FM", func(t *testing.T) {
		cfg := variant("128k", func(c *model.Config) { c.HasTurboSoundFM = true })
		e := newModelWithAudio(t, cfg, newDigestOutput())
		ids := sectionIDs(e)
		if !ids[state.IDTurboSoundFM] {
			t.Error("a TurboSound FM machine has no IDTurboSoundFM section")
		}
		if ids[state.IDAY] || ids[state.IDTurboSound] {
			t.Error("a TurboSound FM machine lists another chip's section")
		}
	})
}

func sectionIDs(e *Emulator) map[state.ID]bool {
	out := make(map[state.ID]bool)
	for _, s := range e.stateSections() {
		out[s.ID] = true
	}
	return out
}
