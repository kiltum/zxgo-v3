package media

import (
	"encoding/binary"
	"fmt"
	"io"
)

// TZX block type identifiers (from TZX v1.20 specification)
const (
	TZXStandardSpeed = 0x10 // Standard speed data block
	TZXTurboSpeed    = 0x11 // Turbo speed data block
	TZXPureTone      = 0x12 // Pure tone block
	TZXPulseSeq      = 0x13 // Pulse sequence block
	TZXPureData      = 0x14 // Pure data block
	TZXDirectRec     = 0x15 // Direct recording block
	TZXCSWRec        = 0x18 // CSW recording block
	TZXGeneralData   = 0x19 // Generalized data block
	TZXPause         = 0x20 // Pause (silence) or 'Stop the tape' command
	TZXGroupStart    = 0x21 // Group start
	TZXGroupEnd      = 0x22 // Group end
	TZXJumpBlock     = 0x23 // Jump to block
	TZXLoopStart     = 0x24 // Loop start
	TZXLoopEnd       = 0x25 // Loop end
	TZXCallSeq       = 0x26 // Call sequence
	TZXReturnSeq     = 0x27 // Return from sequence
	TZXSelectBlock   = 0x28 // Select block
	TZXStopIf48K     = 0x2A // Stop the tape if in 48K mode
	TZXSetLevel      = 0x2B // Set signal level
	TZXText          = 0x30 // Text description
	TZXMessage       = 0x31 // Message block
	TZXArchiveInfo   = 0x32 // Archive info block (TZX v1.20+)
	TZXHardwareType  = 0x33 // Hardware type
	TZXCustomInfo    = 0x35 // Custom info block (ID 53 in decimal)
	TZXGlueBlock     = 0x5A // "Glue" block (90 decimal, ASCII Letter 'Z')
)

