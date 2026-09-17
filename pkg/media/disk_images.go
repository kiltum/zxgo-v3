package media

import (
	"fmt"
	"io"
	"math/rand"
)

// DiskType represents different disk formats
type DiskType string

const (
	DiskTypeTRD DiskType = "TRD" // TR-DOS format for Beta Disk
	DiskTypeSCL DiskType = "SCL" // Compressed TR-DOS format
	DiskTypeMGT DiskType = "MGT" // Disciple/+D format (interleaved)
	DiskTypeIMG DiskType = "IMG" // Disciple/+D format (sequential)
	DiskTypeDSK DiskType = "DSK" // +3 DOS format
	DiskTypeRAW DiskType = "RAW" // Raw disk image
)

// DiskTrack represents a single track on a floppy disk
type DiskTrack struct {
	Sectors []DiskSector
}

// DiskSector represents a single sector on a floppy disk
type DiskSector struct {
	Track    uint8
	Side     uint8
	SectorID uint8
	Data     []byte
	Deleted  bool   // True if sector is marked as deleted
	N        uint8  // Sector size code (N parameter: 128<<N bytes)
	ST1      uint8  // FDC status register 1 after reading (0x20 = DE / CRC error)
	ST2      uint8  // FDC status register 2 after reading (0x40 = CM, 0x20 = DD)
	IDCRC    uint16 // ID-field CRC (0xCDB4-seeded CCITT), computed and cached on first use
	Weak     []bool // weak-bit mask (nil = no weak bits); Weak[i] reads back random
}

// Disk represents a complete floppy disk
type Disk struct {
	Type            DiskType
	Tracks          []DiskTrack // Array of tracks
	Sides           int         // Number of sides (1 or 2)
	TracksPerSide   int         // Number of tracks per side (typically 40 or 80)
	SectorsPerTrack int         // Sectors per track (typically 9 for TR-DOS)
	SectorSize      int         // Bytes per sector (256 for TR-DOS, 512 for +3)
	ReadOnly        bool        // Write protection flag
}

// TRDiskHeader represents the TR-DOS disk header
type TRDiskHeader struct {
	_unused0     [9]byte
	DiskName     [8]byte
	_unused1     [8]byte
	FilesCount   uint8
	FreeSectors  uint16
	Track0Sector uint8
	DiskType     uint8
	_unused2     [2]byte
}

// TRDOSFileEntry represents a TR-DOS directory entry
type TRDOSFileEntry struct {
	FileType     uint8 // File type (BASIC, CODE, etc.)
	Filename     [8]byte
	Ext          [3]byte
	StartAddress uint16
	Length       uint16
	StartSector  uint16
	StartTrack   uint8
	NoSectors    uint8
}

// NewTRDisk creates a new blank, formatted TR-DOS disk: 80 tracks per side,
// double-sided, 16 sectors of 256 bytes per track (640 KB).
func NewTRDisk() *Disk {
	disk := newBlankTRDOSDisk(trdTracksPerSide, trdSides)
	writeTRDOSInfoSector(disk, trdTracksPerSide, trdSides)
	return disk
}

// LoadTRDisk loads a .TRD format disk image.
//
// A .TRD file is the raw sector image in TR-DOS linear order: 16 sectors of
// 256 bytes per track, sides interleaved track by track, so sector n of the
// file lives at cylinder n/32, side (n/16)&1, sector ID (n%16)+1. Files are
// commonly truncated after the last used sector, so the image is padded out to
// a full disk rather than having its geometry inferred from its length.
func LoadTRDisk(r io.Reader) (*Disk, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read TR-DOS disk image: %w", err)
	}
	if len(data) < trdSectorSize {
		return nil, fmt.Errorf("TR-DOS disk image too short: expected at least %d bytes, got %d",
			trdSectorSize, len(data))
	}

	// Size the disk to hold the image, never smaller than the standard 80/2.
	cylinders := (len(data) + trdSectorsPerCylinder*trdSectorSize - 1) /
		(trdSectorsPerCylinder * trdSectorSize)
	if cylinders < trdTracksPerSide {
		cylinders = trdTracksPerSide
	}
	if cylinders > trdMaxTracksPerSide {
		cylinders = trdMaxTracksPerSide
	}

	disk := newBlankTRDOSDisk(cylinders, trdSides)
	disk.Type = DiskTypeTRD

	for pos := 0; pos*trdSectorSize < len(data); pos++ {
		track, side, sectorID := trdPos(pos)
		sec := disk.sectorRef(track, side, sectorID)
		if sec == nil {
			break // image is larger than the drive can hold
		}
		start := pos * trdSectorSize
		end := start + trdSectorSize
		if end > len(data) {
			end = len(data)
		}
		copy(sec.Data, data[start:end])
	}

	return disk, nil
}

