package media

import "encoding/binary"

// DiskInfo provides detailed information about a disk image
type DiskInfo struct {
	Name            string   // Disk name from header
	Type            DiskType // Disk format type
	Tracks          int      // Total number of tracks
	Sides           int      // Number of sides
	Sectors         int      // Total number of sectors
	SectorsPerTrack int      // Sectors per track
	SizeKB          int      // Total size in kilobytes
	FreeSectors     int      // Number of free sectors (for TR-DOS)
	FileCount       int      // Number of files on disk (for TR-DOS)
}

// GetDetailedInfo returns a DiskInfo struct with detailed information about the disk.
//
// For TR-DOS the metadata lives in the disk-information sector: track 0, side 0,
// sector ID 9 -- not in the first sector, which is the start of the catalog.
func (d *Disk) GetDetailedInfo() DiskInfo {
	info := DiskInfo{
		Type:            d.Type,
		Tracks:          d.TracksPerSide * d.Sides,
		Sides:           d.Sides,
		Sectors:         len(d.Tracks) * d.SectorsPerTrack,
		SectorsPerTrack: d.SectorsPerTrack,
		SizeKB:          (len(d.Tracks) * d.SectorsPerTrack * d.SectorSize) / 1024,
	}

	if d.Type != DiskTypeTRD {
		return info
	}

	sec := d.sectorRef(0, 0, trdInfoSector)
	if sec == nil || len(sec.Data) < trdSectorSize {
		return info
	}

	info.FileCount = int(sec.Data[trdInfoFileCount])
	info.FreeSectors = int(binary.LittleEndian.Uint16(sec.Data[trdInfoFreeSectors:]))
	info.Name = string(sec.Data[trdInfoLabel : trdInfoLabel+8])

	return info
}