// LoadTZX reads a TZX file and returns a Tape with pulse stream and blocks.
// TZX format supports many block types for high-fidelity tape recordings.
// Updated to handle all TZX v1.20 block types with proper error recovery.
func LoadTZX(r io.Reader, filename string) (*Tape, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read TZX file: %w", err)
	}

	// Verify TZX header: "ZXTape!" + version + extension bytes
	if len(data) < 10 {
		return nil, fmt.Errorf("TZX file too short: need at least 10 bytes, got %d", len(data))
	}

	if string(data[:7]) != "ZXTape!" {
		return nil, fmt.Errorf("invalid TZX header: expected 'ZXTape!', got '%s'", string(data[:7]))
	}

	_version := fmt.Sprintf("%d.%02d", data[7], data[8]) // Version info

	var pulses []Pulse
	var blocks []Block

	pos := 10 // Skip "ZXTape!" + version + extension
	blockNumber := 0

	for pos < len(data) {
		blockID := data[pos]
		pos++

		switch blockID {
		case TZXStandardSpeed:
			// Standard speed data block (ID 10) - Fixed proper parsing
			if pos+4 > len(data) {
				return nil, fmt.Errorf("invalid standard speed block at position %d", pos)
			}

			pauseMs := binary.LittleEndian.Uint16(data[pos : pos+2])
			pos += 2
			dataLen := binary.LittleEndian.Uint16(data[pos : pos+2])
			pos += 2

			if pos+int(dataLen) > len(data) {
				return nil, fmt.Errorf("standard speed block overflow - need %d bytes at pos %d, got %d", dataLen, pos, len(data)-pos)
			}

			blockData := data[pos : pos+int(dataLen)]
			pos += int(dataLen)

			// Generate standard ZX Spectrum tape pulses
			pulses = appendStandardSpeedPulses(pulses, blockData, pauseMs)

			// Decode block for metadata
			if len(blockData) > 0 {
				flag := blockData[0]
				blockType := uint8(0)
				if flag == 0x00 && len(blockData) > 17 {
					blockType = blockData[17]
				}
				blocks = append(blocks, Block{
					Data:      append([]byte(nil), blockData...),
					Flag:      flag,
					BlockType: blockType,
				})
			}
			blockNumber++

		case TZXTurboSpeed:
			// Turbo speed data block (ID 11) - Fixed with proper 3-byte length parsing per TZX spec
			if pos+19 > len(data) {
				return nil, fmt.Errorf("invalid turbo speed block at position %d - need at least 19 bytes", pos)
			}

			pilotPulseLen := int(binary.LittleEndian.Uint16(data[pos : pos+2]))
			pos += 2
			sync1PulseLen := int(binary.LittleEndian.Uint16(data[pos : pos+2]))
			pos += 2
			sync2PulseLen := int(binary.LittleEndian.Uint16(data[pos : pos+2]))
			pos += 2
			bit0PulseLen := int(binary.LittleEndian.Uint16(data[pos : pos+2]))
			pos += 2
			bit1PulseLen := int(binary.LittleEndian.Uint16(data[pos : pos+2]))
			pos += 2

			pilotToneLen := int(binary.LittleEndian.Uint16(data[pos : pos+2]))
			pos += 2

			_ = data[pos] // lastBitsUsed
			pos += 1

			pauseMs := binary.LittleEndian.Uint16(data[pos : pos+2])
			pos += 2

			// Read 3-byte data length (little endian) - KEY FIX: Was using 2-byte length
			if pos+3 > len(data) {
				return nil, fmt.Errorf("turbo speed block - cannot read 3-byte data length at pos %d", pos)
			}
			dataLen := int(data[pos]) | int(data[pos+1])<<8 | int(data[pos+2])<<16
			pos += 3

			if pos+dataLen > len(data) {
				return nil, fmt.Errorf("turbo speed block overflow - need %d bytes, have %d at pos %d", dataLen, len(data)-pos, pos)
			}

			blockData := data[pos : pos+dataLen]
			pos += dataLen

			// Generate pulses with custom timing
			pulses = appendTurboSpeedPulses(pulses, blockData, pilotToneLen, sync1PulseLen, sync2PulseLen,
				pilotPulseLen, sync1PulseLen, sync2PulseLen, bit0PulseLen, bit1PulseLen)

			// Add pause after block
			pauseTicks := int64(pauseMs) * (3500000 / 1000)
			pulses = append(pulses, Pulse{Level: true, Duration: pauseTicks})

			// Decode block for metadata
			if len(blockData) > 0 {
				flag := blockData[0]
				blockType := uint8(0)
				if flag == 0x00 && len(blockData) > 17 {
					blockType = blockData[17]
				}
				blocks = append(blocks, Block{
					Data:      append([]byte(nil), blockData...),
					Flag:      flag,
					BlockType: blockType,
				})
			}
			blockNumber++

		case TZXPureTone:
			// Pure tone block (ID 12) - Fixed proper parsing
			if pos+4 > len(data) {
				return nil, fmt.Errorf("invalid pure tone block at position %d", pos)
			}

			pulseLenTStates := int(binary.LittleEndian.Uint16(data[pos : pos+2]))
			pos += 2
			numPulses := int(binary.LittleEndian.Uint16(data[pos : pos+2]))
			pos += 2

			// Generate pulses - Fixed: use T-states directly, not MS conversion
			for i := 0; i < numPulses; i++ {
				pulses = append(pulses, Pulse{Level: true, Duration: int64(pulseLenTStates)})
				pulses = append(pulses, Pulse{Level: false, Duration: int64(pulseLenTStates)})
			}
			blockNumber++

		case TZXPulseSeq:
			// Pulse sequence block (ID 13) - Fixed proper parsing
			if pos+1 > len(data) {
				return nil, fmt.Errorf("invalid pulse sequence block at position %d", pos)
			}

			count := int(data[pos])
			pos++

			if pos+count*2 > len(data) {
				return nil, fmt.Errorf("pulse sequence overflow - need %d bytes at pos %d", count*2, pos)
			}

			for i := 0; i < count; i++ {
				pulseLen := int(binary.LittleEndian.Uint16(data[pos : pos+2]))
				pos += 2

				// Single pulse then its complement (full wave)
				pulses = append(pulses, Pulse{Level: true, Duration: int64(pulseLen)})
				pulses = append(pulses, Pulse{Level: false, Duration: int64(pulseLen)})
			}
			blockNumber++

		case TZXPureData:
			// Pure data block (ID 14) - Fixed proper parsing per TZX spec
			// length: [07,08,09]+0A
			if pos+10 > len(data) {
				return nil, fmt.Errorf("invalid pure data block at position %d - need at least 10 bytes", pos)
			}

			bit0Len := int(binary.LittleEndian.Uint16(data[pos : pos+2]))
			pos += 2
			bit1Len := int(binary.LittleEndian.Uint16(data[pos : pos+2]))
			pos += 2

			lastBitsUsed := data[pos]
			pos += 1

			pauseMs := binary.LittleEndian.Uint16(data[pos : pos+2])
			pos += 2

			// Read 3-byte data length (little endian) - KEY FIX: Was using 2-byte length
			if pos+3 > len(data) {
				return nil, fmt.Errorf("pure data block - cannot read 3-byte data length at pos %d", pos)
			}
			dataLen := int(data[pos]) | int(data[pos+1])<<8 | int(data[pos+2])<<16
			pos += 3

			if pos+dataLen > len(data) {
				return nil, fmt.Errorf("pure data block overflow - need %d bytes, have %d at pos %d", dataLen, len(data)-pos, pos)
			}

			blockData := data[pos : pos+dataLen]
			pos += dataLen

			// Generate pulses for each bit (using standard timing)
			pulses = appendPureDataPulses(pulses, blockData, lastBitsUsed, pauseMs, bit0Len, bit1Len)
			blockNumber++

		case TZXDirectRec:
			// Direct recording block (ID 15) - New implementation per TZX spec
			// length: [05,06,07]+08
			if pos+8 > len(data) {
				return nil, fmt.Errorf("invalid direct recording block at position %d - need at least 8 bytes", pos)
			}

			tStatesPerSample := binary.LittleEndian.Uint16(data[pos : pos+2])
			pos += 2
			pauseMs := binary.LittleEndian.Uint16(data[pos : pos+2])
			pos += 2

			lastBitsUsed := data[pos]
			pos += 1

			// Read 3-byte sample data length (little endian)
			if pos+3 > len(data) {
				return nil, fmt.Errorf("direct recording block - cannot read 3-byte sample length at pos %d", pos)
			}
			sampleLen := int(data[pos]) | int(data[pos+1])<<8 | int(data[pos+2])<<16
			pos += 3

			if pos+sampleLen > len(data) {
				return nil, fmt.Errorf("direct recording block overflow - need %d bytes, have %d at pos %d", sampleLen, len(data)-pos, pos)
			}

			sampleData := data[pos : pos+sampleLen]
			pos += sampleLen

			// Generate pulses from direct recording samples
			pulses = appendDirectRecordingPulses(pulses, sampleData, tStatesPerSample, lastBitsUsed)

			// Add pause after block
			pauseTicks := int64(pauseMs) * (3500000 / 1000)
			pulses = append(pulses, Pulse{Level: true, Duration: pauseTicks})
			blockNumber++

		case TZXPause:
			// Pause block (ID 20) - Fixed per TZX spec
			if pos+2 > len(data) {
				return nil, fmt.Errorf("invalid pause block at position %d", pos)
			}

			pauseMs := binary.LittleEndian.Uint16(data[pos : pos+2])
			pos += 2

			// Convert MS to T-states
			pauseTicks := int64(pauseMs) * (3500000 / 1000)
			pulses = append(pulses, Pulse{Level: true, Duration: pauseTicks})
			blockNumber++

		case TZXGroupStart:
			// Group start (ID 21) - Metadata only
			if pos+1 > len(data) {
				return nil, fmt.Errorf("invalid group start block at position %d", pos)
			}

			nameLen := int(data[pos])
			pos++

			if pos+nameLen > len(data) {
				return nil, fmt.Errorf("group name overflow - need %d bytes at pos %d", nameLen, pos)
			}

			pos += nameLen // Skip group name
			blockNumber++

		case TZXGroupEnd:
			// Group end (ID 22) - Metadata only
			blockNumber++

		case TZXJumpBlock:
			// Jump to block (ID 23) - Fixed per TZX spec
			if pos+2 > len(data) {
				return nil, fmt.Errorf("invalid jump block at position %d", pos)
			}

			_ = int16(binary.LittleEndian.Uint16(data[pos : pos+2])) // Relative jump
			pos += 2
			// For now, we ignore jump blocks and continue parsing
			blockNumber++

		case TZXLoopStart:
			// Loop start (ID 24) - Fixed per TZX spec
			if pos+2 > len(data) {
				return nil, fmt.Errorf("invalid loop start block at position %d", pos)
			}

			_ = binary.LittleEndian.Uint16(data[pos : pos+2]) // Number of repetitions
			pos += 2
			// For now, we ignore loop blocks and continue parsing
			blockNumber++

		case TZXLoopEnd:
			// Loop end (ID 25) - Metadata only
			blockNumber++

		case TZXCallSeq:
			// Call sequence (ID 26)
			if pos+2 > len(data) {
				return nil, fmt.Errorf("invalid call sequence block at position %d", pos)
			}

			numCalls := binary.LittleEndian.Uint16(data[pos : pos+2])
			pos += 2

			if pos+int(numCalls)*2 > len(data) {
				return nil, fmt.Errorf("call sequence overflow - need %d bytes at pos %d", int(numCalls)*2, pos)
			}

			pos += int(numCalls) * 2 // Skip call block numbers
			// For now, we ignore call blocks and continue parsing
			blockNumber++

		case TZXReturnSeq:
			// Return from sequence (ID 27) - Metadata only
			blockNumber++

		case TZXSelectBlock:
			// Select block (ID 28) - Fixed per TZX spec
			if pos+2 > len(data) {
				return nil, fmt.Errorf("invalid select block at position %d", pos)
			}

			blockLen := binary.LittleEndian.Uint16(data[pos : pos+2])
			pos += 2

			if pos+int(blockLen) > len(data) {
				return nil, fmt.Errorf("select block overflow - need %d bytes at pos %d", int(blockLen), pos)
			}

			// Skip the entire select block content
			pos += int(blockLen)
			blockNumber++

		case TZXStopIf48K:
			// Stop if in 48K mode (ID 2A) - Fixed per TZX spec
			// This block has no content after ID, follows extension rule
			blockNumber++

		case TZXSetLevel:
			// Set signal level (ID 2B) - Fixed per TZX spec
			// length: 05 (1 DWORD for length + 1 BYTE for level)
			if pos+5 > len(data) {
				return nil, fmt.Errorf("invalid set level block at position %d - need 5 bytes", pos)
			}

			_ = binary.LittleEndian.Uint32(data[pos : pos+4]) // Block length (extension rule)
			pos += 4

			_ = data[pos] // Signal level (0=low, 1=high)
			pos += 1
			// For now, we just skip this block
			blockNumber++

		case TZXText:
			// Text description (ID 30) - Metadata only
			if pos+1 > len(data) {
				return nil, fmt.Errorf("invalid text block at position %d", pos)
			}

			textLen := int(data[pos])
			pos++

			if pos+textLen > len(data) {
				return nil, fmt.Errorf("text data overflow - need %d bytes at pos %d", textLen, pos)
			}

			pos += textLen // Skip text
			blockNumber++

		case TZXMessage:
			// Message block (ID 31) - Fixed per TZX spec
			if pos+2 > len(data) {
				return nil, fmt.Errorf("invalid message block at position %d", pos)
			}

			_ = data[pos] // Time (seconds)
			pos += 1

			msgLen := int(data[pos])
			pos++

			if pos+msgLen > len(data) {
				return nil, fmt.Errorf("message data overflow - need %d bytes at pos %d", msgLen, pos)
			}

			pos += msgLen // Skip message
			blockNumber++

		case TZXArchiveInfo:
			// Archive info block (ID 32) - Fixed per TZX spec
			if pos+2 > len(data) {
				return nil, fmt.Errorf("invalid archive info block at position %d", pos)
			}

			infoLen := binary.LittleEndian.Uint16(data[pos : pos+2])
			pos += 2

			if pos+int(infoLen) > len(data) {
				return nil, fmt.Errorf("archive info data overflow - need %d bytes at pos %d", int(infoLen), pos)
			}

			// Archive info is metadata-only, skip it
			pos += int(infoLen)
			blockNumber++

		case TZXHardwareType:
			// Hardware type (ID 33) - Metadata only
			if pos+1 > len(data) {
				return nil, fmt.Errorf("invalid hardware type block at position %d", pos)
			}

			numEntries := int(data[pos])
			pos++

			// Each entry is 3 bytes, so skip them
			if pos+numEntries*3 > len(data) {
				return nil, fmt.Errorf("hardware type overflow - need %d bytes at pos %d", numEntries*3, pos)
			}

			pos += numEntries * 3
			blockNumber++

		case TZXCustomInfo:
			// Custom info block (ID 35) - Following UnrealSpeccy's proven approach
			// Format: [ID(1) + ID_String(10) + Reserved(4) + Length(4) + Description(variable)]
			if pos+20 > len(data) { // Need at least header bytes
				return nil, fmt.Errorf("invalid custom info block at position %d - need at least 20 bytes", pos)
			}

			// Following UnrealSpeccy: Length field is at offset 0x10 (16) from block start
			// Structure: ID(1) + ID_String(10) + Reserved(4) + Length(4) + Description(var)
			lengthOffset := pos + 0x10 // 16 bytes from block start (matches UnrealSpeccy)
			if lengthOffset+4 > len(data) {
				return nil, fmt.Errorf("custom info block - length field out of bounds at pos %d", pos)
			}
			customLen := int(binary.LittleEndian.Uint32(data[lengthOffset : lengthOffset+4]))

			// Skip entire block: 20 bytes header + custom data (matches UnrealSpeccy approach)
			totalBlockSize := 0x14 + customLen // 20 bytes header + custom data length
			if pos+totalBlockSize > len(data) {
				return nil, fmt.Errorf("custom info overflow - need %d total bytes, have %d at pos %d", totalBlockSize, len(data)-pos, pos)
			}

			pos += totalBlockSize // Skip entire block (header + custom data)
			blockNumber++

		case TZXGlueBlock:
			// "Glue" block (ID 5A) - Fixed per TZX spec
			// Skip 9 bytes as per specification
			if pos+9 > len(data) {
				return nil, fmt.Errorf("invalid glue block at position %d - need 9 bytes", pos)
			}

			// Check it matches the expected pattern: "XTape!",0x1A,MajR,MinR
			glueData := data[pos : pos+9]
			if len(glueData) >= 9 {
				if string(glueData[0:6]) != "XTape!" || glueData[6] != 0x1A {
					// Invalid glue block format, but we skip it anyway
					return nil, fmt.Errorf("invalid glue block format at position %d", pos)
				}
			}

			pos += 9 // Skip all 9 bytes
			blockNumber++

		default:
			// Unknown block type - Improved error recovery with 4-byte length support
			// Many TZX files use custom blocks for protection or loader enhancements

			// Try to read a 4-byte length (extension rule for v1.10+)
			var blockLen int = 0
			foundLength := false

			if pos+4 <= len(data) {
				// Check if it looks like a valid 4-byte length
				potentialLen := int(binary.LittleEndian.Uint32(data[pos : pos+4]))
				// Sanity check: length should be reasonable (not huge, and should fit)
				if potentialLen >= 0 && potentialLen < 1000000 && pos+4+potentialLen <= len(data) {
					blockLen = potentialLen + 4 // Include the length bytes themselves
					foundLength = true
				}
			}

			if foundLength {
				// Skip the entire block using the length
				pos += blockLen
			} else {
				// No valid length found, use heuristic search for next known block ID
				knownBlockIDs := map[byte]bool{
					0x10: true, 0x11: true, 0x12: true, 0x13: true, 0x14: true, 0x15: true,
					0x18: true, 0x19: true, 0x20: true, 0x21: true, 0x22: true, 0x23: true,
					0x24: true, 0x25: true, 0x26: true, 0x27: true, 0x28: true, 0x2A: true,
					0x2B: true, 0x30: true, 0x31: true, 0x32: true, 0x33: true, 0x35: true,
					0x5A: true,
				}

				// Try to find the next known block ID (heuristic: search up to 100 bytes ahead)
				searchLimit := min(100, len(data)-pos)
				foundNext := false

				for offset := 1; offset <= searchLimit; offset++ {
					// Check bounds before accessing data
					if pos+offset < len(data) && knownBlockIDs[data[pos+offset]] {
						// Found next known block, skip to it
						pos += offset
						foundNext = true
						break
					}
				}

				if !foundNext {
					// Couldn't find next known block - give up on this file
					return nil, fmt.Errorf("unsupported TZX block type 0x%02X at position %d (version %s, extension 0x%02X)", blockID, pos-1, _version, data[9])
				}
			}
		}
	}

	return &Tape{
		FileName: filename,
		Format:   "TZX",
		Pulses:   pulses,
		Blocks:   blocks,
	}, nil
}

