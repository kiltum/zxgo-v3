package frontend

import "github.com/kiltum/zxgo-v3/pkg/replay"

// MediaPaths are the files the machine was given, so that a recording can say which tape
// and disk someone else needs to mount beside it.
//
// A replay is only useful to another person if it names its media: the keys that typed
// `LOAD ""` land on a machine with nothing to load otherwise. The snapshot is named for
// the same reason - a recording made on top of a snapshot is meaningless without it.
type MediaPaths struct {
	Snapshot string
	Tape     string
	Disk     string
}

// MediaPaths returns the files the machine was given.
func (a *App) MediaPaths() MediaPaths { return a.media }

// SetMediaPaths records them, which the command line does when it loads media itself
// rather than through the front end.
func (a *App) SetMediaPaths(m MediaPaths) { a.media = m }

// StartRecording begins recording the machine's input from now, to be written to path on
// a graceful exit.
//
// It attaches a recorder to the machine, which is where input recording has to hang: the
// recorder sees PressKey, ReleaseKey and the transport because those are the methods every
// route to the machine goes through (D9), whether the input came from the host keyboard,
// the on-screen keyboard, or the MCP worker.
//
// There is no stop: a recording ends when the emulator does, which is the same
// life-of-the-process shape the command line's -save-replay has.
func (a *App) StartRecording(path string) {
	a.recorder = replay.NewRecorder()
	a.recordingPath = path
	a.Machine.SetRecorder(a.recorder)
}

// Recording reports whether a recording is in progress, which is what D11's refusal turns
// on: a session load while recording would make the recorded event times meaningless.
func (a *App) Recording() bool { return a.recorder != nil }

// RecordingPath is the file the recording will be written to, empty when there is none.
func (a *App) RecordingPath() string { return a.recordingPath }

// SaveRecording writes the recording and reports what it wrote, as the line a caller
// shows. It is what a graceful exit calls, and it does nothing when nothing was recorded.
func (a *App) SaveRecording() (string, error) {
	if !a.Recording() {
		return "", nil
	}
	summary, err := SaveReplay(a.Machine, a.recorder, a.recordingPath, a.media, a.Machine.FastTape())
	if err != nil {
		return "", err
	}
	// The message is returned unindented: the CLI indents its progress lines and a window
	// would not, so the indentation is the caller's.
	return summary.String(), nil
}
