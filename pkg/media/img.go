package media

import (
	"fmt"
	"io"
)

// IMG format support for Disciple/+D disk images
// IMG is a simple raw dump format with sequential sides: side0/track0 through side0/track79, then side1/track0 through side1/track79
// IMPORTANT: MGT/IMG formats use 512-byte sectors (not 256 like TR-DOS!)

const (
	imgSectorSize     = 512  // IMG format uses 512-byte sectors (not 256!)
	imgTracksPerSide   = 80   // Tracks per side for Disciple/+D
	imgSides           = 2    // Double-sided disks
	imgSectorsPerTrack = 10   // Disciple/+D sectors per track
	imgTotalTracks     = imgTracksPerSide * imgSides // Total tracks in IMG format
)

// LoadIMG loads a IMG format disk image (Disciple/+D)
func LoadIMG(r io.Reader) (*Disk, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read IMG file: %w", err)
	}

	// Calculate expected file size for IMG format
	// Size = tracks * sides * sectors * imgSectorSize
	expectedSize := imgTracksPerSide * imgSides * imgSectorsPerTrack * imgSectorSize

	// Check if file size matches expected format (within tolerance)
	minSize := imgTotalTracks * imgSectorsPerTrack * imgSectorSize / 4 // Allow quarter-size minimum
	maxSize := expectedSize * 2 // Allow double-size for extended formats

	if len(data) < minSize {
		return nil, fmt.Errorf("IMG file too short: expected at least %d bytes, got %d", minSize, len(data))
	}

	if len(data) > maxSize {
		return nil, fmt.Errorf("IMG file too large: expected around %d bytes, got %d", expectedSize, len(data))
	}

	// Determine actual disk size by analyzing the data
	// IMG is sequential: side0/track0 through side0/track79, then side1/track0 through side1/track79
	tracksPerSide := imgTracksPerSide
	actualSectorsPerTrack := imgSectorsPerTrack

	// Adjust for partial disks
	if len(data) < expectedSize {
		// Calculate how many tracks we actually have
		sectorsInFile := len(data) / imgSectorSize
		singleSideTracks := sectorsInFile / actualSectorsPerTrack

		if singleSideTracks < imgTracksPerSide {
			// Single-sided disk
			tracksPerSide = singleSideTracks
		} else {
			// Check if double-sided
			doubleSideTracks := singleSideTracks / 2
			if doubleSideTracks > 0 && doubleSideTracks <= imgTracksPerSide {
				tracksPerSide = doubleSideTracks
			}
		}
	}

	// Determine number of sides
	sides := imgSides
	if len(data) <= imgTracksPerSide*actualSectorsPerTrack*imgSectorSize {
		sides = 1 // Single-sided if it fits in one side
	}

	// Create disk structure
	totalTracks := tracksPerSide * sides
	disk := NewTRDisk()
	disk.Type = DiskTypeIMG
	disk.TracksPerSide = tracksPerSide
	disk.Tracks = make([]DiskTrack, totalTracks)

	// Initialize tracks with 512-byte sectors (different from TR-DOS's 256-byte)
	for track := 0; track < totalTracks; track++ {
		disk.Tracks[track].Sectors = make([]DiskSector, actualSectorsPerTrack)
		for sector := 0; sector < actualSectorsPerTrack; sector++ {
			disk.Tracks[track].Sectors[sector] = DiskSector{
				Track:    uint8(track),
				Side:     uint8(track / tracksPerSide),
				SectorID: uint8(sector + 1),
				Data:     make([]byte, imgSectorSize), // 512-byte sectors
				Deleted:  false,
			}
		}
	}

	// Parse IMG sequential format
	dataOffset := 0

	// IMG format: side0/track0, side0/track1, ..., side0/track79, side1/track0, side1/track1, ...
	for trackNum := 0; trackNum < tracksPerSide*sides; trackNum++ {
		if dataOffset+actualSectorsPerTrack*imgSectorSize > len(data) {
			break // End of file reached
		}

		if trackNum >= len(disk.Tracks) {
			break // Don't exceed allocated tracks
		}

		// Copy sectors for this track
		for sector := 0; sector < actualSectorsPerTrack; sector++ {
			if sector >= len(disk.Tracks[trackNum].Sectors) {
				break
			}

			copy(disk.Tracks[trackNum].Sectors[sector].Data,
				data[dataOffset:dataOffset+imgSectorSize])
			dataOffset += imgSectorSize
		}
	}

	return disk, nil
}

// SaveIMG saves a disk in IMG format
func SaveIMG(w io.Writer, disk *Disk) error {
	// Convert disk to IMG sequential format
	// IMG format: side0/track0, side0/track1, ..., side0/track79, side1/track0, side1/track1, ...

	tracksPerSide := disk.TracksPerSide
	if tracksPerSide == 0 {
		tracksPerSide = 80 // Default to 80 tracks
	}
	if tracksPerSide > imgTracksPerSide {
		tracksPerSide = imgTracksPerSide
	}

	sectorsPerTrack := imgSectorsPerTrack
	if len(disk.Tracks) > 0 && len(disk.Tracks[0].Sectors) > 0 {
		sectorsPerTrack = len(disk.Tracks[0].Sectors)
	}

	// Process all tracks sequentially
	for trackNum := 0; trackNum < len(disk.Tracks); trackNum++ {
		track := disk.Tracks[trackNum]

		for sector := 0; sector < sectorsPerTrack; sector++ {
			if sector >= len(track.Sectors) {
				break
			}

			_, err := w.Write(track.Sectors[sector].Data)
			if err != nil {
				return fmt.Errorf("failed to write IMG data: %w", err)
			}
		}
	}

	return nil
}
