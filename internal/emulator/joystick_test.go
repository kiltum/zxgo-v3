package emulator

import (
	"testing"

	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/replay"
	"github.com/kiltum/zxgo-v3/pkg/sound"
)

// kempstonPort is the port the joystick answers on.
const kempstonPort = 0x1F

// The joystick is a port, so the only thing worth asserting about it is the byte
// the machine reads back: the bit layout is what a game was written against.
func TestKempstonBitLayout(t *testing.T) {
	cases := []struct {
		name string
		send func(*Emulator)
		want uint8
	}{
		{"released", func(e *Emulator) { e.SetJoystick(false, false, false, false, false) }, 0x00},
		{"right", func(e *Emulator) { e.SetJoystick(true, false, false, false, false) }, 0x01},
		{"left", func(e *Emulator) { e.SetJoystick(false, true, false, false, false) }, 0x02},
		{"down", func(e *Emulator) { e.SetJoystick(false, false, true, false, false) }, 0x04},
		{"up", func(e *Emulator) { e.SetJoystick(false, false, false, true, false) }, 0x08},
		{"fire", func(e *Emulator) { e.SetJoystick(false, false, false, false, true) }, 0x10},
		{"up-left and fire", func(e *Emulator) { e.SetJoystick(false, true, false, true, true) }, 0x1A},
	}

	e := New(model.Spectrum48K, &sound.NullOutput{})
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tc.send(e)
			if got := e.ReadPort(kempstonPort); got != tc.want {
				t.Errorf("port %#02x = %#02x, want %#02x", kempstonPort, got, tc.want)
			}
		})
	}
}

// The whole state is written at once, so a later call replaces an earlier one
// outright rather than adding to it: a direction is held only while it is being
// reported.
func TestKempstonStateIsReplacedNotAccumulated(t *testing.T) {
	e := New(model.Spectrum48K, &sound.NullOutput{})
	e.SetJoystick(true, false, false, false, true)
	e.SetJoystick(false, false, false, true, false)
	if got := e.ReadPort(kempstonPort); got != 0x08 {
		t.Errorf("port = %#02x, want just up (%#02x)", got, uint8(0x08))
	}
}

// The reset clears the port, which is why the front end writes its state back
// afterwards: a stick held across a reset is otherwise a stick the machine does
// not see.
func TestResetReleasesTheJoystick(t *testing.T) {
	e := New(model.Spectrum48K, &sound.NullOutput{})
	e.SetJoystick(false, false, false, false, true)
	e.Reset()
	if got := e.ReadPort(kempstonPort); got != 0x00 {
		t.Errorf("port after a reset = %#02x, want a released stick", got)
	}
}

// A session played with the joystick is a session that replays: the recording
// catches the port writes through the emulator's own input method, and playing
// them back leaves a fresh machine holding the same byte.
func TestJoystickReplayReproducesThePort(t *testing.T) {
	cfg := model.Spectrum48K
	script := []struct {
		frame int
		right bool
		fire  bool
	}{
		{frame: 10, right: true, fire: false},
		{frame: 20, right: true, fire: true},
		{frame: 30, right: false, fire: false},
	}

	recorded := New(cfg, &sound.NullOutput{})
	rec := replay.NewRecorder()
	recorded.SetRecorder(rec)

	byFrame := make(map[int][]struct {
		frame int
		right bool
		fire  bool
	})
	for _, step := range script {
		byFrame[step.frame] = append(byFrame[step.frame], step)
	}
	for f := 0; f < 40; f++ {
		for _, step := range byFrame[f] {
			recorded.SetJoystick(step.right, false, false, false, step.fire)
		}
		recorded.RunFrame()
	}

	events := rec.Events()
	if len(events) != len(script) {
		t.Fatalf("recorded %d events, want %d", len(events), len(script))
	}
	for i, ev := range events {
		if ev.Joy == nil {
			t.Fatalf("event %d is not a joystick event", i)
		}
	}
	// The last state recorded is the released stick, so the recorded machine ends
	// holding nothing -- and the replay has to reach the same place.
	doc := replay.NewFile()
	doc.Model = cfg.Key
	doc.CPUHz = recorded.CPUHz()
	doc.Events = events

	played := New(cfg, &sound.NullOutput{})
	played.SetPlayer(replay.NewPlayer(doc.Events, doc.CPUHz, played.CPUHz()))
	for f := 0; f < 40; f++ {
		played.RunFrame()
	}
	if !played.Player().Done() {
		t.Fatalf("replay finished with %d events unplayed", played.Player().Remaining())
	}
	if got, want := played.ReadPort(kempstonPort), recorded.ReadPort(kempstonPort); got != want {
		t.Errorf("replayed port = %#02x, recorded %#02x", got, want)
	}

	// Control: the release is what the last event carried, so a replay of the
	// events before it must leave the stick held. Without this the assertion above
	// would also pass against a machine that ignored the events entirely and ended
	// released by itself.
	held := New(cfg, &sound.NullOutput{})
	held.SetPlayer(replay.NewPlayer(events[:len(events)-1], doc.CPUHz, held.CPUHz()))
	for f := 0; f < 40; f++ {
		held.RunFrame()
	}
	if got := held.ReadPort(kempstonPort); got != 0x11 {
		t.Errorf("port after replaying up to the release = %#02x, want right and fire (%#02x)", got, uint8(0x11))
	}
}
