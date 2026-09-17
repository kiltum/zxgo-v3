package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/kiltum/zxgo-v3/internal/emulator"
	"github.com/kiltum/zxgo-v3/internal/ui"
	"github.com/kiltum/zxgo-v3/internal/ui/sdl"
	"github.com/kiltum/zxgo-v3/pkg/cpu"
	"github.com/kiltum/zxgo-v3/pkg/logger"
	"github.com/kiltum/zxgo-v3/pkg/media"
	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/replay"
	"github.com/kiltum/zxgo-v3/pkg/snap"
	"github.com/kiltum/zxgo-v3/pkg/sound/sdl3"
	"github.com/kiltum/zxgo-v3/pkg/ula"
)

// SDL3's macOS Cocoa backend requires the first OS thread, so lock to it before
// SDL touches anything.
func init() { runtime.LockOSThread() }

func main() {
	modelFlag := flag.String("model", "48k", "ZX Spectrum model: 48k, 128k, 2a3, pentagon")
	tapFlag := flag.String("tap", "", "path to TAP/TZX file to load")
	snaFlag := flag.String("sna", "", "path to SNA snapshot file to load")
	z80Flag := flag.String("z80", "", "path to Z80 snapshot file to load")
	saveSnaFlag := flag.String("save-sna", "", "path to save current state as SNA snapshot")
	diskFlag := flag.String("disk", "", "path to TRD/SCL/DSK disk image to load")
	logFlag := flag.String("log", "", "log level: debug, info, warn, error (default: silent)")
	profFlag := flag.Bool("prof", false, "enable CPU, memory, and trace profiling (writes *.prof files on exit)")
	turbosoundFlag := flag.Bool("turbosound", false, "enable TurboSound (two AY chips; always on for the Pentagon)")
	turbosoundfmFlag := flag.Bool("turbosoundfm", false, "enable the YM2203 FM chip (TurboSound FM, replaces the AY)")
	nofdcFlag := flag.Bool("no-fdc-timing", false, "disable FDC seek/rotational latency (fast loads, not for copy-protected images)")
	fastTapeFlag := flag.Bool("fast-tape", false, "load tapes at full host speed: the speed throttle is off while the tape plays")
	gsFlag := flag.Bool("gs", false, "enable the General Sound card (requires roms/gs105a.rom)")
	snowFlag := flag.Bool("snow", false, "enable the 48K snow artefact (ULA/CPU data-bus conflict; noisy)")
	replayFlag := flag.String("replay", "", "path to a .replay file to play back; its machine and switches are used")
	saveReplayFlag := flag.String("save-replay", "", "path to write a .replay recording of this session on exit")
	flag.Parse()

	// A replay is read before the machine is built: it carries the model and the
	// switches the session ran with, so it has to be in hand before there is a
	// config to fill in.
	var rp *replay.File
	if *replayFlag != "" {
		loaded, err := loadReplay(*replayFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load replay: %v\n", err)
			os.Exit(1)
		}
		rp = loaded
	}

	// The model is the one the replay names, unless one was asked for on the
	// command line: this is the only place in the tree that needs to tell "flag
	// given" from "flag holds its zero value", which is why it asks flag.Visit
	// rather than testing the string. The switches below need no such test --
	// they only ever turn something on, so the union of the file and the command
	// line is what the user meant either way.
	modelKey := *modelFlag
	if rp != nil && !flagWasSet("model") {
		modelKey = rp.Model
	}
	cfg, ok := model.AllModels[modelKey]
	if !ok {
		fmt.Fprintf(os.Stderr, "Unknown model: %s\n", modelKey)
		os.Exit(1)
	}
	if rp != nil && modelKey != rp.Model {
		// The events were stamped against another model's clock; the player
		// rescales them, and the user should know the replay is approximate by
		// their own choice rather than by a defect.
		fmt.Printf("zxgo-v3: replay was recorded on %s, playing on %s (timings scaled)\n",
			rp.Model, modelKey)
	}

	// A/B test override for the /INT position (Pentagon timing debugging).
	if v := os.Getenv("ZXGO_INT_OFFSET"); v != "" {
		if off, err := strconv.Atoi(v); err == nil {
			fmt.Printf("zxgo-v3: overriding InterruptOffset %d -> %d\n", cfg.InterruptOffset, off)
			cfg.InterruptOffset = off
		}
	}

	var settings replay.Settings
	if rp != nil {
		settings = rp.Settings
	}

	// The replay's switches are applied first and the command line's after.
	// Order does not matter and nothing conflicts: every one of these only ever
	// turns something on, so the two can only add up, and a model's own defaults
	// (the Pentagon's TurboSound) survive a settings section that says nothing.
	fastTape := *fastTapeFlag
	if rp != nil {
		fastTape = applyRecordedSettings(&cfg, settings) || fastTape
	}

	// TurboSound: on by default for the Pentagon, opt-in elsewhere.
	if *turbosoundFlag {
		cfg.HasTurboSound = true
	}

	// TurboSound FM (YM2203).
	if *turbosoundfmFlag {
		cfg.HasTurboSoundFM = true
	}

	// Disable FDC timing (fast loads for big, non-copy-protected images).
	if *nofdcFlag {
		cfg.NoFDCTiming = true
	}

	// Enable the General Sound card.
	if *gsFlag {
		cfg.HasGS = true
	}

	// Enable the 48K snow artefact.
	if *snowFlag {
		cfg.HasSnowEffect = true
	}

	fmt.Printf("zxgo-v3: %s\n", cfg.Name)

	// Start profiling if requested
	var profiler *Profiler
	if *profFlag {
		var err error
		profiler, err = StartProfiling()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to start profiling: %v\n", err)
			os.Exit(1)
		}
		defer func() {
			if err := profiler.StopProfiling(); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to stop profiling: %v\n", err)
			}
		}()
	}

	// Setup logger if -log flag is specified
	if *logFlag != "" {
		var level slog.Level
		switch *logFlag {
		case "debug":
			level = slog.LevelDebug
		case "info":
			level = slog.LevelInfo
		case "warn":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		default:
			fmt.Fprintf(os.Stderr, "Unknown log level: %s (use debug, info, warn, error)\n", *logFlag)
			os.Exit(1)
		}
		log := logger.New(os.Stderr, level)

		// Distribute loggers to subsystems
		media.SetDiskLogger(log.WithGroup("disk"), log.WithGroup("disk-ctrl"))
		sdl3.SetLogger(log.WithGroup("audio"))
		sdl.SetLogger(log.WithGroup("ui"))
		ula.SetULALogger(log.WithGroup("ula"))
		emulator.SetInterruptLogger(log.WithGroup("int"))
		cpu.SetInterruptLogger(log.WithGroup("int"))
	}

	audioOut := sdl3.New(44100)
	if err := audioOut.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Audio init failed, continuing without sound: %v\n", err)
	}

	// Create emulator using new ROM banking system
	emu, err := createEmulatorWithLayout(cfg, audioOut, "roms")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create emulator: %v\n", err)
		os.Exit(1)
	}

	// Fast tape load: run unthrottled while the tape plays, so LOAD completes
	// in host time. Audio is dropped for the duration (see Emulator.pumpAudio).
	emu.SetFastTape(fastTape)

	emu.Reset()

	// The media comes from the command line when it was given there, and from
	// the replay otherwise: a session that was recorded with a tape needs that
	// tape back, or the keys that typed LOAD "" land on a machine with nothing
	// to load. A recorded path that no longer resolves is reported and skipped
	// rather than fatal -- a session recorded off a BASIC prompt needs no media
	// at all, and refusing to start would help nobody.
	snapPath := *snaFlag
	if snapPath == "" {
		snapPath = *z80Flag
	}
	tapePath, diskPath := *tapFlag, *diskFlag
	if rp != nil {
		var warn []string
		if snapPath == "" {
			snapPath = resolveMedia(rp.Media.Snapshot, *replayFlag, &warn)
		}
		if tapePath == "" {
			tapePath = resolveMedia(rp.Media.Tape, *replayFlag, &warn)
		}
		if diskPath == "" {
			diskPath = resolveMedia(rp.Media.Disk, *replayFlag, &warn)
		}
		for _, w := range warn {
			fmt.Fprintf(os.Stderr, "zxgo-v3: replay media missing: %s\n", w)
		}
	}

	// Phase 6: Load snapshot file if specified. The path may be a .zip: the
	// emulator unpacks it and picks the snapshot format by the name inside.
	if snapPath != "" {
		if err := loadSnapshot(emu, snapPath); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load snapshot: %v\n", err)
			os.Exit(1)
		}
	}

	// Phase 6: Save snapshot on request
	if *saveSnaFlag != "" {
		defer func() {
			if err := saveSnapshot(emu, *saveSnaFlag); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to save SNA snapshot: %v\n", err)
			}
		}()
	}

	// Phase 5: Load TAP/TZX file if specified. The path may be a .zip holding
	// one: media.LoadTapeFile unpacks it and detects the format by the name
	// inside, so no caller here has to care.
	if tapePath != "" {
		tape, err := media.LoadTapeFile(tapePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load tape file: %v\n", err)
			os.Exit(1)
		}

		if err := emu.LoadTape(tape); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load tape into emulator: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("  Loaded %s: %s (%d blocks, %d pulses)\n",
			tape.Format, tapePath, len(tape.Blocks), len(tape.Pulses))
		// An archive can hold several images (a 48k and a 128k tape, say); say
		// which one was taken, since the pick is not otherwise visible.
		if tape.FileName != tapePath {
			fmt.Printf("  Archive entry: %s\n", tape.FileName)
		}

		// Tape is loaded but NOT playing - manual control like real tape recorder
		fmt.Println("  Tape loaded. Controls:")
		fmt.Println("    1. Type LOAD \"\" and press ENTER")
		fmt.Println("    2. Press CMD+P to start tape playback")
		fmt.Println("    3. Press CMD+P again to pause/stop tape")
		if fastTape {
			fmt.Println("  Fast tape: playback runs unthrottled; sound is dropped until it stops")
		}
		fmt.Println("  Press CMD+S at any time to save the screen as a PNG")
	}

	// Phase 7: Load disk image if specified (.trd/.scl/.dsk, optionally zipped).
	if diskPath != "" {
		disk, err := media.LoadDiskFile(diskPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load disk image: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("  Loaded %s disk: %s\n", disk.Type, diskPath)

		// Mount disk to Beta Disk controller
		if err := emu.LoadDisk(disk); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to mount disk: %v\n", err)
			os.Exit(1)
		}

		// Display disk information
		diskInfo := disk.GetDetailedInfo()
		fmt.Printf("  Disk: %s\n", diskInfo.Name)
		fmt.Printf("  Tracks: %d, Sectors: %d, Total: %d KB\n",
			diskInfo.Tracks, diskInfo.Sectors, diskInfo.SizeKB)
		fmt.Printf("  Free sectors: %d, Files: %d\n",
			diskInfo.FreeSectors, diskInfo.FileCount)

		fmt.Println("  Disk mounted and ready for TR-DOS operations")
	}

	// Recording and playback attach last, once the machine is in the state the
	// user will see: events are stamped from tick 0, which is the first
	// instruction after this point, and a replay is applied from that same
	// origin. Both may be attached at once -- playback goes through the same
	// input methods as a key press, so a replay records itself.
	if *saveReplayFlag != "" {
		rec := replay.NewRecorder()
		emu.SetRecorder(rec)
		// Save on exit, and like -save-sna a failure to write is reported but
		// does not stop the emulator from closing.
		defer func() {
			if err := saveReplay(emu, rec, *saveReplayFlag, modelKey,
				replay.Media{Snapshot: snapPath, Tape: tapePath, Disk: diskPath},
				fastTape); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to save replay: %v\n", err)
			}
		}()
		fmt.Printf("  Recording to %s\n", *saveReplayFlag)
	}

	if rp != nil {
		player := replay.NewPlayer(rp.Events, rp.CPUHz, emu.CPUHz())
		emu.SetPlayer(player)
		keys, tape := rp.Count()
		fmt.Printf("  Replay: %s (%d keys, %d tape actions, recorded on %s)\n",
			*replayFlag, keys, tape, rp.Model)
	}

	run(emu)
}

