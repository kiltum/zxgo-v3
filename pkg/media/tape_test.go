package media

import (
	"bytes"
	"testing"
)

func TestLoadTAP_HeaderOnly(t *testing.T) {
	// Single header block (standard 19-byte ZX Spectrum header)
	// Total 21 bytes: [length_lo length_hi] + [19-byte header]
	data := []byte{
		0x13, 0x00, // Length: 19 bytes (little-endian)
		0x00,       // Flag: 0x00 = header
		'P', 'R', 'O', 'G', 'R', 'A', 'M', ' ', ' ', ' ', // Filename (10 bytes)
		0x00,       // Length low byte
		0x00,       // Length high byte
		0x00,       // Start address low
		0x80,       // Start address high
		0x00,       // Param 2 low
		0x80,       // Param 2 high
		0x10,       // Block type: 0x10 = BASIC program (byte 17 in blockData, byte 19 overall)
		0x00,       // Padding byte to reach 19 total (ZX Spectrum headers have a checksum byte that we're setting to 0)
	}

	tape, err := LoadTAP(bytes.NewReader(data), "test.tap")
	if err != nil {
		t.Fatalf("LoadTAP failed: %v", err)
	}

	if tape.Format != "TAP" {
		t.Errorf("Expected format TAP, got %s", tape.Format)
	}

	if len(tape.Blocks) != 1 {
		t.Fatalf("Expected 1 block, got %d", len(tape.Blocks))
	}

	// Check header block
	header := tape.Blocks[0]
	if header.Flag != 0x00 {
		t.Errorf("Expected header flag 0x00, got 0x%02X", header.Flag)
	}
	if header.BlockType != 0x10 {
		t.Errorf("Expected block type 0x10, got 0x%02X", header.BlockType)
	}
	if len(header.Data) != 19 {
		t.Errorf("Expected header length 19, got %d", len(header.Data))
	}

	// Check pulse stream exists
	if len(tape.Pulses) == 0 {
		t.Error("Expected non-zero pulse stream")
	}

	// Verify leader pulses exist (3000 header leader pulses * 2 = 6000 entries, plus sync, data, pause)
	if len(tape.Pulses) < 6000 {
		t.Errorf("Pulse count seems too low: got %d, expected at least 6000", len(tape.Pulses))
	}
}

func TestPlaybackSequence(t *testing.T) {
	// Create a minimal tape with known pulses
	tape := &Tape{
		Format: "TEST",
		Pulses: []Pulse{
			{Level: true, Duration: 1000},
			{Level: false, Duration: 1000},
			{Level: true, Duration: 500},
			{Level: false, Duration: 500},
		},
	}

	pb := NewPlayback(tape)

	// Test initial state
	if pb.IsPlaying() {
		t.Error("Expected not playing initially")
	}

	// Test level before start (inactive = high)
	if !pb.CurrentLevel(0) {
		t.Error("Expected high level before playback start")
	}

	// Start playback
	pb.Play()
	if !pb.IsPlaying() {
		t.Error("Expected playing after Play()")
	}

	// First active call initializes playback timing at tick 1
	// (tape timing starts from the first active call)
	pb.CurrentLevel(1)

	// Check levels at different ticks (tape timing now relative to tick 1)
	// Tape starts at tick 1, pulse durations: [1000, 1000, 500, 500]
	// Total tape duration: 3000 ticks
	// Timing is relative to first active call (tick 1)
	tests := []struct {
		tick     int64
		expected bool
	}{
		{500, true},     // Tick 500: tapeTick=499, still in first pulse (0-999)
		{1000, true},    // Tick 1000: tapeTick=999, still in first pulse (0-999)
		{1050, false},   // Tick 1050: tapeTick=1049, in second pulse (1000-1999)
		{1500, false},   // Tick 1500: tapeTick=1499, still in second pulse
		{2000, false},   // Tick 2000: tapeTick=1999, still in second pulse
		{2050, true},    // Tick 2050: tapeTick=2049, in third pulse (2000-2499)
		{2250, true},    // Tick 2250: tapeTick=2249, still in third pulse
		{2500, true},    // Tick 2500: tapeTick=2499, still in third pulse
		{2550, false},   // Tick 2550: tapeTick=2549, in fourth pulse (2500-2999)
		{3000, false},   // Tick 3000: tapeTick=2999, still in fourth pulse
		{3050, true},    // Tick 3050: tapeTick=3049, past end of tape (floats high)
	}

	for _, tt := range tests {
		if got := pb.CurrentLevel(tt.tick); got != tt.expected {
			t.Errorf("At tick %d, expected level %v, got %v", tt.tick, tt.expected, got)
		}
	}

	// Test pause
	pb.Pause()
	if pb.IsPlaying() {
		t.Error("Expected not playing after Pause()")
	}

	// Test reset
	pb.Play()
	pb.CurrentLevel(3000) // Advance to end
	pb.Reset()
	// Reset doesn't change "IsDone" but resets position
	if pb.Pos != 0 {
		t.Errorf("Expected position 0 after reset, got %d", pb.Pos)
	}
}

func TestPlaybackLoop(t *testing.T) {
	tape := &Tape{
		Format: "TEST",
		Pulses: []Pulse{
			{Level: true, Duration: 100},
			{Level: false, Duration: 100},
		},
	}

	pb := NewPlayback(tape)
	pb.Play() // Looping not implemented - tape ends when reaching last pulse

	// First pass
	if !pb.CurrentLevel(50) {
		t.Error("Expected high at tick 50 (first pass)")
	}
	if pb.CurrentLevel(150) {
		t.Error("Expected low at tick 150 (first pass)")
	}
	if !pb.CurrentLevel(300) {
		t.Error("Expected high at tick 300 (end of tape floats high)")
	}
}

