package frontend

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kiltum/zxgo-v3/pkg/model"
)

// DialogKind names what a file dialog is for, which is what decides its filters and
// where it opens (UI_DESIGN.md D2).
type DialogKind int

const (
	// noDialog is the zero value: "this is not a dialog". It is first so that an
	// Event that was built without a kind means none rather than quietly meaning the
	// first kind in the list - which is what it did when DialogOpenTape was the zero
	// value, and what took a test to notice.
	noDialog DialogKind = iota

	// DialogOpenTape chooses a tape image to load.
	DialogOpenTape
	// DialogOpenDisk chooses a disk image to mount in the selected drive.
	DialogOpenDisk
	// DialogOpenSnapshot chooses a snapshot to load, DialogSaveSnapshot names the file
	// to write one to. They are two kinds rather than one with a flag because what
	// happens to the answer is different, and because a save dialog needs a name rather
	// than an existing file.
	DialogOpenSnapshot
	DialogSaveSnapshot
	// DialogSaveReplay names the file a recording is written to. There is no "open
	// replay" kind yet: playing one back into a running machine is a separate question -
	// a replay carries its machine, its switches and its media, and only its events can
	// be applied to a machine that is already running.
	DialogSaveReplay
)

// IsSave reports whether a kind is answered by the save dialog rather than the open
// one. It is what the loop picks the backend's call with.
func (k DialogKind) IsSave() bool { return k == DialogSaveSnapshot || k == DialogSaveReplay }

// String is the key the backend and the settings file use for a kind, so both are
// readable rather than numeric.
//
// A kind with no name returns the empty string rather than a plausible-looking one. A
// fallback name is how a settings file came to contain "unknown": a kind was recorded
// under the enum's zero value before that value meant "no dialog", saved under the
// fallback, and then reported as unreadable on the next load. The store now refuses to
// write an unnameable kind, and this makes it unnameable rather than misnamed.
func (k DialogKind) String() string {
	switch k {
	case DialogOpenTape:
		return "tape"
	case DialogOpenDisk:
		return "disk"
	case DialogOpenSnapshot:
		return "snapshot"
	case DialogSaveSnapshot:
		return "snapshot-save"
	case DialogSaveReplay:
		return "replay-save"
	}
	return ""
}

// ParseDialogKind reads a kind name as the settings file spells it.
func ParseDialogKind(name string) (DialogKind, bool) {
	switch name {
	case "tape":
		return DialogOpenTape, true
	case "disk":
		return DialogOpenDisk, true
	case "snapshot":
		return DialogOpenSnapshot, true
	case "snapshot-save":
		return DialogSaveSnapshot, true
	case "replay-save":
		return DialogSaveReplay, true
	}
	return noDialog, false
}

// DialogRequest is a file the front end wants the user to choose.
//
// The front end does not open the dialog: it says what it wants and the loop hands
// that to the backend, which owns the OS's windows (section 5.4). The answer comes
// back later as an EvDialogResult event, because SDL's dialog is asynchronous and
// its callback may run on another thread (D2).
type DialogRequest struct {
	Kind DialogKind
	// Default is where the dialog should open: the last file of this kind the user
	// chose. Empty leaves it to the platform.
	Default string
}

// Settings is the front end's preferences that are neither the keymap nor the layout:
// where the file dialogs open, and whether the machine's state is carried between runs.
// The settings window (M6) fills in the rest.
type Settings struct {
	// LastFile is the last file the user chose for each kind of dialog.
	//
	// A file rather than a directory, because a directory can be derived from a file
	// and not the other way round, and a platform that can select a file is given
	// something to select. The dialog is handed the file when it still exists and its
	// directory when it does not - see dialogDefault.
	LastFile map[DialogKind]string

	// Session turns both halves of D11 on and off: the state written on a graceful exit
	// and read at the next start. It is a pointer so that a settings file which does not
	// mention it keeps the default - on - rather than turning it off, which is the same
	// shape and the same reason as the layout's menu_visible: absent and false are not
	// the same thing, and a plain bool cannot tell them apart.
	Session *bool

	// Model is the machine to build. Empty means the user has never chosen one, in which
	// case a launch with no -model resumes the session used last (D11) and otherwise takes
	// the registry's default. A model that *has* been chosen wins over that history: a user
	// who said which machine they want is not asking to be where they were.
	Model string

	// Z80 is the CPU variant: "nmos" (the hardware, and the default), "cmos" (what the
	// later machines carried), or empty for the default.
	Z80 string

	// MEMPTR is the behaviour of the repeating block I/O instructions' MEMPTR register:
	// "real" (the measured behaviour, and the default) or "documented" (the BC-derived value
	// the Zilog documentation implies), or empty for the default. See KNOWN_BUGS.md: both are
	// implemented and the switch is what keeps the two test suites honest, so this row is a
	// choice between two behaviours rather than a right and a wrong one.
	MEMPTR string

	// The machine's switches, as three-state values: nil leaves the model's own default - the
	// Pentagon's TurboSound is a property of the model, not a preference - while false turns
	// one off that a model has on and true turns one on that it does not. The command line's
	// flags only ever turn things on, so a setting is the more expressive of the two, and it
	// is what the settings window writes.
	machineSwitches
}

