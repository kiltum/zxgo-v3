package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/kiltum/zxgo-v3/internal/emulator"
	"github.com/kiltum/zxgo-v3/internal/ui"
	"github.com/kiltum/zxgo-v3/internal/ui/sdl"
	"github.com/kiltum/zxgo-v3/pkg/cpu"
	"github.com/kiltum/zxgo-v3/pkg/logger"
	"github.com/kiltum/zxgo-v3/pkg/media"
	"github.com/kiltum/zxgo-v3/pkg/model"
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
	flag.Parse()

	cfg, ok := model.AllModels[*modelFlag]
	if !ok {
		fmt.Fprintf(os.Stderr, "Unknown model: %s\n", *modelFlag)
		os.Exit(1)
	}

	// A/B test override for the /INT position (Pentagon timing debugging).
	if v := os.Getenv("ZXGO_INT_OFFSET"); v != "" {
		if off, err := strconv.Atoi(v); err == nil {
			fmt.Printf("zxgo-v3: overriding InterruptOffset %d -> %d\n", cfg.InterruptOffset, off)
			cfg.InterruptOffset = off
		}
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
	emu.SetFastTape(*fastTapeFlag)

	emu.Reset()

	// Phase 6: Load snapshot file if specified
	if *snaFlag != "" {
		if err := loadSnapshot(emu, *snaFlag, "sna"); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load SNA snapshot: %v\n", err)
			os.Exit(1)
		}
	}

	if *z80Flag != "" {
		if err := loadSnapshot(emu, *z80Flag, "z80"); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load Z80 snapshot: %v\n", err)
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

	// Phase 5: Load TAP/TZX file if specified
	if *tapFlag != "" {
		tapFile, err := os.Open(*tapFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open tape file: %v\n", err)
			os.Exit(1)
		}
		defer tapFile.Close()

		var tape *media.Tape
		// Detect format by file extension (though content should also be checked in production)
		if len(*tapFlag) > 4 && (*tapFlag)[len(*tapFlag)-4:] == ".tzx" {
			tape, err = media.LoadTZX(tapFile, *tapFlag)
		} else {
			tape, err = media.LoadTAP(tapFile, *tapFlag)
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to parse tape file: %v\n", err)
			os.Exit(1)
		}

		if err := emu.LoadTape(tape); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load tape into emulator: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("  Loaded %s: %s (%d blocks, %d pulses)\n",
			tape.Format, *tapFlag, len(tape.Blocks), len(tape.Pulses))

		// Tape is loaded but NOT playing - manual control like real tape recorder
		fmt.Println("  Tape loaded. Controls:")
		fmt.Println("    1. Type LOAD \"\" and press ENTER")
		fmt.Println("    2. Press CMD+P to start tape playback")
		fmt.Println("    3. Press CMD+P again to pause/stop tape")
		if *fastTapeFlag {
			fmt.Println("  Fast tape: playback runs unthrottled; sound is dropped until it stops")
		}
		fmt.Println("  Press CMD+S at any time to save the screen as a PNG")
	}

	// Phase 7: Load disk image if specified
	if *diskFlag != "" {
		diskFile, err := os.Open(*diskFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open disk image: %v\n", err)
			os.Exit(1)
		}
		defer diskFile.Close()

		// Detect disk format by file extension
		ext := filepath.Ext(*diskFlag)
		var disk *media.Disk

		switch strings.ToLower(ext) {
		case ".trd", ".trd0", ".trd1":
			disk, err = media.LoadTRDisk(diskFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to load TR-DOS disk image: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("  Loaded TR-DOS disk: %s\n", *diskFlag)

		case ".scl":
			// SCL format - our media library now supports it with 99.9% success rate
			disk, err = media.LoadSCL(diskFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to load SCL disk image: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("  Loaded SCL disk: %s\n", *diskFlag)

		case ".dsk":
			// +3 DOS format (or alternative formats detected)
			disk, err = media.LoadDSK(diskFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to load DSK disk image: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("  Loaded DSK disk: %s\n", *diskFlag)

		default:
			// Try to detect format by content
			fmt.Fprintf(os.Stderr, "Unknown disk format: %s (supported: .trd)\n", ext)
			os.Exit(1)
		}

		if disk == nil {
			fmt.Fprintln(os.Stderr, "Failed to load disk image")
			os.Exit(1)
		}

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

	run(emu)
}

// loadSnapshot loads a snapshot file (SNA or Z80) into the emulator
func loadSnapshot(emu *emulator.Emulator, filePath, format string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open snapshot file: %w", err)
	}
	defer file.Close()

	// Parsing lives in pkg/snap, applying lives in internal/emulator: the
	// emulator is the only thing that knows which machine it is, which is what
	// lets it refuse a snapshot for a machine this is not.
	switch format {
	case "sna":
		s, err := snap.LoadSNA(file)
		if err != nil {
			return fmt.Errorf("failed to parse SNA file: %w", err)
		}
		if err := emu.LoadSNA(s); err != nil {
			return err
		}
		// The SNA format carries no machine byte: 128K-ness is implied by the
		// file length, so a banked machine reports as "128k".
		fmt.Printf("  Loaded SNA snapshot: %s (%s, %d bytes RAM)\n",
			filePath, snapshotClass(s.Is128K, 0), len(s.RAM))

	case "z80":
		z, err := snap.LoadZ80(file)
		if err != nil {
			return fmt.Errorf("failed to parse Z80 file: %w", err)
		}
		if err := emu.LoadZ80(z); err != nil {
			return err
		}
		fmt.Printf("  Loaded Z80 snapshot: %s (%s, %d bytes RAM)\n",
			filePath, snapshotClass(z.Is128K, z.HardwareMode), len(z.RAM))

	default:
		return fmt.Errorf("unsupported snapshot format: %s", format)
	}
	return nil
}

// snapshotClass names the machine a snapshot is for, for the load message. The
// hardware mode is the finer answer when the file carries one, since Is128K
// alone would report every banked machine as a plain 128K.
func snapshotClass(is128K bool, hardwareMode uint8) string {
	if !is128K {
		return "48k"
	}
	switch hardwareMode {
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

	fmt.Printf("  Saved snapshot: %s (%s)\n", filePath, snapshotClass(s.Is128K, 0))
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
			pb := emu.TapePlayback()
			if pb.IsPlaying() {
				pb.Pause()
				fmt.Println("Tape paused (Cmd+P)")
			} else {
				pb.Play()
				fmt.Println("Tape playing (Cmd+P)")
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