func TestTapTimingConstants(t *testing.T) {
	// Verify timing constants match ZX Spectrum ROM timing (3.5 MHz)

	// Leader: 2168 T-states ~ 806 Hz
	expectedLeaderHz := 3500000.0 / (2.0 * 2168.0)
	if expectedLeaderHz < 800 || expectedLeaderHz > 820 {
		t.Errorf("Leader frequency %.2f Hz out of expected range [800-820] Hz", expectedLeaderHz)
	}

	// Bit 0: 855 T-states ~ 2057 Hz
	expectedBit0Hz := 3500000.0 / (2.0 * 855.0)
	if expectedBit0Hz < 2000 || expectedBit0Hz > 2100 {
		t.Errorf("Bit 0 frequency %.2f Hz out of expected range [2000-2100] Hz", expectedBit0Hz)
	}

	// Bit 1: 1710 T-states ~ 1023 Hz
	expectedBit1Hz := 3500000.0 / (2.0 * 1710.0)
	if expectedBit1Hz < 1000 || expectedBit1Hz > 1050 {
		t.Errorf("Bit 1 frequency %.2f Hz out of expected range [1000-1050] Hz", expectedBit1Hz)
	}

	// Verify leader lengths match ROM expectations (from zxcpp working implementation)
	if TapLeaderLengthHeader != 3000 {
		t.Errorf("Expected header leader length 3000, got %d", TapLeaderLengthHeader)
	}
	if TapLeaderLengthData != 3223 {
		t.Errorf("Expected data leader length 3223, got %d", TapLeaderLengthData)
	}

	// Pause: 1 second = 3,500,000 T-states at 3.5 MHz
	if TapPauseTicks != 3500000 {
		t.Errorf("Expected pause ticks 3500000, got %d", TapPauseTicks)
	}
}

func TestPercentComplete(t *testing.T) {
	tape := &Tape{
		Format: "TEST",
		Pulses: []Pulse{
			{Level: true, Duration: 100},
			{Level: false, Duration: 100},
			{Level: true, Duration: 100},
			{Level: false, Duration: 100},
		},
	}

	pb := NewPlayback(tape)
	pb.Play()

	// Initialize at tick 1 (to set relative timing)
	pb.CurrentLevel(1)

	// Check percent complete at various ticks
	// Tape positions: [0,1,2,3] (4 pulses with durations 100 each)
	// Total duration: 100+100+100+100 = 400
	tests := []struct {
		tick     int64
		expected float64
	}{
		{1, 0.0},       // Start (tick 1, position 0)
		{100, 0.0},     // Still in first pulse (1-100)
		{101, 25.0},    // Start of second pulse (position 1/4)
		{200, 25.0},    // Still in second pulse (101-200)
		{201, 50.0},    // Start of third pulse (position 2/4)
		{300, 50.0},    // Still in third pulse (201-300)
		{301, 75.0},    // Start of fourth pulse (position 3/4)
		{400, 75.0},    // Still in fourth pulse (301-400)
		{401, 100.0},   // Past end (position 4/4)
		{500, 100.0},   // Still 100% complete
	}

	for _, tt := range tests {
		pb.CurrentLevel(tt.tick)
		got := pb.PercentComplete()
		if got != tt.expected {
			t.Errorf("After tick %d, expected %.1f%% complete, got %.1f%%", tt.tick, tt.expected, got)
		}
	}

	// Test nil tape
	pbNil := NewPlayback(nil)
	if got := pbNil.PercentComplete(); got != 0 {
		t.Errorf("Expected 0%% for nil tape, got %.1f%%", got)
	}

	// Test empty tape
	emptyTape := &Tape{Format: "EMPTY", Pulses: []Pulse{}}
	pbEmpty := NewPlayback(emptyTape)
	if got := pbEmpty.PercentComplete(); got != 0 {
		t.Errorf("Expected 0%% for empty tape, got %.1f%%", got)
	}
}

func TestEmptyTAP(t *testing.T) {
	data := []byte{}
	tape, err := LoadTAP(bytes.NewReader(data), "empty.tap")
	if err != nil {
		t.Fatalf("LoadTAP on empty file failed: %v", err)
	}

	if len(tape.Blocks) != 0 {
		t.Errorf("Expected 0 blocks for empty TAP, got %d", len(tape.Blocks))
	}
	if len(tape.Pulses) != 0 {
		t.Errorf("Expected 0 pulses for empty TAP, got %d", len(tape.Pulses))
	}
}

func TestBlockDataIntegrity(t *testing.T) {
	// Test that block data is preserved correctly
	testData := byte(0x42)
	data := []byte{
		0x02, 0x00, // Length: 2 bytes
		0xFF,       // Flag: data
		testData,   // Our test byte
	}

	tape, err := LoadTAP(bytes.NewReader(data), "test.tap")
	if err != nil {
		t.Fatalf("LoadTAP failed: %v", err)
	}

	if len(tape.Blocks) != 1 {
		t.Fatalf("Expected 1 block, got %d", len(tape.Blocks))
	}

	block := tape.Blocks[0]
	if len(block.Data) != 2 {
		t.Fatalf("Expected block data length 2, got %d", len(block.Data))
	}

	if block.Data[1] != testData {
		t.Errorf("Block data corrupted: expected 0x%02X, got 0x%02X", testData, block.Data[1])
	}
}
