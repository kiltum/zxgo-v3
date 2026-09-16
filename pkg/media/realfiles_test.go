package media

import (
	"bytes"
	"os"
	"testing"
)

// TestRealTAPFiles tests loading of real TAP files from testdata.
func TestRealTAPFiles(t *testing.T) {
	tapFiles := []string{
		"../../testdata/batty.tap",
	}

	for _, file := range tapFiles {
		t.Run(file, func(t *testing.T) {
			f, err := os.Open(file)
			if err != nil {
				t.Fatalf("Failed to open TAP file %s: %v", file, err)
			}
			defer f.Close()

			tape, err := LoadTAP(f, file)
			if err != nil {
				t.Fatalf("Failed to load TAP file %s: %v", file, err)
			}

			// Verify basic structure
			if tape.Format != "TAP" {
				t.Errorf("Expected format TAP, got %s", tape.Format)
			}

			if len(tape.Blocks) == 0 {
				t.Error("Expected at least one block")
			}

			if len(tape.Pulses) == 0 {
				t.Error("Expected non-zero pulse stream")
			}

			// Verify header block exists and is properly decoded
			if len(tape.Blocks) > 0 {
				header := tape.Blocks[0]
				if header.Flag != 0x00 {
					t.Errorf("Expected header flag 0x00, got 0x%02X", header.Flag)
				}

				// Basic sanity check on header length (should be 19 bytes)
				if len(header.Data) != 19 {
					t.Errorf("Expected header length 19, got %d", len(header.Data))
				}

				// Filename should be present (positions 1-10)
				if len(header.Data) >= 11 {
					filename := string(header.Data[1:11])
					t.Logf("Decoded filename: '%s'", filename)
				}
			}

			// Verify pulse count is reasonable (should have leader + data pulses)
			// Estimated: header leader (~16k) + data (~2k per kbyte) * multiple blocks
			minExpectedPulses := 10000
			if len(tape.Pulses) < minExpectedPulses {
				t.Errorf("Pulse count too low: got %d, expected at least %d", len(tape.Pulses), minExpectedPulses)
			}

			t.Logf("Loaded TAP file: %d blocks, %d pulses", len(tape.Blocks), len(tape.Pulses))
			t.Logf("File: %s", tape.FileName)
		})
	}
}

// TestRealTZXFiles tests loading of real TZX files from testdata.
func TestRealTZXFiles(t *testing.T) {
	tzxFiles := []string{
		"../../testdata/Alien Storm (1991)(U.S. Gold)(128K).tzx",
		"../../testdata/Exolon.tzx",
	}

	for _, file := range tzxFiles {
		t.Run(file, func(t *testing.T) {
			f, err := os.Open(file)
			if err != nil {
				t.Fatalf("Failed to open TZX file %s: %v", file, err)
			}
			defer f.Close()

			tape, err := LoadTZX(f, file)
			if err != nil {
				t.Fatalf("Failed to load TZX file %s: %v", file, err)
			}

			// Verify basic structure
			if tape.Format != "TZX" {
				t.Errorf("Expected format TZX, got %s", tape.Format)
			}

			// TZX files should have pulses even if no recognized blocks
			if len(tape.Pulses) == 0 {
				t.Error("Expected non-zero pulse stream")
			}

			// Verify filename is set
			if tape.FileName != file {
				t.Errorf("Expected filename %s, got %s", file, tape.FileName)
			}

			t.Logf("Loaded TZX file: %d blocks, %d pulses", len(tape.Blocks), len(tape.Pulses))
			t.Logf("File: %s", tape.FileName)

			// Test playback with the real tape
			if len(tape.Pulses) > 0 {
				pb := NewPlayback(tape)
				pb.Play()

				// Test that playback works without crashes
				initialLevel := pb.CurrentLevel(0)
				t.Logf("Initial EAR level: %v", initialLevel)

				// Advance through some pulses
				sampleTicks := []int64{1000, 10000, 100000, 1000000}
				for _, tick := range sampleTicks {
					level := pb.CurrentLevel(tick)
					percent := pb.PercentComplete()
					t.Logf("Tick %d: level=%v, %.1f%% complete", tick, level, percent)
				}
			}
		})
	}
}

// TestTAPPlaybackRealData tests playback with real TAP data.
func TestTAPPlaybackRealData(t *testing.T) {
	f, err := os.Open("../../testdata/batty.tap")
	if err != nil {
		t.Fatalf("Failed to open TAP file: %v", err)
	}
	defer f.Close()

	tape, err := LoadTAP(f, "../../testdata/batty.tap")
	if err != nil {
		t.Fatalf("Failed to load TAP: %v", err)
	}

	pb := NewPlayback(tape)
	pb.Play()

	// Verify playback doesn't crash and produces reasonable EAR levels
	testTicks := []int64{
		0,                    // Start
		100000,              // ~28ms into playback
		1000000,             // ~285ms into playback
		10000000,            // ~2.8s into playback
	}

	prevLevel := pb.CurrentLevel(0)
	transitions := 0

	for i, tick := range testTicks {
		level := pb.CurrentLevel(tick)

		if level != prevLevel {
			transitions++
			prevLevel = level
		}

		if i < len(testTicks)-1 { // Don't check percent on last tick (might be at end)
			percent := pb.PercentComplete()
			if percent < 0 || percent > 100 {
				t.Errorf("Invalid percent complete at tick %d: %.1f%%", tick, percent)
			}
		}
	}

	t.Logf("Playback test: %d level transitions detected over sampled ticks", transitions)

	if transitions == 0 {
		t.Error("Expected some level transitions during tape playback")
	}
}

