package sound

import (
	"math"
	"testing"
)

// stereoOut records per-ear range so a test can assert stereo separation.
type stereoOut struct {
	rate       int
	minL, maxL int16
	minR, maxR int16
}

func newStereoOut(rate int) *stereoOut {
	return &stereoOut{
		rate: rate,
		minL: math.MaxInt16, maxL: math.MinInt16,
		minR: math.MaxInt16, maxR: math.MinInt16,
	}
}
func (o *stereoOut) Init() error     { return nil }
func (o *stereoOut) SampleRate() int { return o.rate }
func (o *stereoOut) Close() error    { return nil }
func (o *stereoOut) Queued() int     { return 0 }
func (o *stereoOut) HasClock() bool  { return false }
func (o *stereoOut) PushSamples(l, r []int16) error {
	for i := range l {
		if l[i] < o.minL {
			o.minL = l[i]
		}
		if l[i] > o.maxL {
			o.maxL = l[i]
		}
		if r[i] < o.minR {
			o.minR = r[i]
		}
		if r[i] > o.maxR {
			o.maxR = r[i]
		}
	}
	return nil
}
func (o *stereoOut) lRange() int32 { return int32(o.maxL) - int32(o.minL) }
func (o *stereoOut) rRange() int32 { return int32(o.maxR) - int32(o.minR) }

// TestMixerStereoPansChannels verifies a genuine StereoSource reaches the ears
// independently. TurboSound puts chip 0 on the left and chip 1 on the right, so
// a tone on chip 0 alone must drive the left ear and leave the right silent.
// (The single AY-3-8912 is mono -- the mixer centres it -- so this tests the
// TurboSound panning, not the AY.)
func TestMixerStereoPansChannels(t *testing.T) {
	out := newStereoOut(testRate)
	m := NewMixer(out, testCPU)
	ts := NewTurboSound(ayChipHz)
	m.AddSource(ts)
	programTone(ts.ay1, 504) // audible tone on chip 0 (left) only

	m.Resolve(testCPU) // one second of audio
	m.Flush()

	if out.lRange() == 0 {
		t.Fatal("left ear produced no signal")
	}
	if out.rRange() > out.lRange()/4 {
		t.Errorf("right ear carried signal: L range=%d, R range=%d (want R ~ 0)",
			out.lRange(), out.rRange())
	}
	t.Logf("stereo: L range=%d, R range=%d", out.lRange(), out.rRange())
}

// TestDCBlockRemovesConstant pins the AC-coupling filter: a constant (unipolar
// DC) input passes through on the first sample (the capacitor charging), then
// decays toward zero. This matches the series capacitor real hardware places at
// the AY output.
func TestDCBlockRemovesConstant(t *testing.T) {
	m := &Mixer{}
	var in, out int32
	m.dcBlock(&in, &out, 10000) // first sample: the DC passes through
	for i := 0; i < 10000; i++ {
		m.dcBlock(&in, &out, 10000)
	}
	if out > 100 || out < -100 {
		t.Errorf("constant input not AC-coupled: out=%d after decay", out)
	}
}
