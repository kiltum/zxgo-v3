package media

import (
	"encoding/binary"
	"fmt"
	"io"
)

// DSK format support for +3 DOS disk images
// DSK is a complex format with two variants: Normal and Extended
// This implementation supports both formats for +3 DOS compatibility

const (
	// DSK Headers - based on the actual DSK_specs.txt format
	normalDSKHeader   = "MV - CPCEMU Disk-File\r\n" // Standard header for normal DSK files
	extendedDSKHeader = "EXTENDED CPC DSK File\r\n" // Standard header for extended DSK files
	dskInfoHeader     = "Disk-Info\r\n"             // Disk information block identifier
	trackInfoHeader   = "Track-Info\r\n"            // Track information block identifier

	// DSK Sector size shifts (1=256, 2=512, 3=1024, etc.)
	dskSectorShift256  = 1
	dskSectorShift512  = 2
	dskSectorShift1024 = 3

	// DSK defaults
	defaultGap3Length = 0x4E // Standard gap 3 length
	defaultFiller     = 0xE5 // Standard filler byte for unformatted sectors
)

// DSKHeader represents the DSK disk information block
type DSKHeader struct {
	Signature  [34]byte // "MV - CPCEMU Disk-File\r\n" or "EXTENDED CPC DSK File\r\n" + "Disk-Info\r\n"
	Creator    [14]byte // Creator name
	Tracks     uint8    // Number of tracks
	Sides      uint8    // Number of sides
	TrackSize  uint16   // Track size in bytes (normal format only)
	TrackSizes []uint16 // Per-track sizes (extended format only)
	IsExtended bool     // True for extended format
}

// DSKTrackInfo represents the track information block
type DSKTrackInfo struct {
	Signature   [12]byte // "Track-Info\r\n"
	Unused      [4]byte
	TrackNumber uint8
	SideNumber  uint8
	Unused2     [2]byte
	SectorSize  uint8           // Sector size shift (1=256, 2=512, 3=1024, etc.)
	SectorCount uint8           // Number of sectors
	Gap3Length  uint8           // Gap 3 length
	FillerByte  uint8           // Filler byte
	SectorList  []DSKSectorInfo // Sector information list
}

// DSKSectorInfo represents sector information in the track list
type DSKSectorInfo struct {
	Track      uint8  // Track number (C parameter in uPD765 FDC)
	Side       uint8  // Side number (H parameter in uPD765 FDC)
	SectorID   uint8  // Sector ID (R parameter in uPD765 FDC)
	SectorSize uint8  // Sector size shift (N parameter in uPD765 FDC)
	StatusReg1 uint8  // uPD765 status register 1 after reading
	StatusReg2 uint8  // uPD765 status register 2 after reading
	DataLength uint16 // Actual data length (extended format only, 0 for normal)
}

