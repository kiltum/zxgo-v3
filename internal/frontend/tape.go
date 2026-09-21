package frontend

// ToggleTapePlayback starts or pauses the tape and reports the new state, or ok
// false when no tape is mounted.
//
// It goes through the emulator's transport methods rather than through the
// Playback object it wraps. That is not a style preference: recording hangs off
// the emulator's methods, so reaching past them to Playback.Pause() makes Cmd+P
// invisible to a recording -- which is exactly what it did, leaving replay files
// with the media named and no tape event to start it.
func ToggleTapePlayback(m Machine) (playing bool, ok bool) {
	if !m.TapeMounted() {
		return false, false
	}
	if m.TapePlaying() {
		m.StopTapePlayback()
		return false, true
	}
	m.StartTapePlayback()
	return true, true
}