// SaveTRDisk saves a disk in .TRD format, in TR-DOS linear sector order
// (the inverse of LoadTRDisk).
func SaveTRDisk(w io.Writer, disk *Disk) error {
	if disk.SectorSize != trdSectorSize {
		return fmt.Errorf("unsupported sector size for TR-DOS: %d", disk.SectorSize)
	}
	if disk.SectorsPerTrack != trdSectorsPerTrack {
		return fmt.Errorf("unsupported sectors per track for TR-DOS: %d", disk.SectorsPerTrack)
	}

	totalSectors := len(disk.Tracks) * disk.SectorsPerTrack
	buffer := make([]byte, totalSectors*disk.SectorSize)

	for pos := 0; pos < totalSectors; pos++ {
		track, side, sectorID := trdPos(pos)
		sec := disk.sectorRef(track, side, sectorID)
		if sec == nil {
			continue
		}
		copy(buffer[pos*disk.SectorSize:], sec.Data)
	}

	if _, err := w.Write(buffer); err != nil {
		return fmt.Errorf("failed to write TR-DOS disk image: %w", err)
	}
	return nil
}

// sectorData returns a copy of the sector's data with weak bits randomized, so a
// weak sector reads back unstable on each read exactly as real hardware returns
// noise for those bytes (Fuse fdd.c weak-byte scrambling).
func sectorData(s *DiskSector) []byte {
	result := make([]byte, len(s.Data))
	copy(result, s.Data)
	if s.Weak != nil {
		for i, w := range s.Weak {
			if w {
				result[i] = byte(rand.Intn(256))
			}
		}
	}
	return result
}

// SectorWeak reports whether the sector at physical track/head pcn/head whose ID
// fields (C/H/R) match carries weak bits (reads back random each time).
func (d *Disk) SectorWeak(pcn, head, c, h, r uint8) bool {
	s := d.sectorAtID(pcn, head, c, h, r)
	return s != nil && s.Weak != nil
}

// ReadSector reads data from a specific track, side, and sector
func (d *Disk) ReadSector(track, side, sector uint8) ([]byte, error) {
	// Calculate track index accounting for sides
	trackIndex := int(track) + (int(side) * d.TracksPerSide)

	if trackIndex >= len(d.Tracks) {
		return nil, fmt.Errorf("invalid track %d (max %d)", track, len(d.Tracks)-1)
	}

	trackData := d.Tracks[trackIndex]
	for _, sec := range trackData.Sectors {
		if sec.SectorID == sector && (sec.Side == side || d.Sides == 1) {
			return sectorData(&sec), nil
		}
	}

	return nil, fmt.Errorf("sector %d not found on track %d, side %d", sector, track, side)
}

// sectorAtID returns the sector at the given physical track/head whose recorded
// C/H/R ID fields match. The uPD765 matches a sector by its ID against the
// READ/WRITE DATA command's C/H/R parameters at the head's physical position (the
// PCN), and copy-protected disks record deliberately wrong C/H values (e.g. a
// sector on physical track 2 whose ID claims C=143), so the physical position and
// the ID are independent.
func (d *Disk) sectorAtID(pcn, head, c, h, r uint8) *DiskSector {
	ti := int(pcn) + int(head)*d.TracksPerSide
	if ti >= len(d.Tracks) {
		return nil
	}
	for i := range d.Tracks[ti].Sectors {
		if s := &d.Tracks[ti].Sectors[i]; s.Track == c && s.Side == h && s.SectorID == r {
			return s
		}
	}
	return nil
}

// ReadSectorByID reads the data of the sector at physical track/head pcn/head
// whose ID fields (C/H/R) match. ok is false if no such sector exists. Weak
// sectors return randomized data, so a re-read differs from the previous read.
func (d *Disk) ReadSectorByID(pcn, head, c, h, r uint8) (data []byte, ok bool) {
	s := d.sectorAtID(pcn, head, c, h, r)
	if s == nil {
		return nil, false
	}
	return sectorData(s), true
}

