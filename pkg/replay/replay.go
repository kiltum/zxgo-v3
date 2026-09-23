// Package replay stores a session as a file another person can play back.
//
// A .replay records what was pressed and when, together with the machine the
// session ran on, so that loading the file reproduces the session rather than
// merely suggesting it. It is the counterpart of the snapshot formats: where
// those capture a machine's state at one instant, this captures the input that
// carried the machine between states.
//
// Two decisions shape everything here.
//
// Events are stamped in T-states, never wall-clock. Fast tape loading is the
// proof of why: -fast-tape does not change the emulated timeline at all, it only
// removes the host-side sleep, so the same load finishes at the same tick on a
// host running it at 50x and on one running it at 10x. Host time would make the
// same session replay minutes of emulated time out of step on any machine but
// the recording one. T-states are also the unit the rest of the emulator uses,
// so a recorded tick is exactly the point playback injects at, with no
// conversion to round.
//
// Events are a tagged union, not a flat key record. CMD+P never reaches the
// keyboard matrix -- it is swallowed as a host command -- so a format holding
// only (row, col, down) could not express the one gesture that drives fast
// loading. Key and tape events share one time-ordered list; exactly one field of
// each event is set, so there is no ambiguous case to decode.
package replay

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"
)

// Format is the marker every .replay file carries, so a wrong file is rejected
// by name rather than by a confusing parse failure somewhere downstream.
const Format = "zxgo-replay"

// Version is the format revision this package writes. Load accepts anything up
// to it and refuses anything newer: a file from a later emulator may use fields
// this build would silently ignore.
const Version = 1

// Tape actions an event may carry.
const (
	TapePlay  = "play"
	TapePause = "pause"
)

// File is a whole .replay document.
type File struct {
	Format  string    `json:"format"`
	Version int       `json:"version"`
	Created time.Time `json:"created"`

	// Model is the registry key ("48k", "128k", "2a3", "pentagon") the session
	// ran on; ModelName is the same machine's display name, for a human reading
	// the file. CPUHz is that machine's clock, which is what lets a replay
	// played on a different model rescale its timestamps instead of drifting.
	Model     string `json:"model"`
	ModelName string `json:"model_name,omitempty"`
	CPUHz     int    `json:"cpu_hz"`

	Settings Settings `json:"settings"`
	Media    Media    `json:"media"`
	Events   []Event  `json:"events"`
}

// Settings are the machine switches a session was run with.
//
// These are recorded individually rather than as a model.Config because they are
// not properties of any model: no entry in model.AllModels turns on the General
// Sound card or the snow artefact, only a flag does. A replay recorded with -gs
// would otherwise play back on a machine without it.
type Settings struct {
	TurboSound   bool `json:"turbosound"`
	TurboSoundFM bool `json:"turbosoundfm"`
	GeneralSound bool `json:"gs"`
	Snow         bool `json:"snow"`
	NoFDCTiming  bool `json:"no_fdc_timing"`
	FastTape     bool `json:"fast_tape"`
}

// Media names the images the session had mounted, exactly as they were given on
// the command line. The paths are hints, not requirements: a replay whose tape
// is missing still plays its keys.
type Media struct {
	Tape     string `json:"tape,omitempty"`
	Disk     string `json:"disk,omitempty"`
	Snapshot string `json:"snapshot,omitempty"`
}

// Event is one thing that happened, at one tick. Exactly one of Key, Joy and
// Tape is set.
type Event struct {
	Tick int64     `json:"tick"`
	Key  *KeyEvent `json:"key,omitempty"`
	Joy  *JoyEvent `json:"joy,omitempty"`
	Tape string    `json:"tape,omitempty"`
}

// KeyEvent is a keyboard matrix change: a raw (row, col) cell, not a host
// scancode. Storing the matrix coordinate is what makes a replay independent of
// the keyboard layout and the platform it was recorded on -- a press is the same
// cell on a Mac and a PC, while SDL scancodes are neither.
type KeyEvent struct {
	Row  int  `json:"row"`
	Col  int  `json:"col"`
	Down bool `json:"down"`
}