// LoadDSK loads a DSK format disk image (+3 DOS)
func LoadDSK(r io.Reader) (*Disk, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read DSK file: %w", err)
	}

	// Check minimum file size (at least disk information block)
	if len(data) < 50 {
		return nil, fmt.Errorf("DSK file too short: expected at least 50 bytes, got %d", len(data))
	}

	// Parse DSK header
	header, err := parseDSKHeader(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse DSK header: %w", err)
	}

	// Create disk structure
	disk := &Disk{
		Type:          DiskTypeDSK,
		TracksPerSide: int(header.Tracks),
		Sides:         int(header.Sides),
		SectorSize:    512, // Default to 512-byte sectors for +3 DOS
		ReadOnly:      false,
	}

	// Initialize tracks
	totalTracks := int(header.Tracks) * int(header.Sides)
	disk.Tracks = make([]DiskTrack, totalTracks)

	// Parse track data
	dataOffset := 256 // Skip 256-byte disk information block

	for trackNum := 0; trackNum < int(header.Tracks); trackNum++ {
		for side := 0; side < int(header.Sides); side++ {
			trackIndex := trackNum + side*int(header.Tracks)

			// Extended DSK: the track size table marks unformatted tracks with
			// a size of 0 (no data and no track info block in the file).
			tableIdx := trackNum*int(header.Sides) + side
			if header.IsExtended && tableIdx < len(header.TrackSizes) && header.TrackSizes[tableIdx] == 0 {
				createDefaultDSKTrack(disk, trackIndex, trackNum, side)
				continue
			}

			// Some DSK files skip unformatted tracks
			if dataOffset >= len(data) {
				createDefaultDSKTrack(disk, trackIndex, trackNum, side)
				continue
			}

			// Check if we have a track header (not all DSK files have track headers)
			if dataOffset+12 > len(data) {
				// Not enough data for track header, create default
				createSimpleDSKTrack(disk, trackIndex, trackNum, side, data, dataOffset)
				continue
			}

			trackSignature := string(data[dataOffset : dataOffset+12])
			if trackSignature != trackInfoHeader {
				// Not a valid track header - might be simple DSK track format
				createSimpleDSKTrack(disk, trackIndex, trackNum, side, data, dataOffset)
				continue
			}

			// Parse track info
			trackInfo, err := parseDSKTrackInfo(data, dataOffset)
			if err != nil {
				// On error, try simple format
				createSimpleDSKTrack(disk, trackIndex, trackNum, side, data, dataOffset)
				continue
			}

			// Create sectors from track info
			disk.Tracks[trackIndex].Sectors = make([]DiskSector, trackInfo.SectorCount)
			// The track info block (24-byte header + sector list) is padded to a
			// 256-byte boundary; sector data follows immediately after.
			sectorDataOffset := alignNextBoundary(dataOffset + 24 + 8*int(trackInfo.SectorCount))

			for sectorIdx := 0; sectorIdx < int(trackInfo.SectorCount); sectorIdx++ {
				if sectorIdx >= len(trackInfo.SectorList) {
					break
				}

				sectorInfo := trackInfo.SectorList[sectorIdx]
				sectorSize := calculateSectorSize(sectorInfo.SectorSize)

				// Update disk sectors per track if needed
				if sectorIdx == 0 && disk.SectorsPerTrack == 0 {
					disk.SectorsPerTrack = int(trackInfo.SectorCount)
					disk.SectorSize = sectorSize
				}

				// The "actual data length" field has three meanings (Fuse
				// open_cpc / cpc_set_weak_range):
				//   0 or == nSize: a normal sector, nSize bytes.
				//   < nSize:       a truncated (copy-protected) sector.
				//   > nSize and a multiple of nSize: a WEAK sector with that many
				//     copies stored contiguously; the weak bytes are those that
				//     differ between copies and read back random each time.
				dl := int(sectorInfo.DataLength)
				numCopies := 1
				logicalLen := sectorSize
				if dl > 0 && dl != sectorSize {
					if dl > sectorSize && dl%sectorSize == 0 {
						numCopies = dl / sectorSize
					} else {
						logicalLen = dl // truncated
					}
				}
				storedLen := sectorSize
				if dl > 0 {
					storedLen = dl
				}

				sec := DiskSector{
					Track:    sectorInfo.Track,
					Side:     sectorInfo.Side,
					SectorID: sectorInfo.SectorID,
					Data:     make([]byte, logicalLen),
					Deleted:  sectorInfo.StatusReg2&0x40 != 0, // ST2 bit 6 (CM) = deleted data mark
					N:        sectorInfo.SectorSize,
					ST1:      sectorInfo.StatusReg1,
					ST2:      sectorInfo.StatusReg2,
				}
				if sectorDataOffset+logicalLen <= len(data) {
					copy(sec.Data, data[sectorDataOffset:sectorDataOffset+logicalLen])
				}

				// Weak sector: compare copies 1..n-1 against copy 0 and mark the
				// differing bytes weak (randomized on each read by sectorData).
				if numCopies > 1 {
					sec.Weak = make([]bool, sectorSize)
					for c := 1; c < numCopies; c++ {
						co := sectorDataOffset + c*sectorSize
						if co+sectorSize > len(data) {
							break
						}
						for i := 0; i < sectorSize; i++ {
							if data[co+i] != sec.Data[i] {
								sec.Weak[i] = true
							}
						}
					}
				}

				disk.Tracks[trackIndex].Sectors[sectorIdx] = sec
				sectorDataOffset += storedLen
			}

			// Advance to the next track. For extended DSK the track size table
			// (which already includes the 256-byte header, and is a multiple of
			// 256) is authoritative; for standard DSK compute it from the track
			// info and align to the next 256-byte boundary.
			if header.IsExtended && tableIdx < len(header.TrackSizes) && header.TrackSizes[tableIdx] > 0 {
				dataOffset += int(header.TrackSizes[tableIdx])
			} else {
				trackSize := calculateTrackSize(*trackInfo)
				dataOffset = alignNextBoundary(dataOffset + trackSize)
			}
		}
	}

	// Validate disk structure
	if len(disk.Tracks) == 0 {
		return nil, fmt.Errorf("DSK file contains no valid tracks")
	}

	return disk, nil
}

