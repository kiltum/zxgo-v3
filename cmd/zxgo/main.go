package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/kiltum/zxgo-v3/internal/emulator"
	"github.com/kiltum/zxgo-v3/internal/frontend"
	"github.com/kiltum/zxgo-v3/internal/ui/imgui"
	"github.com/kiltum/zxgo-v3/pkg/cpu"
	"github.com/kiltum/zxgo-v3/pkg/logger"
	"github.com/kiltum/zxgo-v3/pkg/media"
	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/replay"
	"github.com/kiltum/zxgo-v3/pkg/sound/sdl3"
	"github.com/kiltum/zxgo-v3/pkg/ula"
)

// SDL3's macOS Cocoa backend requires the first OS thread, so lock to it before
// SDL touches anything.
func init() { runtime.LockOSThread() }

func main() {
	modelFlag := flag.String("model", "48k", "ZX Spectrum model: 48k, 128k, 2a3, pentagon, pentagon512")
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
	replayFlag := flag.String("replay", "", "path to a replay to play back; its machine and switches are used ("+frontend.ReplayExt+" is added if the name does not carry it)")
	saveReplayFlag := flag.String("save-replay", "", "path to write a recording of this session on exit ("+frontend.ReplayExt+" is added if the name does not carry it)")
	loadStateFlag := flag.String("load-state", "", "path to a session to resume from ("+frontend.StateExt+" is added if the name does not carry it)")
	saveStateFlag := flag.String("save-state", "", "path to write a session on exit ("+frontend.StateExt+" is added if the name does not carry it)")
	sessionFlag := flag.String("session", "", "path to a session: resumed if it exists, written on exit ("+frontend.StateExt+" is added if the name does not carry it)")
	flag.Parse()

	// A relaunch is the same program started again, not a new machine: the emulator builds its
	// peripheral graph once (UI_DESIGN.md D10), so the settings that need one are applied by
	// running the command line again. This defer is registered before every other one so that
	// it runs *after* them - the session, the snapshot and any recording are written on the way
	// out, exactly as they are for a quit, and only then is the image replaced.
	var relaunch bool
	// app is declared here rather than at the construction below, which is where it belongs,
	// because the relaunch reads the settings out of it on the way out. It is assigned once,
	// before the loop, and never nil when the relaunch runs: only the loop can ask for one.
	var app *frontend.App
	defer func() {
		if relaunch {
			relaunchSelf(app.Settings())
		}
	}()

	// A replay is read before the machine is built: it carries the model and the
	// switches the session ran with, so it has to be in hand before there is a
	// config to fill in.
	var rp *replay.File
	replayPath := frontend.LoadPath(*replayFlag, frontend.ReplayExt)
	if replayPath != "" {
		loaded, err := frontend.LoadReplay(replayPath)
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
	// The config directory: nothing else is read from it until the machine is built, but
	// where it is decides which model is built, and one of the things in it - the model the
	// user chose in the settings window - decides that too, so both are resolved first.
	store, err := frontend.DefaultStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "zxgo-v3: %v\n", err)
	}
	if store.Dir != "" {
		if err := store.EnsureDir(); err != nil {
			fmt.Fprintf(os.Stderr, "zxgo-v3: %v\n", err)
		}
	}

	// The front end's preferences are read before anything that depends on them: the model
	// below, the switches after it, and the session path after that. `prefs` rather than
	// `settings`: that name belongs to the replay's recorded switches, further down.
	prefs, err := store.LoadSettings()
	if err != nil {
		fmt.Fprintf(os.Stderr, "zxgo-v3: %v\n", err)
	}

	// The model, in order of who asked for it: the flag, then the replay (it was recorded on
	// one), then the settings window's choice, then the session in the config directory - a
	// user who says nothing is asking to be where they were, and that may well be a machine
	// they did not name. A model that *has* been chosen in the settings window wins over that
	// history: a user who named a machine is not asking to be where they were.
	modelKey := *modelFlag
	if !flagWasSet("model") {
		switch {
		case rp != nil:
			modelKey = rp.Model
		case prefs.Model != "":
			modelKey = prefs.Model
		default:
			if last, ok := store.LatestSession(); ok {
				modelKey = last
			}
		}
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

	// Fast tape is not part of the machine config (it is how the host runs, not what the
	// machine is), so it is applied to the emulator rather than through ApplyTo - and it
	// follows the same order as everything else: the replay's recording of it, then the
	// standing setting, then the flag.
	fastTape := *fastTapeFlag
	if rp != nil {
		fastTape = frontend.ApplyRecordedSettings(&cfg, settings) || fastTape
	}

	// The settings window's switches come next, and the command line's flags last: the order
	// is "the standing configuration, then the per-run override", the same order -model
	// follows. A setting the file does not mention leaves what the replay decided, and a flag
	// the user typed wins, because typing it is a decision about this run - which is also why
	// a relaunch drops those flags rather than re-running them (see relaunchArgs: the window
	// has just changed one of these and the new process must see the change).
	prefs.ApplyTo(&cfg)
	if prefs.FastTape != nil {
		fastTape = *prefs.FastTape
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
		frontend.SetLogger(log.WithGroup("ui"))
		imgui.SetLogger(log.WithGroup("ui"))
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

	// A session is resumed before anything the command line names, so a -tap,
	// -disk or -sna given as well replaces what the session had rather than
	// being replaced by it. That is the precedence a replay already uses: the
	// command line wins, and the file fills in the rest.
	//
	// The file may name another model, be corrupt, or not be there at all. None
	// of those is fatal - the machine starts clean and says so - because a
	// session is a convenience and refusing to boot would make a stale file
	// worse than no file.
	// The front end is built here rather than with the window: the session is resumed before
	// the media the command line names, and a recording starts after them, so both need it
	// before there is anything to draw on.
	app = frontend.New(frontend.NewMachine(emu))

	// The session is read from and written to the paths the command line gave: -session
	// names both halves, -load-state and -save-state one each, and the config directory's
	// session is the default when neither was named and the setting is on.
	//
	// A replay run has no session at all - see frontend.SessionFiles, which is where the
	// rule lives - and a user who named one explicitly is told why rather than ignored.
	replayRun := rp != nil || *saveReplayFlag != ""
	saveExplicit := frontend.LoadPath(frontend.StatePathFor(*saveStateFlag, *sessionFlag), frontend.StateExt)
	resumePath, saveTo, err := frontend.SessionFiles(frontend.SessionRequest{
		LoadExplicit: frontend.LoadPath(frontend.StatePathFor(*loadStateFlag, *sessionFlag), frontend.StateExt),
		SaveExplicit: saveExplicit,
		Settings:     prefs,
		Store:        store,
		Model:        cfg.Key,
		Replay:       replayRun,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "zxgo-v3: no session: %v\n", err)
	}

	if resumePath != "" {
		msg, err := frontend.RestoreSession(frontend.NewMachine(emu), resumePath)
		switch {
		case err != nil:
			fmt.Fprintf(os.Stderr, "zxgo-v3: not resuming %s: %v\n", resumePath, err)
		case msg != "":
			fmt.Printf("zxgo-v3: %s\n", msg)
		}
	}

	// The Z80 variant is applied *after* the session, and without the reset a live change
	// takes (see Emulator.SetCPUType). After, because a session carries the CPU it was saved
	// with (pkg/cpu/state.go) and choosing a variant in the settings window is the more
	// recent word - applying it before the session would be a setting the session always
	// overruled, which is the same as not having one. Without the reset, because the machine
	// has just been restored and a reset here would throw that state away. A file that names
	// no variant leaves both alone.
	if isNMOS, ok := prefs.Z80Choice(); ok {
		emu.SetCPUType(isNMOS)
		fmt.Printf("  Z80: %s\n", map[bool]string{true: "NMOS", false: "CMOS"}[isNMOS])
	}

	// MEMPTR's behaviour is the other half of the same idea, and it is applied the same way
	// and in the same place: it is CPU state a session carries too, so it goes on after the
	// session is resumed. It needs no reset at all - it decides a value the next block
	// instruction leaves, not one already computed.
	if realMem, ok := prefs.MEMPTRRealChoice(); ok {
		emu.SetMEMPTRReal(realMem)
		fmt.Printf("  MEMPTR: %s\n", map[bool]string{true: "measured", false: "documented"}[realMem])
	}

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
			snapPath = frontend.ResolveMedia(rp.Media.Snapshot, replayPath, &warn)
		}
		if tapePath == "" {
			tapePath = frontend.ResolveMedia(rp.Media.Tape, replayPath, &warn)
		}
		if diskPath == "" {
			diskPath = frontend.ResolveMedia(rp.Media.Disk, replayPath, &warn)
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
			msg, err := frontend.SaveSnapshot(frontend.NewMachine(emu), *saveSnaFlag)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to save SNA snapshot: %v\n", err)
				return
			}
			fmt.Printf("  %s\n", msg)
		}()
	}

	// The session is written on the way out, which is the same graceful-exit
	// path the replay recording uses: the loop returns on the window closing or
	// on an interrupt, and the deferred saves run. It is written deflated: RAM
	// and a mostly empty disk compress well, and a session is not a file anyone
	// reads in an editor.
	// A recording started from a window is not visible here, so the exit checks the app as
	// well: a replay run has no session whether the replay was named on the command line or
	// asked for from the File menu.
	if saveTo != "" && !app.Recording() {
		defer func() {
			// The settings window can turn the session off while the machine runs, so the
			// setting is asked again here rather than only at startup. Only a session the
			// config directory owns is affected: a path named on the command line is the
			// user's decision, and the command line outranks a setting - the same order
			// -model follows. (Turned *on* mid-run, a session starts at the next launch: the
			// half that reads one happens before there is a window to turn it on in.)
			if !app.Settings().SessionEnabled() && saveExplicit == "" {
				fmt.Println("  Session not saved: turned off in the settings window")
				return
			}
			msg, err := frontend.SaveSessionOnExit(frontend.NewMachine(emu), saveTo)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to save session: %v\n", err)
				return
			}
			fmt.Printf("%s\n", msg)
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
	if recPath := frontend.SavePath(*saveReplayFlag, frontend.ReplayExt); recPath != "" {
		// The front end owns a recording, so the command line and a window take the same
		// path: the recorder hangs off the machine's input methods either way (D9).
		app.SetMediaPaths(frontend.MediaPaths{
			Snapshot: snapPath, Tape: tapePath, Disk: diskPath,
		})
		app.StartRecording(recPath)
		// Save on exit, and like -save-sna a failure to write is reported but does not
		// stop the emulator from closing.
		defer func() {
			msg, err := app.SaveRecording()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to save replay: %v\n", err)
				return
			}
			fmt.Printf("  %s\n", msg)
		}()
		fmt.Printf("  Recording to %s\n", recPath)
	}

	if rp != nil {
		player := replay.NewPlayer(rp.Events, rp.CPUHz, emu.CPUHz())
		emu.SetPlayer(player)
		keys, tape, joy := rp.Count()
		fmt.Printf("  Replay: %s (%d keys, %d tape actions, %d joystick moves, recorded on %s)\n",
			replayPath, keys, tape, joy, rp.Model)
	}

	runImGui(app, emu, store, prefs)

	// The loop has returned and the deferred saves above are about to run. Asking for a
	// relaunch is read here rather than inside the loop because the new process must start
	// after them: it would resume a session that has not been written yet, and a recording
	// would be cut off at the relaunch.
	relaunch = app.RelaunchWanted()
}