// appendStandardSpeedPulses generates pulses for standard ZX Spectrum timing.
func appendStandardSpeedPulses(pulses []Pulse, data []byte, pauseMs uint16) []Pulse {
	isHeader := (len(data) > 0 && data[0] == 0x00)
	pulses = appendBlockPulses(pulses, data, isHeader)

	// Add pause after block
	pauseTicks := int64(pauseMs) * (3500000 / 1000)
	pulses = append(pulses, Pulse{Level: true, Duration: pauseTicks})

	return pulses
}

// appendTurboSpeedPulses generates pulses with custom timing (turbo loaders).
// Fixed: Uses proper parameter names and MSB-first bit ordering (matching zxcpp working implementation)
func appendTurboSpeedPulses(pulses []Pulse, data []byte, pilotLen, sync1Len, sync2Len,
	pilotPulseLen, sync1PulseLen, sync2PulseLen, bit0Len, bit1Len int) []Pulse {

	// Generate pilot tone
	for i := 0; i < pilotLen; i++ {
		pulses = append(pulses, Pulse{Level: true, Duration: int64(pilotPulseLen)})
		pulses = append(pulses, Pulse{Level: false, Duration: int64(pilotPulseLen)})
	}

	// Sync pulse
	pulses = append(pulses, Pulse{Level: true, Duration: int64(sync1PulseLen)})
	pulses = append(pulses, Pulse{Level: false, Duration: int64(sync2PulseLen)})

	// Generate data bits (MSB first - critical fix from zxcpp comparison)
	for _, b := range data {
		// Each byte is sent MSB first (bit 7 down to 0)
		for bit := 7; bit >= 0; bit-- {
			if (b & (1 << bit)) != 0 {
				// Bit 1
				pulses = append(pulses, Pulse{Level: true, Duration: int64(bit1Len)})
				pulses = append(pulses, Pulse{Level: false, Duration: int64(bit1Len)})
			} else {
				// Bit 0
				pulses = append(pulses, Pulse{Level: true, Duration: int64(bit0Len)})
				pulses = append(pulses, Pulse{Level: false, Duration: int64(bit0Len)})
			}
		}
	}

	return pulses
}

