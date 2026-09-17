package media

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/kiltum/zxgo-v3/pkg/archive"
)

// LoadDiskFile loads a disk image from a path, unpacking a .zip on the way.
//
// The format comes from the name the archive reports, not from the path the
// caller passed: a zipped image is detected by the name it has *inside*
// (.trd/.scl/.dsk), which is the whole point of archive.Open handing the name
// back. One dispatcher serves every caller -- cmd/zxgo and the MCP worker used
// to carry their own copies of this switch, which is exactly the kind of
// duplication where one copy gains a format and the other does not.
func LoadDiskFile(path string) (*Disk, error) {
	f, name, err := archive.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	disk, err := decodeDisk(f, name)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return disk, nil
}

// decodeDisk picks the loader by extension. name is the archive entry name, or
// the plain path when nothing was packed.
func decodeDisk(r io.Reader, name string) (*Disk, error) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".trd", ".trd0", ".trd1":
		return LoadTRDisk(r)
	case ".scl":
		return LoadSCL(r)
	case ".dsk", ".edsk":
		return LoadDSK(r)
	default:
		return nil, fmt.Errorf("unknown disk image format %q (supported: .trd .scl .dsk)",
			filepath.Ext(name))
	}
}

// LoadTapeFile loads a tape image from a path, unpacking a .zip on the way.
// See LoadDiskFile for why the format comes from the returned name; the tape's
// own file name is that entry name too, since it is the file that was read.
func LoadTapeFile(path string) (*Tape, error) {
	f, name, err := archive.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var tape *Tape
	switch strings.ToLower(filepath.Ext(name)) {
	case ".tap":
		tape, err = LoadTAP(f, name)
	case ".tzx":
		tape, err = LoadTZX(f, name)
	default:
		return nil, fmt.Errorf("unknown tape format %q (supported: .tap .tzx)", filepath.Ext(name))
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return tape, nil
}
