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

	switch format {
	case "sna":
		snap, err := snap.LoadSNA(file)
		if err != nil {
			return fmt.Errorf("failed to parse SNA file: %w", err)
		}

		// Apply snapshot to emulator
		if err := applySnapshot(emu, snap); err != nil {
			return fmt.Errorf("failed to apply snapshot: %w", err)
		}

		// Determine model type from snapshot
		modelType := "48k"
		if snap.Is128K {
			modelType = "128k"
		}

		fmt.Printf("  Loaded SNA snapshot: %s (%s model, %d bytes RAM)\n",
			filePath, modelType, len(snap.RAM))

	case "z80":
		zsnap, err := snap.LoadZ80(file)
		if err != nil {
			return fmt.Errorf("failed to parse Z80 file: %w", err)
		}

		// Convert Z80 to emulator state
		if err := applyZ80Snapshot(emu, zsnap); err != nil {
			return fmt.Errorf("failed to apply Z80 snapshot: %w", err)
		}

		modelType := "48k"
		if zsnap.Is128K {
			modelType = "128k"
		}

		fmt.Printf("  Loaded Z80 snapshot: %s (%s model, %d bytes RAM)\n",
			filePath, modelType, len(zsnap.RAM))

	default:
		return fmt.Errorf("unsupported snapshot format: %s", format)
	}

	return nil
}

// saveSnapshot saves the current emulator state as an SNA file
func saveSnapshot(emu *emulator.Emulator, filePath string) error {
	snapshot, err := createSnapshot(emu)
	if err != nil {
		return fmt.Errorf("failed to create snapshot: %w", err)
	}

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create snapshot file: %w", err)
	}
	defer file.Close()

	if err := snap.SaveSNA(file, snapshot); err != nil {
		return fmt.Errorf("failed to write SNA file: %w", err)
	}

	fmt.Printf("  Saved snapshot: %s\n", filePath)
	return nil
}

// applySnapshot applies a loaded SNA snapshot to the emulator
func applySnapshot(emu *emulator.Emulator, snapshot *snap.Snapshot) error {
	// Validate snapshot compatibility
	if !snapshot.ValidFor48K() && !snapshot.ValidFor128K() {
		return fmt.Errorf("invalid snapshot: incompatible RAM size")
	}

	// Get CPU instance
	cpu := emu.CPU()

	// Set main registers from SNA header
	cpu.A = snapshot.Header.A
	cpu.F = snapshot.Header.F
	cpu.B, cpu.C = byte(snapshot.Header.BC>>8), byte(snapshot.Header.BC&0xFF)
	cpu.D, cpu.E = byte(snapshot.Header.DE>>8), byte(snapshot.Header.DE&0xFF)
	cpu.H, cpu.L = byte(snapshot.Header.HL>>8), byte(snapshot.Header.HL&0xFF)

	// Set alternate registers
	cpu.A_, cpu.F_ = byte(snapshot.Header.AF_>>8), byte(snapshot.Header.AF_&0xFF)
	cpu.B_, cpu.C_ = byte(snapshot.Header.BC_>>8), byte(snapshot.Header.BC_&0xFF)
	cpu.D_, cpu.E_ = byte(snapshot.Header.DE_>>8), byte(snapshot.Header.DE_&0xFF)
	cpu.H_, cpu.L_ = byte(snapshot.Header.HL_>>8), byte(snapshot.Header.HL_&0xFF)

	// Set index registers
	cpu.IX = snapshot.Header.IX
	cpu.IY = snapshot.Header.IY

	// Set special registers
	cpu.I = snapshot.Header.I
	cpu.R = snapshot.Header.R
	cpu.IM = snapshot.Header.IM
	cpu.SP = snapshot.Header.SP

	// Set interrupt flags from IFF2 (bit 2 = interrupt enabled)
	cpu.IFF1 = (snapshot.Header.IFF2 & 0x04) != 0
	cpu.IFF2 = cpu.IFF1

	// Load RAM into memory mapper (SNA RAM starts at 0x4000)
	mapper := emu.Mapper()
	if len(snapshot.RAM) != 49152 {
		return fmt.Errorf("invalid snapshot RAM size: expected 49152, got %d", len(snapshot.RAM))
	}

	// Write RAM to addresses 0x4000-0xFFFF
	for i := 0; i < 49152; i++ {
		addr := uint16(0x4000 + i)
		mapper.WriteByte(addr, snapshot.RAM[i])
	}

	// Set ULA border color
	ula := emu.ULA()
	ula.SetBorderColor(int(snapshot.Header.Border))

	// Pop PC from stack (SNA stores PC at stack pointer location)
	pc, err := snapshot.GetRegisterValue("PC")
	if err == nil {
		cpu.PC = uint16(pc)
		// Adjust stack pointer past the PC we just consumed
		cpu.SP += 2
	} else {
		// If PC couldn't be retrieved from stack, use a default
		cpu.PC = 0x8000 // Safe default for basic operation
	}

	return nil
}

