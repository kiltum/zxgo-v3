package emulator

import (
	"fmt"

	"github.com/kiltum/zxgo-v3/pkg/media"
	"github.com/kiltum/zxgo-v3/pkg/state"
)

// Storage and media: the disk controllers and the disks they have mounted, and
// the tape.
//
// The controllers are ordinary components - their registers and their
// absolute-tick deadlines - but the media are not, for a reason that decides
// their whole design:
//
//   - a disk is mutable, so it is embedded. The user may have written to it and
//     the file on disk may have been replaced since, and a session that restores
//     a program's memory with a stale disk under it is worse than no session.
//   - a tape cannot have been written to (tape writing is out of scope), so its
//     pulse stream is a pure function of the file it came from and a reference
//     is enough. The reference is a path plus a hash of the pulse stream, which
//     is what decides whether the tape in hand is the tape that was playing.

// diskSlot is one mounted disk and the place it came from, so a loader can put
// it back where it was.
type diskSlot struct {
	name string
	disk *media.Disk
}

// diskSlots lists every disk the machine has mounted.
//
// The names are the slots the controllers expose rather than a numbering of the
// list: a state file written by a machine with a second +3 drive has to be
// readable by one with only the first, and a slot that is not there is simply
// not in the list either way.
func (e *Emulator) diskSlots() []diskSlot {
	var slots []diskSlot
	if e.betaDisk != nil {
		for i, d := range e.betaDisk.Drives() {
			if d != nil {
				slots = append(slots, diskSlot{name: fmt.Sprintf("beta:%d", i), disk: d})
			}
		}
	}
	if e.upd765 != nil {
		for i, d := range e.upd765.Drives() {
			if d != nil {
				slots = append(slots, diskSlot{name: fmt.Sprintf("upd765:%d", i), disk: d})
			}
		}
	}
	return slots
}

// attachDisks puts the restored disks back into the drives they came from. A
// slot this machine does not have is reported and skipped: a session saved with
// a second drive restored onto a machine with one is a loss, not a failure.
func (e *Emulator) attachDisks(slots []diskSlot) []string {
	var skipped []string
	for _, s := range slots {
		name, index := splitSlot(s.name)
		switch name {
		case "beta":
			if e.betaDisk != nil && index < len(e.betaDisk.Drives()) {
				e.betaDisk.MountDiskToDrive(s.disk, index)
				continue
			}
		case "upd765":
			if e.upd765 != nil && index < len(e.upd765.Drives()) {
				e.upd765.MountDiskToDrive(s.disk, index)
				continue
			}
		}
		skipped = append(skipped, s.name)
	}
	return skipped
}

// splitSlot reads "controller:index", which diskSlots built.
func splitSlot(name string) (string, int) {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == ':' {
			index := 0
			if _, err := fmt.Sscanf(name[i+1:], "%d", &index); err != nil {
				return name, -1
			}
			return name[:i], index
		}
	}
	return name, -1
}

// storageSections is the storage and media part of the table.
func (e *Emulator) storageSections() []state.Spec {
	var sections []state.Spec
	if e.betaDisk != nil {
		sections = append(sections, state.Section(state.IDBetaDisk, 1, true, e.betaDisk))
	}
	if e.upd765 != nil {
		sections = append(sections, state.Section(state.IDUPD765, 1, true, e.upd765))
	}
	sections = append(sections,
		state.Spec{
			ID:       state.IDDiskImages,
			Version:  1,
			Required: false, // a machine may have no disk mounted
			Save: func(enc *state.Encoder) error {
				slots := e.diskSlots()
				enc.Count(len(slots))
				for _, s := range slots {
					enc.String(s.name)
					media.SaveDisk(enc, s.disk)
				}
				return nil
			},
			Load: func(dec *state.Decoder) error {
				// Parse every disk first: a truncated image must not leave the
				// machine with drive A remounted and drive B half-read.
				n := dec.Count(4)
				if err := dec.Err(); err != nil {
					return err
				}
				slots := make([]diskSlot, 0, n)
				for i := 0; i < n; i++ {
					name := dec.String()
					disk, err := media.LoadDisk(dec)
					if err != nil {
						return fmt.Errorf("disk %q: %w", name, err)
					}
					slots = append(slots, diskSlot{name: name, disk: disk})
				}
				e.attachDisks(slots)
				return nil
			},
		},
		state.Spec{
			ID:       state.IDTape,
			Version:  1,
			Required: false, // a machine may have no tape loaded
			Save: func(enc *state.Encoder) error {
				if e.tape == nil || e.tape.Tape == nil {
					enc.Bool(false)
					return nil
				}
				enc.Bool(true)
				media.SavePlayback(enc, e.tape)
				return nil
			},
			Load: func(dec *state.Decoder) error {
				present := dec.Bool()
				if err := dec.Err(); err != nil {
					return err
				}
				if !present {
					e.tape = nil
					return nil
				}
				name, hash, pulses, apply, err := media.LoadPlayback(dec)
				if err != nil {
					return err
				}
				return e.restoreTape(name, hash, pulses, apply)
			},
		})
	return sections
}

// restoreTape re-attaches the tape a file refers to.
//
// Three outcomes, all reported rather than refused, because a tape is a
// reference and a reference can go stale without the machine being unusable:
//
//   - the file is there and the pulse stream matches - resume where it stopped;
//   - the file is there and the stream does not match - load it from the start
//     rather than resuming into the middle of a different tape;
//   - the file is gone - leave the machine with no tape.
//
// The report comes back through the load Result, so a caller can say what
// happened instead of guessing.
func (e *Emulator) restoreTape(name string, hash uint32, pulses int, apply func(*media.Playback) error) error {
	tape, err := media.LoadTapeFile(name)
	if err != nil {
		// Gone, unreadable, or no longer a tape: the machine runs without one.
		e.tape = nil
		e.noteTapeOutcome(tapeOutcomeMissing, name)
		return nil
	}
	gotHash, gotPulses := media.TapeIdentity(tape)
	if gotHash != hash || gotPulses != pulses {
		e.tape = media.NewPlayback(tape)
		e.noteTapeOutcome(tapeOutcomeChanged, name)
		return nil
	}

	pb := media.NewPlayback(tape)
	if err := apply(pb); err != nil {
		return err
	}
	e.tape = pb
	e.noteTapeOutcome(tapeOutcomeResumed, name)
	return nil
}

// tapeOutcome records what a load did with the tape, for the session report.
type tapeOutcome int

const (
	tapeOutcomeNone tapeOutcome = iota
	tapeOutcomeResumed
	tapeOutcomeChanged
	tapeOutcomeMissing
)

// noteTapeOutcome stashes the tape's fate on the emulator so the next load
// result can carry it. It is emulator state, not machine state: it describes
// what the last load did, and a save never looks at it.
func (e *Emulator) noteTapeOutcome(o tapeOutcome, name string) {
	e.tapeResult = &TapeInfo{Outcome: o, Name: name}
}

// TapeInfo says what happened to the tape a state file referred to.
type TapeInfo struct {
	Outcome tapeOutcome
	Name    string
}

// String reports the outcome for a session message.
func (t TapeInfo) String() string {
	switch t.Outcome {
	case tapeOutcomeResumed:
		return "tape resumed: " + t.Name
	case tapeOutcomeChanged:
		return "tape changed on disk, loaded from the start: " + t.Name
	case tapeOutcomeMissing:
		return "tape missing, none mounted: " + t.Name
	}
	return ""
}
