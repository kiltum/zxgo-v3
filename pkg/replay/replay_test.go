package replay

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// sample builds a small but non-trivial document: both event kinds, both key
// edges, and events deliberately out of tick order so ordering is exercised.
func sample() *File {
	return &File{
		Model:     "128k",
		ModelName: "ZX Spectrum 128K",
		CPUHz:     3546900,
		Settings:  Settings{TurboSound: true, FastTape: true},
		Media:     Media{Tape: "game.tzx"},
		Events: []Event{
			{Tick: 300, Key: &KeyEvent{Row: 6, Col: 0, Down: false}},
			{Tick: 100, Key: &KeyEvent{Row: 6, Col: 0, Down: true}},
			{Tick: 200, Tape: TapePlay},
		},
	}
}

func TestRoundTrip(t *testing.T) {
	original := sample()

	var buf bytes.Buffer
	if err := Save(&buf, original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Format != Format || loaded.Version != Version {
		t.Errorf("format/version = %s/%d, want %s/%d", loaded.Format, loaded.Version, Format, Version)
	}
	if loaded.Model != original.Model || loaded.CPUHz != original.CPUHz {
		t.Errorf("machine = %s/%d, want %s/%d", loaded.Model, loaded.CPUHz, original.Model, original.CPUHz)
	}
	if loaded.Settings != original.Settings {
		t.Errorf("settings = %+v, want %+v", loaded.Settings, original.Settings)
	}
	if loaded.Media != original.Media {
		t.Errorf("media = %+v, want %+v", loaded.Media, original.Media)
	}
	if len(loaded.Events) != len(original.Events) {
		t.Fatalf("got %d events, want %d", len(loaded.Events), len(original.Events))
	}
	// Save sorts, so the reloaded order is tick order rather than input order.
	for i, want := range []int64{100, 200, 300} {
		if got := loaded.Events[i].Tick; got != want {
			t.Errorf("event %d tick = %d, want %d", i, got, want)
		}
	}
	if k := loaded.Events[0].Key; k == nil || k.Row != 6 || k.Col != 0 || !k.Down {
		t.Errorf("event 0 key = %+v, want the (6,0) press", k)
	}
	if got := loaded.Events[1].Tape; got != TapePlay {
		t.Errorf("event 1 tape = %q, want %q", got, TapePlay)
	}
	if got := loaded.Events[2].Key; got == nil || got.Down {
		t.Errorf("event 2 key = %+v, want the (6,0) release", got)
	}
}

// The events survive a second save byte for byte, which is what makes a file
// diffable and a re-recorded session comparable.
func TestSaveIsStable(t *testing.T) {
	f := sample()
	var first bytes.Buffer
	if err := Save(&first, f); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(bytes.NewReader(first.Bytes()))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var second bytes.Buffer
	if err := Save(&second, loaded); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Errorf("re-save differs:\n--- first\n%s\n--- second\n%s", first.Bytes(), second.Bytes())
	}
}

