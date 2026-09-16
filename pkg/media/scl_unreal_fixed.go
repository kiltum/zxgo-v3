package media

import (
	"encoding/binary"
	"fmt"
	"io"
)

// TR-DOS disk geometry.
//
// A TR-DOS track holds 16 sectors of 256 bytes, so a standard 80-track
// double-sided disk is 80 * 2 * 16 * 256 = 655360 bytes -- the familiar 640 KB
// .TRD size. Both reference implementations use these numbers
// (Unreal devices/fdd/fdd_trd.cpp: max_trd_sectors = 16).
const (
	trdSectorsPerTrack  = 16
	trdSectorSize       = 256
	trdTracksPerSide    = 80
	trdSides            = 2
	trdMaxTracksPerSide = 86 // Beta Disk hardware maximum

	// Logical sector addressing. TR-DOS numbers sectors linearly across the
	// disk and derives the physical address from that number:
	//
	//	cylinder  = pos / 32
	//	side      = (pos / 16) & 1
	//	sector ID = (pos & 15) + 1
	//
	// The catalog stores a file's start as this linear position, split into a
	// "track" byte (pos >> 4) and a "sector" byte (pos & 15).
	trdSectorsPerCylinder = trdSectorsPerTrack * trdSides
)

// TR-DOS disk-information sector. It is the 9th sector of track 0 side 0, and
// these are byte offsets within it (Unreal fdd_trd.cpp / fdd_scl.cpp).
const (
	trdInfoSector      = 9    // sector ID, not index
	trdInfoFirstSector = 0xE1 // first free sector, 0-15
	trdInfoFirstTrack  = 0xE2 // first free track, in units of 16 sectors
	trdInfoDiskType    = 0xE3 // 0x16 = 80 track double sided
	trdInfoFileCount   = 0xE4 // number of catalog entries in use
	trdInfoFreeSectors = 0xE5 // free sector count, little-endian word
	trdInfoTRDOSID     = 0xE7 // always 0x10
	trdInfoLabel       = 0xF5 // 8-byte disk label
)

// trdCatalogSectors is the number of sectors the catalog occupies on track 0:
// sector IDs 1 to 8, 16 entries of 16 bytes each, 128 files maximum.
const trdCatalogSectors = 8

// trdPos converts a linear TR-DOS sector position into a physical address.
func trdPos(pos int) (track uint8, side uint8, sector uint8) {
	return uint8(pos / trdSectorsPerCylinder),
		uint8((pos / trdSectorsPerTrack) & 1),
		uint8((pos % trdSectorsPerTrack) + 1)
}

// newBlankTRDOSDisk builds an empty, formatted TR-DOS disk of the given size.
func newBlankTRDOSDisk(tracksPerSide, sides int) *Disk {
	disk := &Disk{
		Type:            DiskTypeTRD,
		Sides:           sides,
		TracksPerSide:   tracksPerSide,
		SectorsPerTrack: trdSectorsPerTrack,
		SectorSize:      trdSectorSize,
		ReadOnly:        false,
	}

	disk.Tracks = make([]DiskTrack, tracksPerSide*sides)
	for i := range disk.Tracks {
		track := i % tracksPerSide
		side := i / tracksPerSide
		disk.Tracks[i].Sectors = make([]DiskSector, trdSectorsPerTrack)
		for s := 0; s < trdSectorsPerTrack; s++ {
			disk.Tracks[i].Sectors[s] = DiskSector{
				Track:    uint8(track),
				Side:     uint8(side),
				SectorID: uint8(s + 1),
				Data:     make([]byte, trdSectorSize),
			}
		}
	}
	return disk
}

// writeTRDOSInfoSector stamps the disk-information sector of a blank disk.
func writeTRDOSInfoSector(disk *Disk, tracksPerSide, sides int) {
	info := disk.sectorRef(0, 0, trdInfoSector)
	if info == nil {
		return
	}

	total := tracksPerSide * sides * trdSectorsPerTrack

	// First free sector sits right after the catalog: linear position 16,
	// i.e. track 1 (in 16-sector units), sector 0.
	info.Data[trdInfoFirstSector] = 0
	info.Data[trdInfoFirstTrack] = 1

	switch {
	case tracksPerSide >= 80 && sides == 2:
		info.Data[trdInfoDiskType] = 0x16
	case tracksPerSide >= 80:
		info.Data[trdInfoDiskType] = 0x18
	case sides == 2:
		info.Data[trdInfoDiskType] = 0x17
	default:
		info.Data[trdInfoDiskType] = 0x19
	}

	info.Data[trdInfoFileCount] = 0
	binary.LittleEndian.PutUint16(info.Data[trdInfoFreeSectors:], uint16(total-trdSectorsPerTrack))
	info.Data[trdInfoTRDOSID] = 0x10
	copy(info.Data[trdInfoLabel:trdInfoLabel+8], []byte("        "))
}