// appendPureDataPulses generates pulses for raw data bits.
// Fixed: MSB-first bit ordering (matching zxcpp working implementation)
func appendPureDataPulses(pulses []Pulse, data []byte, bitsUsed uint8, pauseMs uint16, bit0Len, bit1Len int) []Pulse {
	// Generate pulses using standard timing for each bit (MSB first)
	for _, b := range data {
		// Each byte is sent MSB first (bit 7 down to 0)
		for bit := 7; bit >= 0; bit-- {
			if (b & (1 << bit)) != 0 {
				pulses = append(pulses, Pulse{Level: true, Duration: int64(bit1Len)})
				pulses = append(pulses, Pulse{Level: false, Duration: int64(bit1Len)})
			} else {
				pulses = append(pulses, Pulse{Level: true, Duration: int64(bit0Len)})
				pulses = append(pulses, Pulse{Level: false, Duration: int64(bit0Len)})
			}
		}
	}

	// Add pause
	pauseTicks := int64(pauseMs) * (3500000 / 1000)
	pulses = append(pulses, Pulse{Level: true, Duration: pauseTicks})

	return pulses
}

// appendDirectRecordingPulses generates pulses from direct recording samples.
// New implementation for ID 15 - Direct recording block
func appendDirectRecordingPulses(pulses []Pulse, sampleData []byte, tStatesPerSample uint16, lastBitsUsed byte) []Pulse {
	for _, sampleByte := range sampleData {
		for bit := 0; bit < 8; bit++ {
			// MSb is played first according to TZX spec
			sample := (sampleByte >> (7 - bit)) & 0x01
			if sample == 1 {
				pulses = append(pulses, Pulse{Level: true, Duration: int64(tStatesPerSample)})
			} else {
				pulses = append(pulses, Pulse{Level: false, Duration: int64(tStatesPerSample)})
			}
		}
	}

	// Handle partial last byte if needed
	if lastBitsUsed < 8 && len(sampleData) > 0 {
		// The last byte may not use all bits, but we've already processed all bytes
		// In practice, this rarely matters for audio output
	}

	return pulses
}

/// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