// JoyEvent is the Kempston joystick as it stood after a change: the five
// directions the port has bits for, not the host control that moved. A pad's
// stick, its D-pad and a keyboard cursor key all arrive here as the same
// direction, which is why the recorded file does not say which was used.
//
// It is a whole state rather than a change to one direction because a Kempston
// port is read as a byte: replaying an event means writing the port, and there
// is nothing to be gained by replaying five single-bit deltas whose intermediate
// combinations no game ever sees.
type JoyEvent struct {
	Right bool `json:"right,omitempty"`
	Left  bool `json:"left,omitempty"`
	Down  bool `json:"down,omitempty"`
	Up    bool `json:"up,omitempty"`
	Fire  bool `json:"fire,omitempty"`
}

// Load reads a .replay document, rejecting anything that is not one or comes
// from a later format revision. Events are sorted by tick, so a hand-edited file
// that is out of order still plays in the order it happened.
func Load(r io.Reader) (*File, error) {
	var f File
	dec := json.NewDecoder(r)
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("read replay file: %w", err)
	}
	if f.Format != Format {
		return nil, fmt.Errorf("not a replay file: format is %q, want %q", f.Format, Format)
	}
	if f.Version < 1 || f.Version > Version {
		return nil, fmt.Errorf("unsupported replay version %d (this build reads up to %d)",
			f.Version, Version)
	}
	if f.Model == "" {
		return nil, fmt.Errorf("replay file names no model")
	}
	for i, ev := range f.Events {
		if err := ev.validate(); err != nil {
			return nil, fmt.Errorf("event %d: %w", i, err)
		}
	}
	f.Sort()
	return &f, nil
}

// Save writes a document, indented, with its events in tick order.
//
// Indented rather than compact because the file is meant to be readable: the
// point of the format is one person handing a session to another, and a
// human-readable file can be inspected, diffed and hand-corrected. The size
// costs nothing at the scale a session reaches.
func Save(w io.Writer, f *File) error {
	f.Format = Format
	f.Version = Version
	if f.Created.IsZero() {
		f.Created = time.Now()
	}
	f.Sort()
	if f.Events == nil {
		f.Events = []Event{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(f); err != nil {
		return fmt.Errorf("write replay file: %w", err)
	}
	return nil
}

// Sort orders the events by tick. Player relies on it; Load and Save both apply
// it so no other caller has to remember.
func (f *File) Sort() {
	sort.SliceStable(f.Events, func(i, j int) bool { return f.Events[i].Tick < f.Events[j].Tick })
}

// Duration returns the span of the recording in T-states.
func (f *File) Duration() int64 {
	if len(f.Events) == 0 {
		return 0
	}
	var last int64
	for _, ev := range f.Events {
		if ev.Tick > last {
			last = ev.Tick
		}
	}
	return last
}

// Count returns how many key presses, tape actions and joystick changes the file
// holds.
func (f *File) Count() (keys, tape, joy int) {
	for _, ev := range f.Events {
		switch {
		case ev.Key != nil:
			keys++
		case ev.Joy != nil:
			joy++
		default:
			tape++
		}
	}
	return keys, tape, joy
}

// validate checks one event is well-formed. A damaged event is refused rather
// than skipped: a replay that silently drops a key press is a replay that lies.
func (e *Event) validate() error {
	if e.Tick < 0 {
		return fmt.Errorf("negative tick %d", e.Tick)
	}
	switch {
	case e.Key != nil && e.Joy != nil:
		return fmt.Errorf("carries both a key and a joystick state")
	case e.Key != nil && e.Tape != "":
		return fmt.Errorf("carries both a key and a tape action")
	case e.Joy != nil && e.Tape != "":
		return fmt.Errorf("carries both a joystick state and a tape action")
	case e.Key != nil:
		if e.Key.Row < 0 || e.Key.Row > 7 || e.Key.Col < 0 || e.Key.Col > 4 {
			return fmt.Errorf("key (%d,%d) is outside the 8x5 matrix", e.Key.Row, e.Key.Col)
		}
		return nil
	case e.Joy != nil:
		// Nothing to check: five flags are every value a JoyEvent can hold, and
		// all thirty-two of them are states the port can be written with.
		return nil
	case e.Tape != "":
		if e.Tape != TapePlay && e.Tape != TapePause {
			return fmt.Errorf("unknown tape action %q", e.Tape)
		}
		return nil
	default:
		return fmt.Errorf("carries neither a key, a joystick state nor a tape action")
	}
}

// Recorder collects events as they happen. It is not safe for concurrent use:
// input arrives on the emulator's own goroutine, which is also the one that
// drains it.
type Recorder struct {
	events  []Event
	enabled bool
}

// NewRecorder returns a recorder that starts capturing immediately.
func NewRecorder() *Recorder {
	return &Recorder{enabled: true}
}

// Key records a matrix change.
func (r *Recorder) Key(tick int64, row, col int, down bool) {
	if !r.enabled {
		return
	}
	r.events = append(r.events, Event{Tick: tick, Key: &KeyEvent{Row: row, Col: col, Down: down}})
}

// Joy records a joystick state, which is the whole of it rather than one
// direction: see JoyEvent.
func (r *Recorder) Joy(tick int64, j JoyEvent) {
	if !r.enabled {
		return
	}
	ev := j
	r.events = append(r.events, Event{Tick: tick, Joy: &ev})
}

// Tape records a playback action.
func (r *Recorder) Tape(tick int64, action string) {
	if !r.enabled {
		return
	}
	r.events = append(r.events, Event{Tick: tick, Tape: action})
}

// Enable turns capture on or off. The disk and snapshot loaders run through the
// same input API as a key press in some paths, and a caller that wants only the
// session proper can close the window around setup.
func (r *Recorder) Enable(on bool) { r.enabled = on }

// Len returns how many events have been captured.
func (r *Recorder) Len() int { return len(r.events) }

// Events returns a copy of what was captured, sorted by tick.
func (r *Recorder) Events() []Event {
	out := make([]Event, len(r.events))
	copy(out, r.events)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Tick < out[j].Tick })
	return out
}