// A release is written as down:false rather than being dropped by omitempty:
// a key that goes down and never comes up is a different session.
func TestReleaseKeepsDownField(t *testing.T) {
	var buf bytes.Buffer
	if err := Save(&buf, &File{Model: "48k", Events: []Event{
		{Tick: 5, Key: &KeyEvent{Row: 2, Col: 3, Down: false}},
	}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !strings.Contains(buf.String(), `"down": false`) {
		t.Errorf("release event lost its down field:\n%s", buf.String())
	}
}

func TestLoadRejects(t *testing.T) {
	full := func() *File { return sample() }

	write := func(t *testing.T, mutate func(*File) string) string {
		t.Helper()
		f := full()
		if replacement := mutate(f); replacement != "" {
			return replacement
		}
		var buf bytes.Buffer
		if err := Save(&buf, f); err != nil {
			t.Fatalf("Save: %v", err)
		}
		return buf.String()
	}

	cases := []struct {
		name string
		doc  string
		want string
	}{
		{"not json", "this is not a replay at all", "read replay file"},
		{"empty", "", "read replay file"},
		{"wrong format", `{"format":"something-else","version":1,"model":"48k"}`, "not a replay file"},
		{"no format", `{"version":1,"model":"48k"}`, "not a replay file"},
		{"future version", `{"format":"zxgo-replay","version":99,"model":"48k"}`, "unsupported replay version"},
		{"zero version", `{"format":"zxgo-replay","version":0,"model":"48k"}`, "unsupported replay version"},
		{"no model", `{"format":"zxgo-replay","version":1}`, "names no model"},
		{"negative tick", `{"format":"zxgo-replay","version":1,"model":"48k","events":[{"tick":-1,"key":{"row":0,"col":0,"down":true}}]}`, "negative tick"},
		{"row out of range", `{"format":"zxgo-replay","version":1,"model":"48k","events":[{"tick":1,"key":{"row":8,"col":0,"down":true}}]}`, "outside the 8x5 matrix"},
		{"col out of range", `{"format":"zxgo-replay","version":1,"model":"48k","events":[{"tick":1,"key":{"row":0,"col":5,"down":true}}]}`, "outside the 8x5 matrix"},
		{"unknown tape action", `{"format":"zxgo-replay","version":1,"model":"48k","events":[{"tick":1,"tape":"rewind"}]}`, "unknown tape action"},
		{"both fields", `{"format":"zxgo-replay","version":1,"model":"48k","events":[{"tick":1,"key":{"row":0,"col":0,"down":true},"tape":"play"}]}`, "both a key and a tape action"},
		{"neither field", `{"format":"zxgo-replay","version":1,"model":"48k","events":[{"tick":1}]}`, "neither a key nor a tape action"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(strings.NewReader(tc.doc))
			if err == nil {
				t.Fatalf("Load accepted %s", tc.doc)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}

	// A valid document still loads, so the table above is not passing by
	// rejecting everything.
	if _, err := Load(strings.NewReader(write(t, func(*File) string { return "" }))); err != nil {
		t.Fatalf("Load of a valid document failed: %v", err)
	}
}

// A hand-edited file whose events are out of order plays in the order they
// happened, not the order they appear.
func TestLoadSortsEvents(t *testing.T) {
	doc := `{"format":"zxgo-replay","version":1,"model":"48k","events":[
		{"tick":900,"tape":"pause"},
		{"tick":100,"tape":"play"},
		{"tick":500,"tape":"pause"}]}`

	f, err := Load(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for i, want := range []int64{100, 500, 900} {
		if got := f.Events[i].Tick; got != want {
			t.Errorf("event %d tick = %d, want %d", i, got, want)
		}
	}
}

func TestRecorder(t *testing.T) {
	r := NewRecorder()
	r.Key(300, 6, 0, false)
	r.Key(100, 6, 0, true)
	r.Tape(200, TapePlay)

	if r.Len() != 3 {
		t.Fatalf("Len = %d, want 3", r.Len())
	}
	events := r.Events()
	for i, want := range []int64{100, 200, 300} {
		if events[i].Tick != want {
			t.Errorf("event %d tick = %d, want %d", i, events[i].Tick, want)
		}
	}

	// Events() hands back a copy: mutating it must not disturb the recorder.
	events[0].Tick = 999999
	if r.Events()[0].Tick != 100 {
		t.Error("Events() returned the recorder's own slice")
	}

	// Disabled, nothing more is captured.
	r.Enable(false)
	r.Key(400, 1, 1, true)
	if r.Len() != 3 {
		t.Errorf("Len = %d after a disabled Key, want 3", r.Len())
	}
	r.Enable(true)
	r.Key(400, 1, 1, true)
	if r.Len() != 4 {
		t.Errorf("Len = %d after re-enabling, want 4", r.Len())
	}
}

func TestRecorderFile(t *testing.T) {
	r := NewRecorder()
	r.Key(10, 0, 0, true)

	// The document is assembled the way the CLI does it: the recorder supplies
	// the events, NewFile supplies the envelope, the caller the machine.
	f := NewFile()
	f.Model = "48k"
	f.CPUHz = 3500000
	f.Events = r.Events()

	if f.Format != Format || f.Version != Version {
		t.Errorf("File format/version = %s/%d", f.Format, f.Version)
	}
	if f.Created.IsZero() {
		t.Error("NewFile left the timestamp zero")
	}
	if len(f.Events) != 1 {
		t.Fatalf("File has %d events, want 1", len(f.Events))
	}
	// A document built this way must be loadable as it stands.
	var buf bytes.Buffer
	if err := Save(&buf, f); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := Load(&buf)
	if err != nil {
		t.Fatalf("Load of a recorded document: %v", err)
	}
	if len(loaded.Events) != 1 || loaded.Events[0].Key == nil || !loaded.Events[0].Key.Down {
		t.Errorf("reloaded events = %+v, want the single press", loaded.Events)
	}
}

// Save repairs a document whose envelope was built by hand rather than by
// NewFile, so no file is ever written without a marker or a version.
func TestSaveStampsEnvelope(t *testing.T) {
	var buf bytes.Buffer
	if err := Save(&buf, &File{Model: "48k"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	var probe struct {
		Format  string `json:"format"`
		Version int    `json:"version"`
		Created string `json:"created"`
	}
	if err := json.Unmarshal(buf.Bytes(), &probe); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if probe.Format != Format || probe.Version != Version {
		t.Errorf("format/version = %q/%d, want %q/%d", probe.Format, probe.Version, Format, Version)
	}
	if probe.Created == "" {
		t.Error("no created timestamp was written")
	}
}

// Player hands events over in tick order and stops when it runs out.
func TestPlayerAppliesInOrder(t *testing.T) {
	events := []Event{
		{Tick: 100, Tape: TapePlay},
		{Tick: 250, Key: &KeyEvent{Row: 1, Col: 2, Down: true}},
		{Tick: 400, Tape: TapePause},
	}
	p := NewPlayer(events, 3500000, 3500000)

	var got []int64
	// Nothing is due before the first event.
	if n := p.Apply(99, func(e Event) { got = append(got, e.Tick) }); n != 0 {
		t.Errorf("Apply(99) ran %d events, want 0", n)
	}
	// An event at exactly the current tick is due: that is the boundary the
	// recorder stamped it at.
	if n := p.Apply(100, func(e Event) { got = append(got, e.Tick) }); n != 1 {
		t.Errorf("Apply(100) ran %d events, want 1", n)
	}
	// A jump of several events at once still runs them in order.
	if n := p.Apply(1000, func(e Event) { got = append(got, e.Tick) }); n != 2 {
		t.Errorf("Apply(1000) ran %d events, want 2", n)
	}
	if want := []int64{100, 250, 400}; len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("event %d tick = %d, want %d", i, got[i], want[i])
			}
		}
	}
	if !p.Done() || p.Remaining() != 0 {
		t.Errorf("Done = %v, Remaining = %d, want true/0", p.Done(), p.Remaining())
	}
	if p.Len() != 3 {
		t.Errorf("Len = %d, want 3", p.Len())
	}
}

// Playing a recording made on one model on another moves the timestamps by the
// clock ratio, so the session keeps its shape instead of slipping.
func TestPlayerRescalesAcrossModels(t *testing.T) {
	// 3.5 MHz (48K) to 3.584 MHz (Pentagon): every tick is worth slightly more
	// there, so the same moment is a slightly larger tick number.
	events := []Event{
		{Tick: 3500000, Tape: TapePlay},  // one second at 3.5 MHz
		{Tick: 7000000, Tape: TapePause}, // two seconds
	}
	p := NewPlayer(events, 3500000, 3584000)

	var got []int64
	p.Apply(1<<62, func(e Event) { got = append(got, e.Tick) })
	want := []int64{3584000, 7168000}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d tick = %d, want %d", i, got[i], want[i])
		}
	}
}

// The same clock on both sides leaves the events untouched -- the normal case,
// since a replay applies the model it was recorded on.
func TestPlayerSameClockIsExact(t *testing.T) {
	events := []Event{
		{Tick: 12345, Key: &KeyEvent{Row: 6, Col: 0, Down: true}},
		{Tick: 67890, Key: &KeyEvent{Row: 6, Col: 0, Down: false}},
	}
	p := NewPlayer(events, 3546900, 3546900)

	var got []int64
	p.Apply(1<<62, func(e Event) { got = append(got, e.Tick) })
	if got[0] != 12345 || got[1] != 67890 {
		t.Errorf("got %v, want the recorded ticks verbatim", got)
	}
}

// An unknown clock on either side means no rescale rather than a divide by zero.
func TestPlayerWithoutClocks(t *testing.T) {
	events := []Event{{Tick: 500, Tape: TapePlay}}

	for _, tc := range []struct {
		name           string
		recorded, want int
	}{
		{"recorded unknown", 0, 500},
		{"target unknown", 3500000, 500},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			target := 3500000
			if tc.name == "target unknown" {
				target = 0
			}
			p := NewPlayer(events, tc.recorded, target)
			var got int64 = -1
			p.Apply(1<<62, func(e Event) { got = e.Tick })
			if got != int64(tc.want) {
				t.Errorf("tick = %d, want %d", got, tc.want)
			}
		})
	}
}

