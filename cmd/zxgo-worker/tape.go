package main

import (
	"encoding/json"
	"fmt"

	"github.com/kiltum/zxgo-v3/internal/emulator"
	"github.com/kiltum/zxgo-v3/pkg/media"
)

// loadTapeFile loads a .tap or .tzx tape into the emulator (not started). The
// path may be a .zip holding the tape: media.LoadTapeFile unpacks it and detects
// the format from the name inside, so nothing here depends on the extension.
func loadTapeFile(e *emulator.Emulator, path string) error {
	tape, err := media.LoadTapeFile(path)
	if err != nil {
		return err
	}
	return e.LoadTape(tape)
}

// tapeControl plays, pauses, or rewinds the tape.
func (w *worker) tapeControl(raw json.RawMessage) (any, error) {
	var p struct {
		Name   string `json:"name"`
		Action string `json:"action"` // "play", "pause", "reset"
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e, err := w.getMachine(p.Name)
	if err != nil {
		return nil, err
	}
	if e.TapePlayback() == nil {
		return nil, fmt.Errorf("no tape loaded")
	}
	switch p.Action {
	case "play":
		e.StartTapePlayback()
	case "pause":
		e.StopTapePlayback()
	case "reset":
		e.ResetTapePlayback()
	default:
		return nil, fmt.Errorf("unknown action %q (play, pause, reset)", p.Action)
	}
	return "ok", nil
}
