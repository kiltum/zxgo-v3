package media

import (
	"fmt"
	"io"
)

// Pulse represents a single EAR pulse with duration in T-states.
// The ZX Spectrum tape format is pulse-based: a sequence of high/low
// transitions that the ROM loader measures to decode bits.
type Pulse struct {
	Level    bool  // true = high, false = low
	Duration int64 // Duration in T-states
}

// Block represents a decoded tape block with its data and timing.
type Block struct {
	Data      []byte  // Raw block data
	Flag      uint8   // 0x00=header, 0xFF=data
	BlockType uint8   // Header type (0-7) if flag==0x00
}

// Tape represents a complete tape (TAP/TZX file) as a sequence of pulses.
// The pulse stream drives the EAR bit through port 0xFE bit 6.
type Tape struct {
	FileName string
	Format   string   // "TAP" or "TZX"
	Pulses   []Pulse  // Ordered sequence of pulses (the bitstream)
	Blocks   []Block  // Decoded data blocks (for inspection/debugging)
}

// Playback handles tape playback state and synchronization with CPU T-states.
type Playback struct {
	Tape        *Tape
	Pos         int   // Current pulse index
	TapeTick    int64 // Tape's own tick counter (only advances when active)
	NextTick    int64 // Next tape tick when level should change
	Active      bool  // Playback active
	LastCPUTick int64 // Last CPU tick seen (for delta calculation)
	EndNotified bool  // Track if we've notified user that tape ended
}

// NewPlayback creates a new tape playback controller.
func NewPlayback(tape *Tape) *Playback {
	pb := &Playback{
		Tape:        tape,
		Pos:         0,
		Active:      false,
		TapeTick:    0,  // Tape starts at position 0
		NextTick:    0,  // Will be set when first pulse starts
		LastCPUTick: 0,  // Track delta for timing
		EndNotified: false, // Track if we've notified tape ended
	}
	return pb
}

// CurrentLevel returns the current EAR level at the given CPU tick.
// The ULA reads this through port 0xFE bit 6.
// CRITICAL: Tape only advances when Active=true, using relative timing.
func (pb *Playback) CurrentLevel(currentTick int64) bool {
	if pb.Tape == nil || len(pb.Tape.Pulses) == 0 || !pb.Active {
		return true // EAR bit floats high when no tape or stopped
	}

	// Calculate CPU delta and advance tape tick counter only when active
	if pb.Active && pb.LastCPUTick >= 0 {
		cpuDelta := currentTick - pb.LastCPUTick
		pb.TapeTick += cpuDelta // Only advance tape when active
	}
	pb.LastCPUTick = currentTick

	// Initialize NextTick on first active call
	if pb.NextTick == 0 && pb.Pos == 0 {
		if len(pb.Tape.Pulses) > 0 {
			pb.NextTick = pb.Tape.Pulses[0].Duration
		}
	}

	// Advance position if we've passed the next transition (using tape clock, not CPU clock)
	for pb.Pos < len(pb.Tape.Pulses) && pb.TapeTick >= pb.NextTick {
		pb.Pos++
		if pb.Pos < len(pb.Tape.Pulses) {
			pb.NextTick += pb.Tape.Pulses[pb.Pos].Duration
		}
	}

	if pb.Pos >= len(pb.Tape.Pulses) {
			if pb.Active && !pb.EndNotified {
				// Auto-stop playback when tape ends
				pb.Active = false
				pb.EndNotified = true
			}
			return true // End of tape: EAR floats high
		}

	return pb.Tape.Pulses[pb.Pos].Level
}

// Reset rewinds to the beginning.
func (pb *Playback) Reset() {
	pb.Pos = 0
	pb.TapeTick = 0
	pb.NextTick = 0
	pb.LastCPUTick = 0
	pb.EndNotified = false // Reset end notification so user can be notified again
	// Active state is preserved - user can still be paused/playing
}

// Play starts playback. When playback starts from inactive state, timing resets
// and playback will begin from the first tick after calling Play().
func (pb *Playback) Play() {
	pb.Active = true
	// Reset timing to state as-if we're starting fresh
	pb.LastCPUTick = -1        // Special value to indicate "not yet started"
	// Don't reset Pos/TapeTick/NextTick - tape position preserves
}

// Pause pauses playback.
func (pb *Playback) Pause() { pb.Active = false }

// IsPlaying returns true if tape is actively playing.
func (pb *Playback) IsPlaying() bool { return pb.Active }

// Ended returns true if tape has finished and auto-stopped.
func (pb *Playback) Ended() bool {
	return pb.Pos >= len(pb.Tape.Pulses) && pb.Tape != nil && len(pb.Tape.Pulses) > 0
}

// IsDone returns true if playback has reached the end.
func (pb *Playback) IsDone() bool {
	return pb.Tape != nil && pb.Pos >= len(pb.Tape.Pulses)
}

// PercentComplete returns playback completion percentage (0-100).
func (pb *Playback) PercentComplete() float64 {
	if pb.Tape == nil || len(pb.Tape.Pulses) == 0 {
		return 0
	}
	return float64(pb.Pos) / float64(len(pb.Tape.Pulses)) * 100.0
}