// applyRecordedSettings turns on every switch a session ran with, and reports
// whether it ran with fast tape.
//
// Fast tape is returned rather than applied because it is emulator state, not
// part of the machine config -- and that is exactly how it came to be recorded,
// printed and never used: a settings field nobody reads looks the same as one
// that works. Every field here is a switch that only turns something on, so a
// replay can add to a machine but never take away from it.
func applyRecordedSettings(cfg *model.Config, s replay.Settings) (fastTape bool) {
	if s.TurboSound {
		cfg.HasTurboSound = true
	}
	if s.TurboSoundFM {
		cfg.HasTurboSoundFM = true
	}
	if s.NoFDCTiming {
		cfg.NoFDCTiming = true
	}
	if s.GeneralSound {
		cfg.HasGS = true
	}
	if s.Snow {
		cfg.HasSnowEffect = true
	}
	return s.FastTape
}

// toggleTapePlayback starts or pauses the tape and reports the new state, or ok
// false when no tape is mounted.
//
// It goes through the emulator's transport methods rather than through the
// Playback object it wraps. That is not a style preference: recording hangs off
// the emulator's methods, so reaching past them to Playback.Pause() makes Cmd+P
// invisible to a recording -- which is exactly what it did, leaving replay files
// with the media named and no tape event to start it.
func toggleTapePlayback(emu *emulator.Emulator) (playing bool, ok bool) {
	if emu.TapePlayback() == nil {
		return false, false
	}
	if emu.TapePlayback().IsPlaying() {
		emu.StopTapePlayback()
		return false, true
	}
	emu.StartTapePlayback()
	return true, true
}

