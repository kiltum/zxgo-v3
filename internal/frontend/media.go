package frontend

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kiltum/zxgo-v3/pkg/replay"
)

// LoadReplay reads a .replay file.
func LoadReplay(path string) (*replay.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return replay.Load(f)
}

// ResolveMedia finds a path a replay recorded: as it was recorded, then beside
// the .replay file itself, since a session handed to someone else usually
// arrives with its tape in the same folder. A path that resolves in neither
// place is appended to missing and reported by the caller.
func ResolveMedia(recorded, replayPath string, missing *[]string) string {
	if recorded == "" {
		return ""
	}
	if _, err := os.Stat(recorded); err == nil {
		return recorded
	}
	beside := filepath.Join(filepath.Dir(replayPath), filepath.Base(recorded))
	if _, err := os.Stat(beside); err == nil {
		return beside
	}
	*missing = append(*missing, recorded)
	return ""
}

// ReplaySummary describes what a save wrote, for the caller to report: the CLI
// prints it, and the GUI shows the same numbers in its status read-out.
type ReplaySummary struct {
	Path       string
	KeyEvents  int
	TapeEvents int
	JoyEvents  int
	Duration   time.Duration
}

// String renders the summary the way the CLI prints it. The joystick count is
// named only when there is one, so the sentence a keyboard-only session has
// always produced is unchanged.
func (s ReplaySummary) String() string {
	joy := ""
	if s.JoyEvents > 0 {
		joy = fmt.Sprintf(", %d joystick moves", s.JoyEvents)
	}
	return fmt.Sprintf("Saved replay: %s (%d keys, %d tape actions%s, %s)",
		s.Path, s.KeyEvents, s.TapeEvents, joy, s.Duration)
}

// SaveReplay writes the recorded session and reports what it wrote. The machine
// description comes from the emulator itself, so the file cannot disagree with
// what actually ran: the model key is the one that was looked up, and the
// switches are read out of the configured machine rather than out of the flags,
// which is what makes a Pentagon's always-on TurboSound reproduce on a model
// where it is not the default.
func SaveReplay(m Machine, rec *replay.Recorder, path string,
	recorded MediaPaths, fastTape bool) (ReplaySummary, error) {

	summary := ReplaySummary{Path: path}

	cfg := m.ModelConfig()

	f := replay.NewFile()
	// The model key comes from the machine's own config rather than from the caller: the
	// two cannot disagree, because the config *is* the key's entry in the registry
	// (STATE_DESIGN.md S1 pins that with a test).
	f.Model = cfg.Key
	f.ModelName = cfg.Name
	f.CPUHz = m.CPUHz()
	f.Settings = replay.Settings{
		TurboSound:   cfg.HasTurboSound,
		TurboSoundFM: cfg.HasTurboSoundFM,
		GeneralSound: cfg.HasGS,
		Snow:         cfg.HasSnowEffect,
		NoFDCTiming:  cfg.NoFDCTiming,
		FastTape:     fastTape,
	}
	f.Media = replay.Media{
		Snapshot: recorded.Snapshot,
		Tape:     recorded.Tape,
		Disk:     recorded.Disk,
	}
	f.Events = rec.Events()

	out, err := os.Create(path)
	if err != nil {
		return summary, err
	}
	defer out.Close()
	if err := replay.Save(out, f); err != nil {
		return summary, err
	}

	summary.KeyEvents, summary.TapeEvents, summary.JoyEvents = f.Count()
	summary.Duration = time.Duration(f.Duration() * int64(time.Second) / int64(m.CPUHz()))
	return summary, nil
}