// SectorStatusByID returns the recorded ST1/ST2 for the sector at physical
// track/head pcn/head whose ID fields (C/H/R) match.
func (d *Disk) SectorStatusByID(pcn, head, c, h, r uint8) (st1, st2 uint8, ok bool) {
	s := d.sectorAtID(pcn, head, c, h, r)
	if s == nil {
		return 0, 0, false
	}
	return s.ST1, s.ST2, true
}

// SectorDeletedByID reports whether the sector at physical track/head pcn/head
// whose ID fields (C/H/R) match carries a deleted-data mark.
func (d *Disk) SectorDeletedByID(pcn, head, c, h, r uint8) bool {
	s := d.sectorAtID(pcn, head, c, h, r)
	return s != nil && s.Deleted
}

// SectorNByID returns the size code (N) of the sector at physical track/head
// pcn/head whose ID fields (C/H/R) match.
func (d *Disk) SectorNByID(pcn, head, c, h, r uint8) (uint8, bool) {
	s := d.sectorAtID(pcn, head, c, h, r)
	if s == nil {
		return 0, false
	}
	return s.N, true
}

// SectorSizeByID returns the recorded data length of the sector at physical
// track/head pcn/head whose ID fields (C/H/R) match. This is len(s.Data), not
// 128<<N: truncated sectors (copy protection) store fewer bytes than their N
// size code implies.
func (d *Disk) SectorSizeByID(pcn, head, c, h, r uint8) (int, bool) {
	s := d.sectorAtID(pcn, head, c, h, r)
	if s == nil {
		return 0, false
	}
	return len(s.Data), true
}

// WriteSectorByID writes data to the sector at physical track/head pcn/head whose
// ID fields (C/H/R) match.
func (d *Disk) WriteSectorByID(pcn, head, c, h, r uint8, data []byte) error {
	if d.ReadOnly {
		return fmt.Errorf("disk is write protected")
	}
	s := d.sectorAtID(pcn, head, c, h, r)
	if s == nil {
		return fmt.Errorf("sector C=%d H=%d R=%d not found on track %d", c, h, r, pcn)
	}
	if len(data) != len(s.Data) {
		return fmt.Errorf("invalid data size: expected %d, got %d", len(s.Data), len(data))
	}
	copy(s.Data, data)
	return nil
}

// FirstSectorID returns the sector ID of the first sector on a track/side. The
// +3 uses this (via READ ID) to detect the disk's sector-ID convention: 01-09
// for +3 format, 41-49 for CPC system, C1-C9 for CPC data.
func (d *Disk) FirstSectorID(track, side uint8) (uint8, bool) {
	trackIndex := int(track) + (int(side) * d.TracksPerSide)
	if trackIndex >= len(d.Tracks) || len(d.Tracks[trackIndex].Sectors) == 0 {
		return 0, false
	}
	return d.Tracks[trackIndex].Sectors[0].SectorID, true
}

// FirstSectorN returns the sector size code (N) of the first sector on a
// track/side. Copy-protected loaders check this via READ ID to verify a sector
// was recorded with a non-standard size (e.g. N=6 for 8192-byte sectors).
func (d *Disk) FirstSectorN(track, side uint8) (uint8, bool) {
	trackIndex := int(track) + (int(side) * d.TracksPerSide)
	if trackIndex >= len(d.Tracks) || len(d.Tracks[trackIndex].Sectors) == 0 {
		return 0, false
	}
	return d.Tracks[trackIndex].Sectors[0].N, true
}

// SectorDeleted reports whether a sector carries the deleted data mark (ST2 bit
// 6 / CM). Copy-protected loaders use READ DELETED DATA against these sectors.
func (d *Disk) SectorDeleted(track, side, sector uint8) bool {
	trackIndex := int(track) + (int(side) * d.TracksPerSide)
	if trackIndex >= len(d.Tracks) {
		return false
	}
	for _, sec := range d.Tracks[trackIndex].Sectors {
		if sec.SectorID == sector && (sec.Side == side || d.Sides == 1) {
			return sec.Deleted
		}
	}
	return false
}