// flagWasSet reports whether a flag was named on the command line, as opposed to
// holding its default. Nothing else in the tree needs this: every other switch
// has a zero value that means "not asked for", but a replay file can say yes
// where the default says no, and "the user asked for 128k" has to be
// distinguishable from "the user did not mention the model".
func flagWasSet(name string) bool {
	set := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

// loadReplay reads a .replay file.
func loadReplay(path string) (*replay.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return replay.Load(f)
}

// resolveMedia finds a path a replay recorded: as it was recorded, then beside
// the .replay file itself, since a session handed to someone else usually
// arrives with its tape in the same folder. A path that resolves in neither
// place is appended to missing and reported by the caller.
func resolveMedia(recorded, replayPath string, missing *[]string) string {
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

// saveReplay writes the recorded session. The machine description comes from the
// emulator itself, so the file cannot disagree with what actually ran: the model
// key is the one that was looked up, and the switches are read out of the
// configured machine rather than out of the flags, which is what makes a
// Pentagon's always-on TurboSound reproduce on a model where it is not the
// default.
func saveReplay(emu *emulator.Emulator, rec *replay.Recorder, path, modelKey string,
	media replay.Media, fastTape bool) error {

	cfg := emu.ModelConfig()

	f := replay.NewFile()
	f.Model = modelKey
	f.ModelName = cfg.Name
	f.CPUHz = emu.CPUHz()
	f.Settings = replay.Settings{
		TurboSound:   cfg.HasTurboSound,
		TurboSoundFM: cfg.HasTurboSoundFM,
		GeneralSound: cfg.HasGS,
		Snow:         cfg.HasSnowEffect,
		NoFDCTiming:  cfg.NoFDCTiming,
		FastTape:     fastTape,
	}
	f.Media = media
	f.Events = rec.Events()

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	if err := replay.Save(out, f); err != nil {
		return err
	}

	keys, tape := f.Count()
	fmt.Printf("  Saved replay: %s (%d keys, %d tape actions, %s)\n",
		path, keys, tape, time.Duration(f.Duration()*int64(time.Second)/int64(emu.CPUHz())))
	return nil
}

// loadSnapshot loads a snapshot file (.sna or .z80, optionally packed in a .zip)
// into the emulator. The format is not passed in: it is whatever the file (or
// the archive entry) turns out to be, so -sna and -z80 both just work.
func loadSnapshot(emu *emulator.Emulator, filePath string) error {
	info, err := emu.LoadSnapshotFile(filePath)
	if err != nil {
		return err
	}
	fmt.Printf("  Loaded %s snapshot: %s (%s, %d bytes RAM)\n",
		info.Format, filePath, snapshotClass(info), info.RAMBytes)
	if info.Name != filePath {
		fmt.Printf("  Archive entry: %s\n", info.Name)
	}
	return nil
}

// snapshotClass names the machine a snapshot is for, for the load message. The
// hardware mode is the finer answer when the file carries one, since Is128K
// alone would report every banked machine as a plain 128K.
func snapshotClass(info emulator.SnapshotInfo) string {
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

// saveSnapshot writes the machine's state as an SNA file. The emulator builds
// it - including the 128K form, which this used to write as 48K whatever the
// machine was.
func saveSnapshot(emu *emulator.Emulator, filePath string) error {
	s, err := emu.CreateSNA()
	if err != nil {
		return fmt.Errorf("failed to create snapshot: %w", err)
	}

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create snapshot file: %w", err)
	}
	defer file.Close()

	if err := snap.SaveSNA(file, s); err != nil {
		return fmt.Errorf("failed to write SNA file: %w", err)
	}

	fmt.Printf("  Saved snapshot: %s (%s)\n", filePath,
		snapshotClass(emulator.SnapshotInfo{Is128K: s.Is128K}))
	return nil
}

func run(emu *emulator.Emulator) {
	gui := sdl.New(emu.ULA().Width(), emu.ULA().Height())
	if err := gui.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init SDL3: %v\n", err)
		os.Exit(1)
	}

	// Teardown order matters: audio must close BEFORE gui.Destroy(), which calls
	// SDL_Quit and tears down everything. Defers run LIFO, so this ordering is
	// correct as written -- do not reorder.
	defer gui.Destroy()
	defer emu.AudioOut().Close()

	onKey := func(row, col int, pressed bool) {
		if pressed {
			emu.PressKey(row, col)
		} else {
			emu.ReleaseKey(row, col)
		}
	}

	// Tape control commands (only Ghost key - Cmd+P)
	onCommand := func(command string, pressed bool) {
		if emu.TapePlayback() == nil && command == "tape-playpause" {
			return // No tape loaded for tape commands
		}

		switch command {
		case "screenshot":
			// One file per press: the name carries a millisecond stamp.
			path, err := ui.SavePNG(".", emu.ULA().GetScreen(), emu.ULA().Width(), emu.ULA().Height())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Screenshot failed: %v\n", err)
			} else {
				fmt.Println("Screenshot: " + path)
			}

		case "tape-playpause":
			if playing, ok := toggleTapePlayback(emu); ok {
				if playing {
					fmt.Println("Tape playing (Cmd+P)")
				} else {
					fmt.Println("Tape paused (Cmd+P)")
				}
			}
		}
	}

	var tapeEndNotified bool

	// lastFastRender paces presentation during a fast tape load. Presenting
	// every emulated frame would hand the loop to the display: SDL blocks in
	// SDL_RenderPresent when its drawable pool runs dry, so the load would run
	// at the refresh rate instead of at host speed. 100 ms keeps the window
	// visibly alive at a negligible cost.
	var lastFastRender time.Time

	// A recording is long and hard to reproduce, so an interrupt has to take the
	// same exit as closing the window: the loop returns, the deferred saves run,
	// and teardown stays in its documented order. Without this, Ctrl+C would kill
	// the process with the replay unwritten.
	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupted)

	for {
		// Check if tape just finished and notify user
		if emu.TapePlayback() != nil {
			if emu.TapePlayback().Ended() && !tapeEndNotified {
				fmt.Println("Tape ended - playback stopped")
				tapeEndNotified = true
			}
			// Reset notification when user restarts tape
			if !emu.TapePlayback().Ended() && tapeEndNotified {
				tapeEndNotified = false
			}
		}

		select {
		case <-interrupted:
			fmt.Println("Interrupted - saving")
			return
		default:
		}

		if !gui.ProcessEvents(onKey, onCommand) {
			return
		}
		if emu.RunSlice() {
			if !emu.FastTapeActive() {
				lastFastRender = time.Time{}
				gui.RenderFrame(emu.ULA().GetScreen())
			} else if now := time.Now(); now.Sub(lastFastRender) >= 100*time.Millisecond {
				lastFastRender = now
				gui.RenderFrame(emu.ULA().GetScreen())
			}
		}
	}
}