// sectorRef returns a pointer to the live sector, not a copy. The loaders need
// to write through it; ReadSector deliberately hands out copies instead.
func (d *Disk) sectorRef(track, side, sectorID uint8) *DiskSector {
	idx := int(track) + int(side)*d.TracksPerSide
	if idx < 0 || idx >= len(d.Tracks) {
		return nil
	}
	for i := range d.Tracks[idx].Sectors {
		if d.Tracks[idx].Sectors[i].SectorID == sectorID {
			return &d.Tracks[idx].Sectors[i]
		}
	}
	return nil
}

// addTRDOSFile appends one file to a TR-DOS disk, following Unreal's
// eFdd::AddFile (devices/fdd/fdd_scl.cpp): copy the 14-byte header into the
// next free catalog slot, append the file's start position as bytes 14 and 15,
// then write the data sectors and advance the free-space pointers.
func addTRDOSFile(disk *Disk, header []byte, data []byte) error {
	info := disk.sectorRef(0, 0, trdInfoSector)
	if info == nil {
		return fmt.Errorf("disk has no information sector")
	}

	lenSectors := int(header[13])
	free := int(binary.LittleEndian.Uint16(info.Data[trdInfoFreeSectors:]))
	if lenSectors > free {
		return fmt.Errorf("disk full: file needs %d sectors, %d free", lenSectors, free)
	}

	fileCount := int(info.Data[trdInfoFileCount])
	if fileCount >= trdCatalogSectors*16 {
		return fmt.Errorf("catalog full: %d files", fileCount)
	}

	// Catalog slot: 16 bytes per entry, 16 entries per sector, sector IDs 1-8.
	entryPos := fileCount * 16
	dir := disk.sectorRef(0, 0, uint8(1+entryPos/trdSectorSize))
	if dir == nil {
		return fmt.Errorf("catalog sector for entry %d missing", fileCount)
	}
	off := entryPos % trdSectorSize
	copy(dir.Data[off:off+14], header[:14])
	dir.Data[off+14] = info.Data[trdInfoFirstSector]
	dir.Data[off+15] = info.Data[trdInfoFirstTrack]

	// Copy the data into the sectors starting at the first free position.
	pos := int(info.Data[trdInfoFirstSector]) + trdSectorsPerTrack*int(info.Data[trdInfoFirstTrack])
	for i := 0; i < lenSectors; i++ {
		track, side, sector := trdPos(pos + i)
		sec := disk.sectorRef(track, side, sector)
		if sec == nil {
			return fmt.Errorf("sector %d (trk %d side %d sec %d) out of range",
				pos+i, track, side, sector)
		}
		start := i * trdSectorSize
		if start < len(data) {
			end := start + trdSectorSize
			if end > len(data) {
				end = len(data)
			}
			copy(sec.Data, data[start:end])
		}
	}

	next := pos + lenSectors
	info.Data[trdInfoFirstSector] = uint8(next & 0x0F)
	info.Data[trdInfoFirstTrack] = uint8(next >> 4)
	info.Data[trdInfoFileCount] = uint8(fileCount + 1)
	binary.LittleEndian.PutUint16(info.Data[trdInfoFreeSectors:], uint16(free-lenSectors))
	return nil
}

// LoadSCLUnreal loads an SCL image by building a real TR-DOS disk from it,
// which is how both reference emulators handle the format.
func LoadSCLUnreal(r io.Reader) (*Disk, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read SCL file: %w", err)
	}
	if len(data) < 9 {
		return nil, fmt.Errorf("SCL file too short: %d bytes", len(data))
	}
	if string(data[0:8]) != "SINCLAIR" {
		return nil, fmt.Errorf("invalid SCL signature: expected 'SINCLAIR', got %q", data[0:8])
	}

	numFiles := int(data[8])
	if numFiles == 0 {
		return nil, fmt.Errorf("SCL file contains no files")
	}

	headerEnd := 9 + 14*numFiles
	if len(data) < headerEnd {
		return nil, fmt.Errorf("SCL file too short for %d file headers: need %d bytes, got %d",
			numFiles, headerEnd, len(data))
	}

	disk := newBlankTRDOSDisk(trdTracksPerSide, trdSides)
	writeTRDOSInfoSector(disk, trdTracksPerSide, trdSides)

	// File data follows the headers, one file after another, each a whole
	// number of 256-byte sectors. Some SCL files carry a 4-byte checksum
	// after the data; a short final read is tolerated rather than rejected.
	offset := headerEnd
	for i := 0; i < numFiles; i++ {
		header := data[9+i*14 : 9+i*14+14]
		size := int(header[13]) * trdSectorSize // byte 13 = length in sectors

		end := offset + size
		if end > len(data) {
			end = len(data)
		}
		if offset > len(data) {
			return nil, fmt.Errorf("SCL file truncated: file %d starts past end of data", i)
		}

		if err := addTRDOSFile(disk, header, data[offset:end]); err != nil {
			return nil, fmt.Errorf("SCL file %d (%q): %w", i, header[0:8], err)
		}
		offset += size
	}

	return disk, nil
}