// parseDSKHeader parses the DSK disk information block
func parseDSKHeader(data []byte) (*DSKHeader, error) {
	if len(data) < 50 {
		return nil, fmt.Errorf("insufficient data for DSK header")
	}

	header := &DSKHeader{
		IsExtended: false,
	}

	// Check signature - DSK signatures have many variations
	// Standard: "MV - CPCEMU Disk-File", "MV - CPC format Disk Image", etc.
	// Extended: "EXTENDED CPC DSK File", "EXTENDED CPC DSK", etc.

	sigOffset := 32
	if len(data) < sigOffset {
		sigOffset = len(data)
	}
	sig := string(data[0:sigOffset])

	// First check for alternative DSK formats (non-CPC formats)
	altFormat := DetectAlternativeDSKFormat(data)
	if altFormat != "" {
		return nil, fmt.Errorf("not a CPC DSK file - detected alternative format: %s", altFormat)
	}

	// Try to detect format type based on signature patterns
	header.IsExtended = false

	// Check for extended format patterns
	if containsSub(sig, "EXTENDED CPC DSK") || containsSub(sig, "EXTENDED") {
		header.IsExtended = true
	} else if containsSub(sig, "MV - CPC") || containsSub(sig, "MV -") {
		header.IsExtended = false
	} else {
		return nil, fmt.Errorf("invalid DSK signature: got '%s'", sig)
	}

	// Copy whatever signature we have
	sigLen := 34
	if len(data) < sigLen {
		sigLen = len(data)
	}
	copy(header.Signature[:], data[0:sigLen])

	// Parse creator name (bytes 22-36 in the spec, but locations vary)
	creatorStart := 22
	creatorEnd := 36
	if creatorEnd > len(data) {
		creatorEnd = len(data)
	}
	if creatorEnd > creatorStart {
		copy(header.Creator[:], data[creatorStart:creatorEnd])
	}

	// Parse disk parameters (typically around byte 48-50)
	// Many DSK files have different parameter locations, try sensible defaults
	paramsStart := 48
	if paramsStart+2 <= len(data) {
		header.Tracks = data[paramsStart]
		header.Sides = data[paramsStart+1]
	} else {
		// Default values if not found
		header.Tracks = 40
		header.Sides = 2
	}

	if header.IsExtended {
		// Extended format: variable track sizes. One byte per track (high byte
		// of the track length, i.e. track length/256). A size of 0 marks an
		// unformatted track (no data and no track info block in the file).
		n := int(header.Tracks) * int(header.Sides)
		header.TrackSizes = make([]uint16, n)
		for i := 0; i < n && 52+i < len(data); i++ {
			header.TrackSizes[i] = uint16(data[52+i]) << 8
		}
	} else {
		// Normal format: track size typically around byte 50-52
		trackSizeStart := 50
		if trackSizeStart+2 <= len(data) {
			header.TrackSize = binary.LittleEndian.Uint16(data[trackSizeStart : trackSizeStart+2])
		}
	}

	return header, nil
}

// parseDSKTrackInfo parses a DSK track information block
func parseDSKTrackInfo(data []byte, offset int) (*DSKTrackInfo, error) {
	if offset+12 > len(data) {
		return nil, fmt.Errorf("insufficient data for track header")
	}

	trackInfo := &DSKTrackInfo{}

	// Parse track header
	if string(data[offset:offset+12]) != trackInfoHeader {
		return nil, fmt.Errorf("invalid track header signature")
	}
	copy(trackInfo.Signature[:], data[offset:offset+12])

	// Parse track parameters
	trackInfo.TrackNumber = data[offset+16]
	trackInfo.SideNumber = data[offset+17]
	trackInfo.SectorSize = data[offset+20]  // Sector size shift
	trackInfo.SectorCount = data[offset+21] // Number of sectors
	trackInfo.Gap3Length = data[offset+22]
	trackInfo.FillerByte = data[offset+23]

	// Parse sector list
	sectorListOffset := offset + 24
	trackInfo.SectorList = make([]DSKSectorInfo, trackInfo.SectorCount)

	for i := 0; i < int(trackInfo.SectorCount); i++ {
		sectorOffset := sectorListOffset + i*8
		if sectorOffset+8 > len(data) {
			break
		}

		trackInfo.SectorList[i] = DSKSectorInfo{
			Track:      data[sectorOffset+0],
			Side:       data[sectorOffset+1],
			SectorID:   data[sectorOffset+2],
			SectorSize: data[sectorOffset+3],
			StatusReg1: data[sectorOffset+4],
			StatusReg2: data[sectorOffset+5],
			DataLength: binary.LittleEndian.Uint16(data[sectorOffset+6 : sectorOffset+8]),
		}
	}

	return trackInfo, nil
}