// applyZ80Snapshot applies a loaded Z80 snapshot to the emulator
func applyZ80Snapshot(emu *emulator.Emulator, snapshot *snap.Z80Snapshot) error {
	// Get CPU instance
	cpu := emu.CPU()

	// Set main registers from Z80 header
	cpu.A = byte(snapshot.Header.AF >> 8)
	cpu.F = byte(snapshot.Header.AF & 0xFF)
	cpu.B, cpu.C = byte(snapshot.Header.BC>>8), byte(snapshot.Header.BC&0xFF)
	cpu.D, cpu.E = byte(snapshot.Header.DE>>8), byte(snapshot.Header.DE&0xFF)
	cpu.H, cpu.L = byte(snapshot.Header.HL>>8), byte(snapshot.Header.HL&0xFF)

	// Set alternate registers
	cpu.A_, cpu.F_ = byte(snapshot.Header.AF_>>8), byte(snapshot.Header.AF_&0xFF)
	cpu.H_, cpu.L_ = byte(snapshot.Header.HL_>>8), byte(snapshot.Header.HL_&0xFF)
	cpu.D_, cpu.E_ = byte(snapshot.Header.DE_>>8), byte(snapshot.Header.DE_&0xFF)
	cpu.B_, cpu.C_ = byte(snapshot.Header.BC_>>8), byte(snapshot.Header.BC_&0xFF)

	// Set index registers
	cpu.IX = snapshot.Header.IX
	cpu.IY = snapshot.Header.IY

	// Set special registers
	cpu.I = snapshot.Header.I
	cpu.R = snapshot.Header.R
	cpu.SP = snapshot.Header.SP
	cpu.PC = snapshot.Header.PC

	// Set interrupt flags: byte 27 = IFF1 (0=DI, otherwise EI), byte 28 = IFF2.
	cpu.IFF1 = snapshot.Header.IFF != 0
	cpu.IFF2 = snapshot.Header.IFF2 != 0

	// Set interrupt mode (byte 29, bits 0-1; already masked in the loader).
	cpu.IM = snapshot.Header.IM

	// Load RAM into memory mapper
	mapper := emu.Mapper()

	if snapshot.Is128K {
		// 128K format: RAM pages arranged according to Z80 page mapping
		// Pages 3-10 map to ZX Spectrum 128K pages 0-7
		expectedSize := 8 * 16384 // 8 pages * 16KB = 128KB

		if len(snapshot.RAM) == expectedSize {
			// Write each 16K RAM bank from the snapshot data
			for bank := 0; bank < 8; bank++ {
				bankStart := bank * 16384
				bankEnd := bankStart + 16384
				mapper.SetBank(bank, snapshot.RAM[bankStart:bankEnd])
			}
		} else {
			return fmt.Errorf("incomplete 128K Z80 snapshot: got %d bytes RAM, expected %d bytes for 128K format", len(snapshot.RAM), expectedSize)
		}
	} else {
		// 48K format: write full 48K RAM starting at 0x4000
		if len(snapshot.RAM) == 49152 {
			// Write full 48K RAM starting at 0x4000
			for i := 0; i < len(snapshot.RAM); i++ {
				addr := uint16(0x4000 + i)
				mapper.WriteByte(addr, snapshot.RAM[i])
			}
		} else if len(snapshot.RAM) > 0 {
			return fmt.Errorf("incomplete or compressed Z80 snapshot: got %d bytes RAM, expected 49152 for 48K format", len(snapshot.RAM))
		}
	}

	return nil
}

// createSnapshot creates an SNA snapshot from the current emulator state
func createSnapshot(emu *emulator.Emulator) (*snap.Snapshot, error) {
	// Get CPU instance
	cpu := emu.CPU()

	// Build SNA header from CPU state
	header := snap.SNAHeader{
		// Main registers
		A:  cpu.A,
		F:  cpu.F,
		BC: uint16(cpu.B)<<8 | uint16(cpu.C),
		DE: uint16(cpu.D)<<8 | uint16(cpu.E),
		HL: uint16(cpu.H)<<8 | uint16(cpu.L),

		// Alternate registers
		AF_: uint16(cpu.A_)<<8 | uint16(cpu.F_),
		BC_: uint16(cpu.B_)<<8 | uint16(cpu.C_),
		DE_: uint16(cpu.D_)<<8 | uint16(cpu.E_),
		HL_: uint16(cpu.H_)<<8 | uint16(cpu.L_),

		// Index registers
		IY: cpu.IY,
		IX: cpu.IX,

		// Special registers
		I:  cpu.I,
		R:  cpu.R,
		SP: cpu.SP,
		IM: cpu.IM,
	}

	// Set IFF2 from IFF1 state
	if cpu.IFF1 {
		header.IFF2 = 0x04
	}

	// Capture ULA state for border color
	ula := emu.ULA()
	header.Border = uint8(ula.BorderColor()) // Assuming there's aBorderColor() method

	// Create snapshot and capture RAM
	snapshot := &snap.Snapshot{
		Header: header,
		RAM:    make([]byte, 49152),
		Is128K: false, // Default to 48K for now
	}

	// Capture current RAM state from 0x4000-0xFFFF
	mapper := emu.Mapper()
	for i := 0; i < 49152; i++ {
		addr := uint16(0x4000 + i)
		snapshot.RAM[i] = mapper.ReadByte(addr)
	}

	return snapshot, nil
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
