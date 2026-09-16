package media

import (
	"fmt"
	"io"
)

// SCLEntry represents a file entry in SCL format.
//
// The 14-byte layout matches the first 14 bytes of a TR-DOS catalog entry:
// 8 bytes of name, 1 type byte, 4 parameter bytes (start address word then
// length-in-bytes word), and the length in sectors at offset 13. The catalog's
// last two bytes -- start sector and start track -- are not in the SCL and get
// filled in when the file is placed on the disk.
type SCLEntry struct {
	Filename    [8]byte // File name (8 characters)
	FileType    byte    // File type
	Params      [4]byte // Start address word, then length-in-bytes word
	LengthSect  byte    // Length in sectors (offset 13)
}

// SCLHeader represents the SCL file header
type SCLHeader struct {
	Signature  [8]byte // "SINCLAIR"
	FilesCount byte    // Number of files
}

// NewSCLEntry creates a new SCL entry from TR-DOS file entry
func NewSCLEntry(trdEntry TRDOSFileEntry) SCLEntry {
	return SCLEntry{
		Filename:   trdEntry.Filename,
		FileType:   trdEntry.FileType,
		LengthSect: byte((trdEntry.Length + 255) / 256), // Convert bytes to sectors
	}
}

// ToTRDOSFileEntry converts SCL entry to TR-DOS file entry
func (e SCLEntry) ToTRDOSFileEntry() TRDOSFileEntry {
	return TRDOSFileEntry{
		Filename:       e.Filename,
		FileType:       e.FileType,
		StartAddress:   0, // Will be set by caller
		Length:         uint16(e.LengthSect) * 256,
		StartSector:    0, // Will be set by caller
		StartTrack:     0,  // Will be set by caller
	}
}

// LoadSCL loads an SCL format disk image using UnrealSpeccy's proven flexible approach
func LoadSCL(r io.Reader) (*Disk, error) {
	// Use the improved UnrealSpeccy-compatible implementation
	return LoadSCLUnreal(r)
}

// SaveSCL saves a disk in SCL format
func SaveSCL(w io.Writer, disk *Disk) error {
	// This would require scanning the TRD disk structure and extracting
	// files directory entries and data into SCL format
	// Implementation would be the reverse of LoadSCL
	return fmt.Errorf("SCL saving not yet implemented")
}
