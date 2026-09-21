package frontend

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kiltum/zxgo-v3/pkg/state"
)

// stateOptions is how a session is written: deflated, because RAM and a mostly empty disk
// compress well and nobody reads this file in an editor.
func stateOptions() state.Options { return state.Options{Deflate: true} }

// SessionFileExt is the extension of a session file in the config directory. It is the
// native format of STATE_DESIGN.md - a binary container, not one of the JSON preference
// files - and it sits beside them.
const SessionFileExt = ".zxstate"

// SessionPath is the session file for one model, which is what this store reads and writes.
//
// **One file per model, because a session is a machine.** They share a directory but not a
// file: a 48K run that found a Pentagon's session refused it (correctly - D11 - the stamps
// belong to another machine) and then overwrote it on the way out, so the Pentagon session
// was gone the next time the user went back to it. Per model, every machine keeps its own
// place to return to, and nothing is refused that the user did not explicitly ask for.
//
// modelKey is a registry key (`model.Config.Key`): "48k", "128k", "2a3", "pentagon".
func (s Store) SessionPath(modelKey string) string {
	return filepath.Join(s.Dir, "session-"+modelKey+SessionFileExt)
}

// ErrReplayHasNoSession is why a session is not used while a replay is playing or being
// recorded. It is an error rather than a silent skip because a user who named a session
// explicitly is owed the answer; a run that only *would* have used the config directory's
// session says nothing, because the user never asked for one.
var ErrReplayHasNoSession = errors.New("this run is a replay, which has no session")

// LatestSession is the model whose session was saved most recently, and whether there is
// one at all: the machine the user was last on.
//
// It is what a launch with no -model resumes. Without it, a user who was on a Pentagon and
// runs the emulator again with no arguments gets a 48K machine and a report that their
// session belongs to another model - which is technically right and useless: what they asked
// for, by saying nothing, is to be where they were.
//
// The directory is scanned rather than an index kept, because the files are few and each
// one's timestamp is already the answer: a session file is written when the emulator exits,
// so the newest is the last machine used.
func (s Store) LatestSession() (modelKey string, ok bool) {
	entries, err := filepath.Glob(filepath.Join(s.Dir, "session-*"+SessionFileExt))
	if err != nil {
		return "", false
	}

	var newest time.Time
	for _, path := range entries {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		if !ok || info.ModTime().After(newest) {
			key := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "session-"), SessionFileExt)
			if key == "" {
				continue
			}
			newest, modelKey, ok = info.ModTime(), key, true
		}
	}
	return modelKey, ok
}

// SessionRequest is what a session resolution depends on, as one value: the paths the
// command line named, the settings, the store, which model is running, and whether this run
// is a replay. It is a struct rather than six parameters because the call site reads as what
// it is - and because a bool in the middle of an argument list is a comment waiting to
// happen.
type SessionRequest struct {
	// LoadExplicit and SaveExplicit are -load-state and -save-state (or -session for
	// both), already extension-resolved. Empty means the half was not named.
	LoadExplicit string
	SaveExplicit string
	Settings     Settings
	Store        Store
	// Model is the registry key of the machine that is running.
	Model string
	// Replay is true when a replay is playing or being recorded.
	Replay bool
}

// SessionFiles resolves the two paths a session run uses: where it is read from and where it
// is written. Both are empty when the run has no session, and a user who named one is told
// why.
//
// A replay run has no session, and that is the whole rule. Playing a replay back is a
// performance of a recording whose events are stamped from the tick the machine started:
// resuming a state first would put every event at the wrong place, and saving a state
// afterwards would make the *next* run continue the replay from where it stopped instead of
// starting it - which is exactly what happened, twice, before this was one rule instead of
// two (D11 had the load refused while recording and said nothing about playback or about
// saving).
//
// Recording is the same case seen from the other end: the file being written is the record
// of the run, and a session beside it says the same thing twice.
//
// An explicit -session, -load-state or -save-state is refused with a reason rather than
// silently ignored: the user asked for something.
func SessionFiles(r SessionRequest) (resume, saveTo string, err error) {
	if r.Replay {
		if r.LoadExplicit != "" || r.SaveExplicit != "" {
			return "", "", ErrReplayHasNoSession
		}
		return "", "", nil
	}

	resume = r.LoadExplicit
	saveTo = r.SaveExplicit
	if resume == "" && saveTo == "" {
		// Neither half was named: this model's session in the config directory is the
		// default, if the setting is on, and it is the same file for both halves.
		if r.Settings.SessionEnabled() && r.Store.Dir != "" {
			resume = r.Store.SessionPath(r.Model)
			saveTo = resume
		}
	}
	return resume, saveTo, nil
}

// RestoreSession applies a session to the machine and reports what it restored, as the
// line a caller shows.
//
// A session that cannot be read - corrupt, from another model, or simply not there - is an
// error and the machine is left alone: STATE_DESIGN.md's container refuses rather than
// half-applying, and a user whose emulator will not start because of a stale file would be
// worse off than one who starts clean. The caller decides whether that is fatal; it is not.
func RestoreSession(m Machine, path string) (string, error) {
	info, err := m.LoadSession(path)
	if err != nil {
		// A session that is not there is not a failure and not worth a line: it is the
		// first run, or a run after the file was deleted. Anything else - a corrupt file,
		// another model's, a snapshot it names that has gone - is reported, because the
		// user asked for a session and did not get one.
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	msg := fmt.Sprintf("resumed %s: %s saved by %s, %d ticks (%d frames) at %s",
		path, info.Model, info.Build, info.Ticks, info.Frames,
		info.Saved.Format("2006-01-02 15:04:05"))
	if info.Result != nil && len(info.Result.Skipped) > 0 {
		msg += fmt.Sprintf("; %d unknown chunk(s) were skipped", len(info.Result.Skipped))
	}
	return msg, nil
}

// SaveSessionOnExit writes the machine's state and the media it has mounted, and reports
// where, as the line a caller shows.
//
// The write is atomic - a temporary file and a rename, in STATE_DESIGN.md's container -
// so a crash during it leaves the previous session rather than half of a new one. Session
// state is deflated: RAM and a mostly empty disk compress well, and nobody reads this file
// in an editor.
func SaveSessionOnExit(m Machine, path string) (string, error) {
	if err := m.SaveSession(path, stateOptions()); err != nil {
		return "", err
	}
	return fmt.Sprintf("Session saved: %s", path), nil
}