// Z80 variants, as settings.json spells them.
const (
	Z80NMOS = "nmos"
	Z80CMOS = "cmos"
)

// MEMPTR behaviours, as settings.json spells them.
const (
	MEMPTRReal       = "real"
	MEMPTRDocumented = "documented"
)

// Z80Choice reports the CPU variant the settings ask for, and whether they ask for one at
// all. Empty means "no opinion", which is not the same as NMOS: a session resumed from disk
// carries the CPU it was saved with (pkg/cpu/state.go), and a file that says nothing must
// leave that alone rather than quietly forcing the machine back to NMOS.
func (s Settings) Z80Choice() (isNMOS bool, ok bool) {
	switch s.Z80 {
	case Z80NMOS:
		return true, true
	case Z80CMOS:
		return false, true
	}
	return true, false
}

// MEMPTRRealChoice reports the MEMPTR behaviour the settings ask for, and whether they ask
// for one at all. It follows Z80Choice's rule and for the same reason: the CPU state carries
// the behaviour it was saved with, so "no opinion" has to be expressible.
func (s Settings) MEMPTRRealChoice() (real, ok bool) {
	switch s.MEMPTR {
	case MEMPTRReal:
		return true, true
	case MEMPTRDocumented:
		return false, true
	}
	return true, false
}

// ApplyTo turns the settings' switches on in a machine configuration.
//
// It is called before the machine is built, because these switches decide what is built: a
// sound card is added or replaced at construction, which is why D10 lists them as
// restart-required. Each switch is applied only when the settings actually say something, so
// a model's own default survives a file that does not mention it.
//
// It is applied *between* the replay's recorded switches and the command line's flags, which
// is the same order -model follows: the standing configuration first, the per-run override
// last. A user who types -turbosound gets it for that run, and the file turns it off again
// for the next one.
//
// Fast tape is not here: it is emulator state rather than machine configuration, the same
// distinction ApplyRecordedSettings makes when it returns it instead of applying it.
func (s Settings) ApplyTo(cfg *model.Config) {
	for _, c := range switchChoices {
		if c.apply == nil {
			continue // emulator state, applied to the emulator rather than to the config
		}
		if v := c.get(s.machineSwitches); v != nil {
			c.apply(cfg, *v)
		}
	}
}

// modelOrDefault is the model the settings ask for, with the machine that is running as the
// answer when the user has never chosen one. It is what the settings window flags.
func (s Settings) modelOrDefault(running string) string {
	if s.Model != "" {
		return s.Model
	}
	return running
}

// SessionEnabled reports whether the session is saved and restored. It is on by default,
// which is the one place that default lives.
func (s Settings) SessionEnabled() bool { return s.Session == nil || *s.Session }

// SetSession turns the session on or off, which is what the settings window does when its
// session row is clicked.
func (s *Settings) SetSession(on bool) { s.Session = &on }

// Equal reports whether two settings are the same, so a caller can persist on change
// rather than on every frame.
func (s Settings) Equal(other Settings) bool {
	// Compared by meaning rather than by pointer: nil and a pointer to true both mean
	// "on", and a save that fired because of that difference would rewrite the file for
	// nothing.
	if s.SessionEnabled() != other.SessionEnabled() {
		return false
	}
	if s.Model != other.Model || s.Z80 != other.Z80 || s.MEMPTR != other.MEMPTR {
		return false
	}
	// The switches are compared by meaning too: nil and an explicit value are the same
	// answer when the value is what the model already has, and the model's default is what
	// nil means.
	for _, c := range switchChoices {
		on, set := c.value(s.machineSwitches)
		otherOn, otherSet := c.value(other.machineSwitches)
		if set != otherSet || (set && on != otherOn) {
			return false
		}
	}
	if len(s.LastFile) != len(other.LastFile) {
		return false
	}
	for kind, path := range s.LastFile {
		if other.LastFile[kind] != path {
			return false
		}
	}
	return true
}

// Settings returns the front end's settings as they stand.
func (a *App) Settings() Settings {
	out := a.settings
	out.LastFile = make(map[DialogKind]string, len(a.settings.LastFile))
	for kind, path := range a.settings.LastFile {
		out.LastFile[kind] = path
	}
	for _, c := range switchChoices {
		c.set(&out.machineSwitches, copyBool(c.get(a.settings.machineSwitches)))
	}
	return out
}