// relaunchSelf starts the emulator again with the machine the settings now name.
//
// The command line is the one this process was started with, with the model replaced: the
// settings window writes the switches to settings.json, and the new process reads them
// there, so nothing else has to be carried across. The -model flag is the exception, because
// an explicit flag wins over the settings (a user who named a machine on the command line
// meant it) - and the flag the user gave is exactly what a relaunch has to override, since
// the whole point of it is that the machine changed.
//
// The settings are written here rather than assumed to be on disk: the loop persists them
// when they change, but it leaves the moment the relaunch is asked for, so the last change -
// the one that made the user reach for the button - may not have been written yet.
//
// **It is exec, not a second process.** Starting another copy with os/exec made the new
// emulator a *child of a dying Cocoa application*, and on macOS that is not the same
// situation as a launch from a shell: it inherits the parent's window-server connections and
// it is not a front-end application in its own right. The observed result was the new process
// dying inside SDL_Init, in AppKit's own startup. exec replaces this process's image instead,
// so the emulator that comes up has the same pid, the same terminal and the same Dock
// presence - it is this program, restarted - and there is no moment at which two copies of
// it exist.
//
// Everything that had to happen before it - the session snapshot, a recording, the audio
// device, SDL itself - has already happened: this runs from the outermost defer, so every
// other one has run first.
func relaunchSelf(set frontend.Settings) {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "zxgo-v3: cannot relaunch: %v\n", err)
		return
	}

	store, err := frontend.DefaultStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "zxgo-v3: cannot relaunch: %v\n", err)
		return
	}
	if store.Dir != "" {
		if err := store.SaveSettings(set); err != nil {
			fmt.Fprintf(os.Stderr, "zxgo-v3: %v\n", err)
		}
	}

	args := append([]string{exe}, relaunchArgs(os.Args[1:], set.Model)...)
	fmt.Printf("zxgo-v3: relaunching: %s\n", strings.Join(args, " "))

	// Exec only returns on failure, and then the emulator simply stays closed: the user has
	// the command line above and can run it themselves.
	if err := syscall.Exec(exe, args, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "zxgo-v3: cannot relaunch (%v); run it with:\n  %s\n",
			err, strings.Join(args, " "))
	}
}

