package main

import (
	"encoding/json"
	"os"

	"github.com/kiltum/zxgo-v3/internal/emulator"
	"github.com/kiltum/zxgo-v3/pkg/snap"
)

// loadSnapshotFile loads a .sna or .z80 snapshot from path into the emulator.
// The path may be a .zip holding the snapshot; the emulator unpacks it and picks
// the format from the name inside.
func loadSnapshotFile(e *emulator.Emulator, path string) error {
	_, err := e.LoadSnapshotFile(path)
	return err
}

// loadSnapshot loads a snapshot file into an existing machine.
func (w *worker) loadSnapshot(raw json.RawMessage) (any, error) {
	var p struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e, err := w.getMachine(p.Name)
	if err != nil {
		return nil, err
	}
	if err := loadSnapshotFile(e, p.Path); err != nil {
		return nil, err
	}
	return "ok", nil
}

// saveSnapshot saves the machine state as an SNA file.
func (w *worker) saveSnapshot(raw json.RawMessage) (any, error) {
	var p struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e, err := w.getMachine(p.Name)
	if err != nil {
		return nil, err
	}
	s, err := e.CreateSNA()
	if err != nil {
		return nil, err
	}
	f, err := os.Create(p.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := snap.SaveSNA(f, s); err != nil {
		return nil, err
	}
	return "ok", nil
}