// calculateSectorSize converts sector size shift to actual bytes
func calculateSectorSize(sizeShift uint8) int {
	// N is a 3-bit field (0-7); mask it so a corrupt image cannot shift past the
	// word width and produce 0 (which would divide-by-zero downstream).
	sizeShift &= 0x07
	// N=0 is 128 bytes, but the +3 DSK convention is 512-byte sectors.
	if sizeShift == 0 {
		return 512
	}
	return 128 << sizeShift
}

// calculateTrackSize calculates the size of a track in bytes
func calculateTrackSize(trackInfo DSKTrackInfo) int {
	// Track header: 24 bytes
	// Sector list: 8 bytes * sector count
	// Sector data: sum of actual sector sizes
	totalSize := 24 + 8*int(trackInfo.SectorCount)

	for _, sector := range trackInfo.SectorList {
		sectorSize := calculateSectorSize(sector.SectorSize)
		if sector.DataLength > 0 {
			totalSize += int(sector.DataLength)
		} else {
			totalSize += sectorSize
		}
	}

	return totalSize
}

// alignNextBoundary rounds up to the next 256-byte boundary
func alignNextBoundary(offset int) int {
	return ((offset + 255) / 256) * 256
}

// createSimpleDSKTrack creates a simple DSK track (for files without fullheaders)
func createSimpleDSKTrack(disk *Disk, trackIndex, trackNum, side int, fileData []byte, dataOffset int) {
	// Simple DSK format: just raw sector data
	// Assume 9 sectors of 512 bytes for +3 DOS
	sectorsPerTrack := 9
	sectorSize := 512

	if disk.SectorsPerTrack == 0 {
		disk.SectorsPerTrack = sectorsPerTrack
	}
	if disk.SectorSize == 0 {
		disk.SectorSize = sectorSize
	}

	disk.Tracks[trackIndex].Sectors = make([]DiskSector, sectorsPerTrack)
	for sectorIdx := 0; sectorIdx < sectorsPerTrack; sectorIdx++ {
		disk.Tracks[trackIndex].Sectors[sectorIdx] = DiskSector{
			Track:    uint8(trackNum),
			Side:     uint8(side),
			SectorID: uint8(sectorIdx + 1),
			Data:     make([]byte, sectorSize),
			Deleted:  false,
		}

		// Copy sector data if available
		offset := dataOffset + sectorIdx*sectorSize
		if offset+sectorSize <= len(fileData) {
			copy(disk.Tracks[trackIndex].Sectors[sectorIdx].Data,
				fileData[offset:offset+sectorSize])
		} else if offset < len(fileData) {
			// Copy whatever data is available
			availableBytes := len(fileData) - offset
			if availableBytes > sectorSize {
				availableBytes = sectorSize
			}
			copy(disk.Tracks[trackIndex].Sectors[sectorIdx].Data,
				fileData[offset:offset+availableBytes])
		}
	}
}

// createDefaultDSKTrack creates an unformatted track (no sectors). A track with
// a size of 0 in the extended-DSK track size table has no data and no track
// info block; READ ID must report "no data" for it, not a synthesised sector.
func createDefaultDSKTrack(disk *Disk, trackIndex, trackNum, side int) {
	disk.Tracks[trackIndex].Sectors = nil
}

// SaveDSK saves a disk in DSK format
func SaveDSK(w io.Writer, disk *Disk) error {
	// This would require generating proper DSK format with all headers and alignments
	// Implementation would be the reverse of LoadDSK
	return fmt.Errorf("DSK saving not yet implemented")
}