// relaunchArgs is the command line for a relaunched process: the one it was given, with the
// machine flags removed.
//
// Which machine and which sound devices are the settings' business now - the window has just
// changed one of them, and the new process reads settings.json - and a flag left on the line
// would override the very change the user asked for, since a per-run flag beats the file (see
// main). Everything else is carried across: the media, the replay, the session paths, the log
// level. A relaunch is the same run on another machine, not a fresh start.
//
// The model is re-added at the end, which is that same rule seen from the other side: the
// user's -model is the one flag the relaunch must displace, because the flag beats the file
// and the *file* is where the window put the choice.
func relaunchArgs(args []string, modelKey string) []string {
	// The names a machine is built from. -model's value is a separate argument in its
	// two-word form; the switches are booleans, which Go's flag package takes bare or as
	// -name=false, so only -model has an argument to swallow.
	dropValue := map[string]bool{"-model": true}
	drop := map[string]bool{
		"-model": true, "-turbosound": true, "-turbosoundfm": true,
		"-gs": true, "-snow": true,
	}

	out := make([]string, 0, len(args)+2)
	for i := 0; i < len(args); i++ {
		a := args[i]
		name := a
		if eq := strings.IndexByte(a, '='); eq >= 0 {
			name = a[:eq]
		}
		if !drop[name] {
			out = append(out, a)
			continue
		}
		if dropValue[name] && name == a {
			i++ // the value is the next argument
		}
	}
	if modelKey != "" {
		out = append(out, "-model", modelKey)
	}
	return out
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

// loadSnapshot loads a snapshot file (.sna or .z80, optionally packed in a .zip)
// into the emulator. The format is not passed in: it is whatever the file (or
// the archive entry) turns out to be, so -sna and -z80 both just work.
func loadSnapshot(emu *emulator.Emulator, filePath string) error {
	info, err := emu.LoadSnapshotFile(filePath)
	if err != nil {
		return err
	}
	fmt.Printf("  Loaded %s snapshot: %s (%s, %d bytes RAM)\n",
		info.Format, filePath,
		frontend.SnapshotClass(frontend.SnapshotInfo{
			Is128K: info.Is128K, HardwareMode: info.HardwareMode}), info.RAMBytes)
	if info.Name != filePath {
		fmt.Printf("  Archive entry: %s\n", info.Name)
	}
	return nil
}

// runImGui owns the desktop windows: the main window with the machine's screen and
// the menubar, and one OS window per tool (UI_DESIGN.md M2).
//
// It takes the app and the emulator both. The app is what it drives; the emulator is here
// for one line of teardown, and it has to be *this* function's defer because the audio must
// close before the backend destroys the windows - a defer in main would run after it.
//
// The window layout is a UI preference rather than machine state, so it is
// restored whether or not the session setting is on (D4). A store or a layout file
// that cannot be read is reported and ignored: the defaults come back and the
// emulator starts (D3).
func runImGui(app *frontend.App, emu *emulator.Emulator, store frontend.Store, prefs frontend.Settings) {
	if store.Dir != "" {
		// A layout is applied when there is one. When there is not, the loop places
		// the main window itself: it is the one that knows how big the screen is and
		// which displays are attached (see PlaceMainDefault).
		layout, err := store.LoadLayout()
		switch {
		case err != nil:
			fmt.Fprintf(os.Stderr, "zxgo-v3: %v\n", err)
		case layout != nil:
			app.SetLayout(*layout)
		}

		// The user's key bindings are a table over the defaults, so what the file
		// holds is their changes. A file that is partly unreadable is still applied
		// as far as it goes: the report above says which entries were skipped.
		user, err := store.LoadKeymap()
		if err != nil {
			fmt.Fprintf(os.Stderr, "zxgo-v3: %v\n", err)
		}
		if len(user) > 0 {
			app.SetKeymap(frontend.Merge(frontend.DefaultBindings(), user))
		}

		// And which ZX key a host key stands for, from its own file: the letters, the
		// cursor keys, BACKSPACE being CAPS SHIFT and 0. It is a separate concern from
		// the actions above and has a separate file (D3).
		keys, err := store.LoadMatrix()
		if err != nil {
			fmt.Fprintf(os.Stderr, "zxgo-v3: %v\n", err)
		}
		if len(keys) > 0 {
			app.SetMatrix(frontend.MergeMatrix(frontend.DefaultMatrixMap(), keys))
		}

		// Where the file dialogs open, and whether the session is carried between runs:
		// read in main, because the session path depends on the setting.
		app.SetSettings(prefs)
	}

	backend := imgui.New()
	backend.IniDir = store.Dir
	if err := backend.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init the ImGui backend: %v\n", err)
		os.Exit(1)
	}

	// Teardown order matters: audio must close BEFORE the backend destroys the
	// windows, because the backend's shutdown calls SDL_Quit and tears down every
	// subsystem. Defers run LIFO, so this ordering is correct as written -- do not
	// reorder.
	defer backend.Destroy()
	defer emu.AudioOut().Close()

	// A recording is long and hard to reproduce, so an interrupt has to take the
	// same exit as closing the window: the loop returns, the deferred saves run,
	// and teardown stays in its documented order.
	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupted)

	loop := &frontend.BackendLoop{
		App:       app,
		Backend:   backend,
		Interrupt: interrupted,
		LayoutChanged: func(l frontend.Layout) {
			if store.Dir == "" {
				return
			}
			if err := store.SaveLayout(l); err != nil {
				fmt.Fprintf(os.Stderr, "zxgo-v3: %v\n", err)
			}
		},
		SettingsChanged: func(set frontend.Settings) {
			if store.Dir == "" {
				return
			}
			if err := store.SaveSettings(set); err != nil {
				fmt.Fprintf(os.Stderr, "zxgo-v3: %v\n", err)
			}
		},
	}
	loop.Run()
}
