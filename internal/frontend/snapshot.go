package frontend

import (
	"fmt"
	"os"

	"github.com/kiltum/zxgo-v3/pkg/snap"
)

// SnapshotExt is the extension a saved snapshot gets when it is given without one.
const SnapshotExt = ".sna"

// SaveSnapshot writes the machine's state as an SNA file and reports what it wrote, as
// the one-line message a caller shows.
//
// SNA is the interchange format, and the emulator builds it: `Emulator.CreateSNA`
// knows the 128K form, which a caller assembling the file itself used to write as 48K
// whatever the machine was. The session format (D11) is not this: it is the native one
// that keeps every bank and every device.
//
// The file is written whole and then closed, like every other file here: a snapshot is
// small, and an interrupted one is worth less than none.
func SaveSnapshot(m Machine, path string) (string, error) {
	s, err := m.CreateSNA()
	if err != nil {
		return "", fmt.Errorf("creating the snapshot: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	if err := snap.SaveSNA(f, s); err != nil {
		f.Close()
		return "", fmt.Errorf("writing %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("closing %s: %w", path, err)
	}

	// The message comes back rather than being printed here: the CLI prints it and the
	// window shows it, and neither is this function's business.
	return fmt.Sprintf("Saved snapshot: %s (%s)", path,
		SnapshotClass(SnapshotInfo{Is128K: s.Is128K})), nil
}

// SnapshotInfo is the bit of a snapshot's description the class is read from. It is
// here rather than taking the emulator's own struct so that the classification can be
// tested without building a machine.
type SnapshotInfo struct {
	Is128K       bool
	HardwareMode uint8
}

// SnapshotClass names the machine a snapshot is for, for the message after a save or a
// load.
//
// The hardware mode is the finer answer when the file carries one: Is128K alone would
// report every banked machine as a plain 128K, and a Pentagon's extra banks would go
// unnamed. A snapshot with neither is a 48K.
func SnapshotClass(info SnapshotInfo) string {
	if !info.Is128K {
		return "48k"
	}
	switch info.HardwareMode {
	case 7:
		return "+3"
	case 9:
		return "pentagon"
	default:
		return "128k"
	}
}
