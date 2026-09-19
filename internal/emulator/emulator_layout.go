package emulator

import (
	"fmt"
	"path/filepath"

	"github.com/kiltum/zxgo-v3/pkg/bus"
	"github.com/kiltum/zxgo-v3/pkg/cpu"
	"github.com/kiltum/zxgo-v3/pkg/gs"
	"github.com/kiltum/zxgo-v3/pkg/io_ports"
	"github.com/kiltum/zxgo-v3/pkg/media"
	"github.com/kiltum/zxgo-v3/pkg/mem"
	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/rom"
	"github.com/kiltum/zxgo-v3/pkg/sound"
	"github.com/kiltum/zxgo-v3/pkg/ula"
)

// NewWithLayout creates an emulator using the new ROM layout system.
// romsDir is the directory to search for ROM files (typically "roms/").
func NewWithLayout(cfg model.Config, layout *model.ROMLayout, romsDir string, audioOut sound.AudioOutput) (*Emulator, error) {
	// Shared TR-DOS arbitration state: the mapper sets it on ROM switches, the
	// Kempston reads it to stand down while TR-DOS is paged in. Per-emulator so
	// two machines in one process (the MCP worker) do not interfere.
	trdos := &io_ports.TRDosState{}

	// Create memory mapper with ROM layout
	mapper, err := mem.NewMapperWithLayout(cfg, layout, romsDir)
	if err != nil {
		return nil, fmt.Errorf("creating memory mapper: %w", err)
	}
	if s, ok := mapper.(interface{ SetTRDosState(*io_ports.TRDosState) }); ok {
		s.SetTRDosState(trdos)
	}

	portBus := io_ports.NewPortBus()

	// 48K has no paging ports: the TR-DOS ROM switch happens via the M1 fetch
	// trap, not a port write, so a 48K machine must not respond to 0x7FFD. Only
	// 128K/+2A/+3 register the mapper as a PortHandler for 0x7FFD/0x1FFD.
	if cfg.PagingModel != "none" {
		portBus.Register(mapper.(io_ports.PortHandler))
	}

	u := ula.New(mapper, cfg)
	portBus.Register(u)
	if cfg.HasFloatingBus {
		// The ULA supplies the data bus for port reads no peripheral drives
		// (the ZX floating bus). 48K/128K have it; +2A/+3 and Pentagon do not.
		portBus.SetFloatingBus(u)
	}

	// Model dispatch is by timing constant, not by name (see CLAUDE.md).
	// Pentagon's longer frame (71680 T-states) yields 71680*50 = 3.584 MHz.
	cpuHz := 3500000
	switch {
	case cfg.ClockEndFrame() == 71680: // Pentagon
		cpuHz = 3584000
	case cfg.ClockPerLine() == 228: // 128K
		cpuHz = 3546900
	}

	// Audio: sources record level events against the tick clock, the mixer
	// resolves them onto a fixed sample grid, a ring buffer feeds the device.
	// AY attaches here as a second source without touching any timing code.
	mixer := sound.NewMixer(audioOut, cpuHz)
	beeper := sound.NewBeeper()
	mixer.AddSource(beeper)
	portBus.Register(beeper)

	// AY sound source: a single AY-3-8912, a TurboSound pair (two AYs), or a
	// YM2203 (TurboSound FM). All run at HALF the CPU clock (1.7734 MHz for a
	// 128K) and share the 0xFFFD/0xBFFD ports, so exactly one is selected.
	var ay sound.AYChip
	switch {
	case cfg.HasTurboSoundFM:
		ts := sound.NewTSFM(sound.YMClock)
		ts.SetCPUClock(cpuHz)
		ts.SetSampleRate(audioOut.SampleRate())
		ay = ts
	case cfg.HasTurboSound:
		ay = sound.NewTurboSound(uint32(cpuHz / 2))
	default:
		single := sound.NewAY8912()
		single.SetClockFrequency(uint32(cpuHz / 2))
		ay = single
	}
	portBus.Register(ay)
	mixer.AddSource(ay)

	// Covox 8-bit DAC on port 0xFB (Pentagon/ATM standard; the Scorpion 0xDD
	// variant is added with the Scorpion model). Harmless on other models -- an
	// unwritten DAC outputs silence.
	covox := sound.NewCovox(0xFB)
	portBus.Register(covox)
	mixer.AddSource(covox)

	// General Sound card (optional): a second Z80 + 4-channel DAC on host ports
	// 0xBB/0xB3, enabled via the -gs flag.
	var gsCard *gs.GS
	if cfg.HasGS {
		gsCard = gs.New(cpuHz)
		// Load the GS ROM: embedded, overridden by roms/gs105a.rom if present.
		// The override may be packed (roms/gs105a.rom.zip), like every other
		// image loader here.
		gsROM := rom.Get("gs/gs105a.rom")
		if data, err := mem.ReadROMFile(filepath.Join(romsDir, "gs105a.rom")); err == nil {
			gsROM = data
		}
		if err := gsCard.LoadROM(gsROM); err != nil {
			return nil, fmt.Errorf("loading GS ROM: %w", err)
		}
		portBus.Register(gsCard)
		mixer.AddSource(gsCard)
	}

	kempston := io_ports.NewKempston()
	kempston.SetTRDosState(trdos)
	portBus.Register(kempston)

	// Phase 7: Beta Disk controller (for all models that support -disk flag)
	betaDisk := media.NewBetaDiskController()
	betaDisk.SetClockHz(cpuHz)
	betaDisk.SetNoTiming(cfg.NoFDCTiming)
	portBus.Register(betaDisk)

	// The +2A/+3 has its floppy controller built in, so it exists whether or not
	// a disk is mounted. It used to be created on the first mount, which made a
	// machine that had not been given a disk behave as if it had no controller -
	// and, once state files existed, left a restored session with nowhere to put
	// the disk the file carried. A machine with no disk still has a controller
	// that reports "no disk"; it does not have a hole where the ports should be.
	var upd765 *media.UPD765
	if cfg.PagingModel == "2a3" {
		upd765 = media.NewUPD765()
		upd765.SetClockHz(cpuHz)
		upd765.SetNoTiming(cfg.NoFDCTiming)
		portBus.Register(upd765)
	}

	emuBus := bus.NewDefaultBus(mapper, portBus)

	trace := newTrace(65536)
	emuBus.SetWriteTrace(trace.recordWrite)

	// Snow effect: a CPU write to contended memory drives the data bus, and the
	// ULA reads its screen bytes off that same bus, so an overlapping fetch sees
	// the CPU's byte. Opt-in via Config.HasSnowEffect ("-snow") because it is an
	// authentic 48K artefact that most people do not want to look at, and because
	// on the 128K the ROM's vector table lives in contended RAM, where the same
	// rule grains the whole picture.
	if cfg.HasSnowEffect {
		u.SetSnowEffect(true)
		emuBus.SetMCycleCallback(func(kind bus.MCycleKind, addr uint16, tick int64, value uint8) {
			if kind != bus.MCycleMemWrite {
				return
			}
			if mapper.IsContended(addr) {
				u.SnowWrite(tick, value)
			}
		})
	}

	return &Emulator{
		cfg:      cfg,
		cpu:      cpu.New(emuBus),
		ula:      u,
		mapper:   mapper,
		portBus:  portBus,
		bus:      emuBus,
		beeper:   beeper,
		ay:       ay,
		covox:    covox,
		gs:       gsCard,
		mixer:    mixer,
		kempston: kempston,
		audioOut: audioOut,
		cpuHz:    cpuHz,
		betaDisk: betaDisk,
		upd765:   upd765,
		trace:    trace,
	}, nil
}