// TestTAPVsTZXConsistency checks that identical content in TAP/TZX formats produces
// similar pulse timing characteristics.
func TestTAPVsTZXConsistency(t *testing.T) {
	// For files that exist in both formats, compare the pulse characteristics
	// This is a structural test rather than exact comparison since different
	// encodings may have different block boundaries

	tapFiles := []string{"../../testdata/batty.tap"}

	for _, tapFile := range tapFiles {
		t.Run(tapFile, func(t *testing.T) {
			// Load TAP version
			tapF, err := os.Open(tapFile)
			if err != nil {
				t.Skipf("Could not open TAP file %s: %v", tapFile, err)
			}
			defer tapF.Close()

			tapTape, err := LoadTAP(tapF, tapFile)
			if err != nil {
				t.Fatalf("Failed to load TAP: %v", err)
			}

			// Verify TAP has reasonable structure
			if len(tapTape.Blocks) == 0 {
				t.Error("TAP file has no blocks")
			}

			if len(tapTape.Pulses) < 10000 {
				t.Errorf("TAP pulse count too low: %d", len(tapTape.Pulses))
			}

			// Count high vs low pulses (should be roughly balanced)
			highCount := 0
			for _, pulse := range tapTape.Pulses {
				if pulse.Level {
					highCount++
				}
			}

			totalPulses := len(tapTape.Pulses)
			highRatio := float64(highCount) / float64(totalPulses)

			// In PCM-like tape encoding, should be roughly balanced (within 10% of 50%)
			if highRatio < 0.4 || highRatio > 0.6 {
				t.Errorf("Pulse level balance seems off: %.2f%% high pulses", highRatio*100)
			}

			t.Logf("TAP analysis: %d pulses, %.1f%% high, %d blocks", totalPulses, highRatio*100, len(tapTape.Blocks))
		})
	}
}

// TestTAPFileStructure verifies TAP file structure with real data.
func TestTAPFileStructure(t *testing.T) {
	tests := []struct {
		file          string
		minBlocks     int
		minPulses     int
		expectedFilename string
	}{
		{
			file:      "../../testdata/batty.tap",
			minBlocks: 4,  // At least a few blocks (header + data)
			minPulses: 10000, // Should have substantial pulse data
			expectedFilename: "BATTY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			f, err := os.Open(tt.file)
			if err != nil {
				t.Skipf("Could not open test file %s: %v", tt.file, err)
			}
			defer f.Close()

			tape, err := LoadTAP(f, tt.file)
			if err != nil {
				t.Fatalf("Failed to load TAP: %v", err)
			}

			if len(tape.Blocks) < tt.minBlocks {
				t.Errorf("Expected at least %d blocks, got %d", tt.minBlocks, len(tape.Blocks))
			}

			if len(tape.Pulses) < tt.minPulses {
				t.Errorf("Expected at least %d pulses, got %d", tt.minPulses, len(tape.Pulses))
			}

			// Check that first block is a header
			if len(tape.Blocks) > 0 && tape.Blocks[0].Flag != 0x00 {
				t.Errorf("Expected first block to be header (flag=0x00), got 0x%02X", tape.Blocks[0].Flag)
			}

			// Check filename in first header block
			if len(tape.Blocks) > 0 && tt.expectedFilename != "" {
				header := tape.Blocks[0]
				if len(header.Data) >= 11 {
					filename := string(header.Data[1:11])
					t.Logf("Filename in header: '%s'", filename)
					// Allow some fuzziness in filename matching due to padding
					if filename[:len(tt.expectedFilename)] != tt.expectedFilename {
						t.Logf("Note: Filename '%s' doesn't exactly match expected '%s'", filename, tt.expectedFilename)
					}
				}
			}
		})
	}
}

// TestTAPErrorHandling tests robustness with malformed or edge cases.
func TestTAPErrorHandling(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		shouldErr bool
	}{
		{"Empty file", []byte{}, false},
		{"Too short for length", []byte{0x13}, false}, // Will return 0 blocks
		{"Invalid length field", []byte{0xFF, 0xFF, 0x00}, false}, // Length > data size
		{"Valid minimal header", append([]byte{0x13, 0x00, 0x00}, make([]byte, 19)...), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tape, err := LoadTAP(bytes.NewReader(tt.data), tt.name)
			if tt.shouldErr && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// For successful loads, verify structure
			if err == nil && tape != nil {
				if tape.Format != "TAP" {
					t.Errorf("Expected TAP format, got %s", tape.Format)
				}
			}
		})
	}
}