// SectorStatus returns the FDC status bytes (ST1, ST2) recorded for a sector in
// the image. Extended DSK stores these to model copy protection (e.g. ST1 bit 5
// DE = CRC error for a truncated sector). ok is false if the sector is absent.
func (d *Disk) SectorStatus(track, side, sector uint8) (st1, st2 uint8, ok bool) {
	trackIndex := int(track) + (int(side) * d.TracksPerSide)
	if trackIndex >= len(d.Tracks) {
		return 0, 0, false
	}
	for _, sec := range d.Tracks[trackIndex].Sectors {
		if sec.SectorID == sector && (sec.Side == side || d.Sides == 1) {
			return sec.ST1, sec.ST2, true
		}
	}
	return 0, 0, false
}

// WriteSector writes data to a specific track, side, and sector
func (d *Disk) WriteSector(track, side, sector uint8, data []byte) error {
	if d.ReadOnly {
		return fmt.Errorf("disk is write protected")
	}

	trackIndex := int(track) + (int(side) * d.TracksPerSide)

	if trackIndex >= len(d.Tracks) {
		return fmt.Errorf("invalid track %d (max %d)", track, len(d.Tracks)-1)
	}

	trackData := d.Tracks[trackIndex]
	for i := range trackData.Sectors {
		if trackData.Sectors[i].SectorID == sector && trackData.Sectors[i].Side == side {
			if len(data) != d.SectorSize {
				return fmt.Errorf("invalid data size: expected %d, got %d", d.SectorSize, len(data))
			}
			copy(trackData.Sectors[i].Data, data)
			return nil
		}
	}

	return fmt.Errorf("sector %d not found on track %d, side %d", sector, track, side)
}

// FormatDisk formats a disk with fresh sector data
func (d *Disk) FormatDisk() error {
	if d.ReadOnly {
		return fmt.Errorf("disk is write protected")
	}

	for track := 0; track < len(d.Tracks); track++ {
		for sector := 0; sector < d.SectorsPerTrack; sector++ {
			// A track may hold fewer than SectorsPerTrack sectors (an unformatted
			// track has none, a truncated image has fewer); skip the missing ones
			// rather than indexing past the slice.
			if sector >= len(d.Tracks[track].Sectors) {
				break
			}
			d.Tracks[track].Sectors[sector].Data = make([]byte, d.SectorSize)
			d.Tracks[track].Sectors[sector].Deleted = false
		}
	}

	return nil
}

// FormatTrack replaces the sectors on a physical track with sc newly-formatted
// sectors whose IDs come from the C/H/R/N bytes supplied by a FORMAT TRACK
// command, each filled with the data fill byte. n is the command's size code.
func (d *Disk) FormatTrack(track, side uint8, n uint8, fill byte, ids []byte) {
	ti := int(track) + int(side)*d.TracksPerSide
	if ti >= len(d.Tracks) {
		return
	}
	sc := len(ids) / 4
	size := 128 << n
	sectors := make([]DiskSector, sc)
	for i := 0; i < sc; i++ {
		data := make([]byte, size)
		for j := range data {
			data[j] = fill
		}
		sectors[i] = DiskSector{
			Track:    ids[i*4+0],
			Side:     ids[i*4+1],
			SectorID: ids[i*4+2],
			N:        ids[i*4+3],
			Data:     data,
		}
	}
	d.Tracks[ti].Sectors = sectors
}

// GetDiskInfo returns information about the disk
func (d *Disk) GetDiskInfo() string {
	totalSectors := len(d.Tracks) * d.SectorsPerTrack
	totalBytes := totalSectors * d.SectorSize

	return fmt.Sprintf("Disk Type: %s, Tracks: %d, Sides: %d, Sectors/Track: %d, Size: %d bytes",
		d.Type, d.TracksPerSide, d.Sides, d.SectorsPerTrack, totalBytes)
}

// DetectDiskFormat determines the disk format from file data
func DetectDiskFormat(data []byte) DiskType {
	if len(data) < 256 {
		return DiskTypeRAW
	}

	// Check for TR-DOS signature in header (typically has characteristic patterns)
	// Simple heuristic: TR-DOS files are typically 80*9*256 = 184320 bytes
	const trdSize = 80 * 9 * 256
	if len(data) == trdSize {
		// Additional check for TR-DOS directory structure
		if data[228] <= 0x10 { // Disk type field typical values
			return DiskTypeTRD
		}
	}

	// Check for SCL format (has SCL header)
	if len(data) >= 8 && string(data[0:4]) == "AZXC" || string(data[0:4]) == "RSXD" {
		return DiskTypeSCL
	}

	// Default to RAW for unknown formats
	return DiskTypeRAW
}
