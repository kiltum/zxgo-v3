package media

import (
	"fmt"
	"io"
)

// MGT format support for Disciple/+D disk images
// MGT is a simple raw dump format with interleaved sides: side0/track0, side1/track0, side0/track1, side1/track1, etc.
// IMPORTANT: MGT/IMG formats use 512-byte sectors (not 256 like TR-DOS!)

const (
	mgtSectorSize   = 512  // MGT format uses 512-byte sectors (not 256!)
	mgtTracksPerSide  = 80   // Tracks per side for Disciple/+D
	mgtSides          = 2    // Double-sided disks
	mgtSectorsPerTrack = 10   // Disciple/+D sectors per track
	mgtTotalTracks    = mgtTracksPerSide * mgtSides // Total tracks in MGT format
)

// LoadMGT loads a MGT format disk image (Disciple/+D)
func LoadMGT(r io.Reader) (*Disk, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read MGT file: %w", err)
	}

	// Calculate expected file size for MGT format
	// Size = tracks * sides * sectors * mgtSectorSize
	expectedSize := mgtTracksPerSide * mgtSides * mgtSectorsPerTrack * mgtSectorSize

	// Check if file size matches expected format (within tolerance)
	minSize := mgtTotalTracks * mgtSectorsPerTrack * mgtSectorSize / 2 // Allow half-size (single-sided)
	maxSize := expectedSize + 4096 // Allow some tolerance

	if len(data) < minSize {
		return nil, fmt.Errorf("MGT file too short: expected at least %d bytes, got %d", minSize, len(data))
	}

	if len(data) > maxSize {
		return nil, fmt.Errorf("MGT file too large: expected %d bytes, got %d", expectedSize, len(data))
	}

	// Determine actual disk size by analyzing the data
	// MGT is interleaved: side0/track0, side1/track0, side0/track1, side1/track1, etc.
	tracksPerSide := mgtTracksPerSide
	actualSectorsPerTrack := mgtSectorsPerTrack

	// Adjust for partial disks
	if len(data) < expectedSize {
		// Calculate how many tracks we actually have
		sectorsInFile := len(data) / mgtSectorSize
		trackPairs := sectorsInFile / (actualSectorsPerTrack * 2) // 2 sides per track pair

		if trackPairs > 0 && trackPairs < mgtTracksPerSide {
			tracksPerSide = trackPairs
		}
	}

	// Create disk structure
	totalTracks := tracksPerSide * mgtSides
	disk := NewTRDisk()
	disk.Type = DiskTypeMGT
	disk.TracksPerSide = tracksPerSide
	disk.Tracks = make([]DiskTrack, totalTracks)

	// Initialize tracks with 512-byte sectors (different from TR-DOS's 256-byte)
	for track := 0; track < totalTracks; track++ {
		disk.Tracks[track].Sectors = make([]DiskSector, actualSectorsPerTrack)
		for sector := 0; sector < actualSectorsPerTrack; sector++ {
			disk.Tracks[track].Sectors[sector] = DiskSector{
				Track:    uint8(track),
				Side:     uint8(track / mgtTracksPerSide),
				SectorID: uint8(sector + 1),
				Data:     make([]byte, mgtSectorSize), // 512-byte sectors
				Deleted:  false,
			}
		}
	}

	// Parse MGT interleaved format
	dataOffset := 0

	// MGT format: side0/track0, side1/track0, side0/track1, side1/track1, ...
	for trackNum := 0; trackNum < tracksPerSide; trackNum++ {
		for side := 0; side < mgtSides; side++ {
			// Calculate actual track number in our linear representation
			// Side 0, Track 0 -> Track 0
			// Side 1, Track 0 -> Track 80
			// Side 0, Track 1 -> Track 1
			actualTrack := side*mgtTracksPerSide + trackNum

			if actualTrack >= len(disk.Tracks) {
				break // Don't exceed allocated tracks
			}

			// Copy sectors for this track
			for sector := 0; sector < actualSectorsPerTrack; sector++ {
				if dataOffset+mgtSectorSize > len(data) {
					break // End of file reached
				}

				if sector < len(disk.Tracks[actualTrack].Sectors) {
					copy(disk.Tracks[actualTrack].Sectors[sector].Data,
						data[dataOffset:dataOffset+mgtSectorSize])
				}

				dataOffset += mgtSectorSize
			}
		}
	}

	return disk, nil
}

// SaveMGT saves a disk in MGT format
func SaveMGT(w io.Writer, disk *Disk) error {
	// Convert disk to MGT interleaved format
	// MGT format: side0/track0, side1/track0, side0/track1, side1/track1, ...

	tracksPerSide := disk.TracksPerSide
	if tracksPerSide == 0 {
		tracksPerSide = 80 // Default to 80 tracks
	}
	if tracksPerSide > mgtTracksPerSide {
		tracksPerSide = mgtTracksPerSide
	}

	sectorsPerTrack := mgtSectorsPerTrack

	for trackNum := 0; trackNum < tracksPerSide; trackNum++ {
		for side := 0; side < mgtSides; side++ {
			actualTrack := side*mgtTracksPerSide + trackNum

			if actualTrack >= len(disk.Tracks) {
				continue
			}

			track := disk.Tracks[actualTrack]

			for sector := 0; sector < sectorsPerTrack; sector++ {
				if sector >= len(track.Sectors) {
					break
				}

				_, err := w.Write(track.Sectors[sector].Data)
				if err != nil {
					return fmt.Errorf("failed to write MGT data: %w", err)
				}
			}
		}
	}

	return nil
}