// ---- TAP format constants ----

// ZX Spectrum tape timing constants (in T-states at 3.5 MHz).
// These match the ROM loader timing exactly - verified against zxcpp working implementation.
const (
	// Leader tone: 2168 T-states per pulse (806 Hz)
	TapLeaderPulse = 2168

	// Sync pulses: 667 and 735 T-states
	TapSyncPulse1 = 667
	TapSyncPulse2 = 735

	// Data bit pulses
	TapBit0Pulse = 855 // ~4115 Hz (both pulses)
	TapBit1Pulse = 1710 // ~2057 Hz (both pulses)

	// Leader length from zxcpp (critical for ROM timing synchronization):
	// - 3000 pulses for header blocks (not 8063 - that was wrong!)
	// - 3223 pulses for data blocks
	// These values were verified by comparing with the working zxcpp implementation.
	TapLeaderLengthHeader = 3000
	TapLeaderLengthData   = 3223

	// Final sync pulse after data: 945 T-states (high level)
	// Required by ROM to properly detect end of data block
	TapFinalSyncPulse = 945

	// Pause after block: 1 second = 3,500,000 T-states
	TapPauseTicks = 3500000
)

// LoadTAP reads a TAP file and returns a Tape with pulse stream and blocks.
func LoadTAP(r io.Reader, filename string) (*Tape, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read TAP file: %w", err)
	}

	var pulses []Pulse
	var blocks []Block

	// Parse all blocks from the TAP file
	for offset := 0; offset < len(data); {
		if offset+2 > len(data) {
			break
		}

		// Block length: 2 bytes, little-endian
		blockLen := int(data[offset]) | int(data[offset+1])<<8
		offset += 2

		if blockLen == 0 || offset+blockLen > len(data) {
			break
		}

		blockData := data[offset : offset+blockLen]
		offset += blockLen

		// Decode block
		flag := uint8(0)
		if len(blockData) > 0 {
			flag = blockData[0]
		}

		blockType := uint8(0)
		// In a header block (flag==0x00), block type is at byte 17:
		// flag(1) + filename(10) + len_low(1) + len_high(1) + addr_low(1) + addr_high(1) + param2_low(1) + param2_high(1) + blocktype(1)
		if flag == 0x00 && len(blockData) > 17 {
			blockType = blockData[17]
		}

		blocks = append(blocks, Block{
			Data:      append([]byte(nil), blockData...),
			Flag:      flag,
			BlockType: blockType,
		})

		// Generate pulse stream for this block
		pulses = appendBlockPulses(pulses, blockData, flag == 0x00)
	}

	return &Tape{
		FileName: filename,
		Format:   "TAP",
		Pulses:   pulses,
		Blocks:   blocks,
	}, nil
}

// appendBlockPulses generates the standard ROM loader pulse sequence for a block.
// isHeader determines leader length (3000 for header, 3223 for data).
// Critical timing fixes from zxcpp comparison:
// 1. Header leader is 3000 pulses, not 8063 (was causing ROM desync)
// 2. Bits are sent MSB-first, not LSB-first (was causing data corruption)
// 3. Final sync pulse of 945 T-states after data (was missing)
func appendBlockPulses(pulses []Pulse, data []byte, isHeader bool) []Pulse {
	// Leader tone: alternating pulses at 806 Hz
	leaderLen := TapLeaderLengthData
	if isHeader {
		leaderLen = TapLeaderLengthHeader
	}
	for i := 0; i < leaderLen; i++ {
		pulses = append(pulses, Pulse{Level: true, Duration: TapLeaderPulse})
		pulses = append(pulses, Pulse{Level: false, Duration: TapLeaderPulse})
	}

	// Sync pulse: one short high, one short low
	pulses = append(pulses, Pulse{Level: true, Duration: TapSyncPulse1})
	pulses = append(pulses, Pulse{Level: false, Duration: TapSyncPulse2})

	// Data bytes: each bit as two pulses (MSB first - matching zxcpp working implementation)
	for _, b := range data {
		// Each byte is sent MSB first (bit 7 down to 0)
		// This was the critical bug - LSB-first was causing byte corruption
		for bit := 7; bit >= 0; bit-- {
			if (b & (1 << bit)) != 0 {
				// Bit 1: two pulses of 1710 T-states
				pulses = append(pulses, Pulse{Level: true, Duration: TapBit1Pulse})
				pulses = append(pulses, Pulse{Level: false, Duration: TapBit1Pulse})
			} else {
				// Bit 0: two pulses of 855 T-states
				pulses = append(pulses, Pulse{Level: true, Duration: TapBit0Pulse})
				pulses = append(pulses, Pulse{Level: false, Duration: TapBit0Pulse})
			}
		}
	}

	// Final sync pulse after data (from zxcpp working implementation)
	// This pulse is critical for ROM to detect end of data block properly
	pulses = append(pulses, Pulse{Level: true, Duration: TapFinalSyncPulse})

	// Pause after block (EAR high / silent)
	pulses = append(pulses, Pulse{Level: true, Duration: TapPauseTicks})

	return pulses
}