// SetSettings applies settings read from disk.
func (a *App) SetSettings(s Settings) {
	a.settings = Settings{
		Model:  s.Model,
		Z80:    s.Z80,
		MEMPTR: s.MEMPTR,
	}
	if len(s.LastFile) > 0 {
		a.settings.LastFile = make(map[DialogKind]string, len(s.LastFile))
		for kind, path := range s.LastFile {
			a.settings.LastFile[kind] = path
		}
	}
	a.settings.Session = copyBool(s.Session)
	for _, c := range switchChoices {
		c.set(&a.settings.machineSwitches, copyBool(c.get(s.machineSwitches)))
	}
}

// requestDialog asks for a file dialog of one kind.
//
// The kind is recorded here rather than when the loop hands the request over: the app
// is waiting for an answer from the moment it asks, and the answer says which dialog
// it is for. Recording it later left a window in which an answer named no kind, and
// nothing happened - which is exactly what the first version did.
func (a *App) requestDialog(kind DialogKind) {
	a.dialogAsked = DialogRequest{Kind: kind, Default: a.dialogDefault(kind)}
	a.dialogWanted = true
	a.dialogWaiting = true
}

// dialogDefault is where a dialog of this kind should open: the file the user chose
// last time if it is still there, otherwise the directory it was in, otherwise
// nothing and the platform decides.
//
// The fallback is what keeps a deleted tape from sending the next dialog to an
// arbitrary place: a directory that still exists is a better answer than a file that
// does not.
func (a *App) dialogDefault(kind DialogKind) string {
	last := a.settings.LastFile[kind]
	if last == "" {
		return ""
	}
	if _, err := os.Stat(last); err == nil {
		return last
	}
	dir := filepath.Dir(last)
	if _, err := os.Stat(dir); err == nil {
		return dir
	}
	return ""
}

// TakeDialogRequest returns the dialog the app wants opened, once. The loop hands it
// to the backend, which owns the OS dialog; the answer arrives as an event.
func (a *App) TakeDialogRequest() (DialogRequest, bool) {
	if !a.dialogWanted {
		return DialogRequest{}, false
	}
	a.dialogWanted = false
	return a.dialogAsked, true
}

// DialogOpen reports whether a dialog has been asked for and not yet answered, which
// is what D5 rule 10 needs: while a native dialog is up, the machine must not hear
// the keyboard. It is true from the moment the user asks, because the next thing they
// type is a filename.
func (a *App) DialogOpen() bool { return a.dialogWanted || a.dialogWaiting }

// applyDialogResult does what the user asked for when a dialog returns.
//
// The kind comes with the answer rather than from the app's own record of what it
// asked: the backend knows which dialog it opened, the event says so, and one source
// cannot disagree with itself. A cancelled or failed dialog has an empty path and
// does nothing - a user who closed the dialog did not ask for anything.
func (a *App) applyDialogResult(kind DialogKind, path string) error {
	a.dialogWaiting = false

	if kind == noDialog || path == "" {
		return nil
	}

	// A save dialog may return a name without the extension, and the extension is added
	// before anything else looks at the path: remembering the name as it came back would
	// remember a file that does not exist, and the next dialog would open at it.
	path = saveNameFor(kind, path)

	// Where the user was is remembered whatever the answer is used for, so the next
	// dialog of this kind opens there.
	if a.settings.LastFile == nil {
		a.settings.LastFile = make(map[DialogKind]string)
	}
	a.settings.LastFile[kind] = path

	switch kind {
	case DialogOpenTape:
		// The path is remembered for the media section of a recording: a replay is only
		// useful to someone else if it says which tape to mount beside it.
		err := a.Machine.LoadTapeFromPath(path)
		if err == nil {
			a.media.Tape = path
		}
		return err
	case DialogOpenDisk:
		if _, err := a.Machine.MountDiskFromPath(path); err != nil {
			return err
		}
		a.media.Disk = path
		return nil
	case DialogOpenSnapshot:
		info, err := a.Machine.LoadSnapshotFile(path)
		if err != nil {
			return err
		}
		a.Notify(fmt.Sprintf("Loaded %s snapshot: %s (%s)", info.Format, path,
			SnapshotClass(SnapshotInfo{Is128K: info.Is128K, HardwareMode: info.HardwareMode})))
		return nil
	case DialogSaveSnapshot:
		msg, err := SaveSnapshot(a.Machine, path)
		if err != nil {
			return err
		}
		a.media.Snapshot = path
		a.Notify(msg)
		return nil
	case DialogSaveReplay:
		// Recording starts here, when the user has named the file, and runs until the
		// emulator exits - the same life-of-the-process shape the command line's
		// -save-replay has.
		a.StartRecording(path)
		a.Notify("Recording to " + path)
		return nil
	}
	return nil
}

// saveNameFor gives a name from a save dialog the extension the format it will be
// written in wants. Which format that is depends on the kind, so the kinds say.
func saveNameFor(kind DialogKind, path string) string {
	switch kind {
	case DialogSaveSnapshot:
		return SavePath(path, SnapshotExt)
	case DialogSaveReplay:
		return SavePath(path, ReplayExt)
	}
	return path
}