// NewFile returns an empty document stamped with the format marker, the current
// version and the time. Callers fill in the machine, the settings, the media and
// the events; nothing about the envelope is theirs to get wrong.
func NewFile() *File {
	return &File{Format: Format, Version: Version, Created: time.Now()}
}

// Player feeds recorded events back in tick order.
//
// It is given the clock the events were recorded at and the clock of the machine
// about to play them. When the two differ -- the recording was made on a
// Pentagon and is being played on a 48K, because the model was forced on the
// command line -- timestamps are rescaled by their ratio. The alternative, using
// the raw ticks, would slip by 2.4% of the session's length, which over a few
// minutes is a visible desync rather than an approximation: the machine would
// reach the same point in its own time, but the input would arrive at the wrong
// place in the program.
type Player struct {
	events   []Event
	next     int
	recorded int
	target   int
}

// NewPlayer prepares events recorded at recordedHz for playback on a machine
// running at targetHz. Either clock may be zero, or the two equal, in which case
// timestamps are used exactly as recorded.
func NewPlayer(events []Event, recordedHz, targetHz int) *Player {
	p := &Player{events: events, recorded: recordedHz, target: targetHz}
	if recordedHz > 0 && targetHz > 0 && recordedHz != targetHz {
		p.events = make([]Event, len(events))
		copy(p.events, events)
		for i := range p.events {
			// Widen before multiplying: a long session at 3.5 MHz exceeds the
			// range of a 32-bit intermediate.
			p.events[i].Tick = p.events[i].Tick * int64(targetHz) / int64(recordedHz)
		}
	}
	return p
}

// Apply runs every event due at or before tick, in order, and returns how many
// it ran. Events are applied at instruction boundaries, which is where a host
// key press would have landed too, so a replay reproduces the session rather
// than approximating it.
func (p *Player) Apply(tick int64, fn func(Event)) int {
	n := 0
	for p.next < len(p.events) && p.events[p.next].Tick <= tick {
		fn(p.events[p.next])
		p.next++
		n++
	}
	return n
}

// Remaining returns how many events are still to play.
func (p *Player) Remaining() int { return len(p.events) - p.next }

// Len returns the total number of events.
func (p *Player) Len() int { return len(p.events) }

// Done reports whether every event has been played.
func (p *Player) Done() bool { return p.next >= len(p.events) }