// The rescale must not mutate the caller's slice: the same events may be played
// twice at different clocks.
func TestPlayerRescaleDoesNotMutate(t *testing.T) {
	events := []Event{{Tick: 3500000, Tape: TapePlay}}
	NewPlayer(events, 3500000, 3584000)
	if events[0].Tick != 3500000 {
		t.Errorf("source event was modified: tick = %d", events[0].Tick)
	}
}

func TestFileCounts(t *testing.T) {
	f := sample()
	keys, tape := f.Count()
	if keys != 2 || tape != 1 {
		t.Errorf("Count = %d keys, %d tape, want 2 and 1", keys, tape)
	}
	if got := f.Duration(); got != 300 {
		t.Errorf("Duration = %d, want 300", got)
	}
	if got := (&File{}).Duration(); got != 0 {
		t.Errorf("Duration of an empty file = %d, want 0", got)
	}
}

// An empty session is a valid file with an empty event list, not a null one:
// "recorded nothing" and "malformed" must not look alike in the JSON.
func TestSaveEmptyEventsIsArray(t *testing.T) {
	var buf bytes.Buffer
	if err := Save(&buf, &File{Model: "48k"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	var probe struct {
		Events []Event `json:"events"`
	}
	if err := json.Unmarshal(buf.Bytes(), &probe); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if probe.Events == nil {
		t.Errorf("events encoded as null:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), `"events": []`) {
		t.Errorf("events is not an empty array:\n%s", buf.String())
	}
}
