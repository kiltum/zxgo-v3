// Package archive opens emulator images that may be packed in a .zip.
//
// Releases ship zipped as often as not (testdata holds .tzx.zip, .tap.zip and
// friends), and unpacking by hand before every run is a chore with no upside.
// Open hands the caller a reader and the name to detect the format from, so a
// loader does not care whether the file arrived packed or not.
package archive

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Extensions are the entry names worth extracting from an archive, in the order
// they win when an archive holds more than one. The order is what decides the
// pick: a release shipping "game.tzx" beside a "readme.txt" loads the tape, and
// one shipping an image we cannot load yet still picks the image, so the error
// names the format rather than a stray text file.
//
// A .zip inside a .zip is not unpacked twice: one layer is what releases
// actually ship.
var Extensions = []string{
	".tzx", ".tap", // tape
	".trd", ".trd0", ".trd1", ".scl", ".dsk", ".edsk", // disk
	".sna", ".z80", // snapshot
	".rom", // ROM
	// Recognised but not loadable yet (no Disciple/+D controller, no UDI/FDI
	// support, no BLK/SZX parser): listed so they beat a readme in the pick, not
	// so they load -- doing so gives the loader's "unknown format" error instead
	// of a text file that fails somewhere deeper.
	".mgt", ".img", ".udi", ".fdi", ".blk", ".szx",
}

// Open opens path for loading. If it is a .zip, the loadable entry inside is
// opened instead and the returned name is that entry's name (so format
// detection, which keys on the extension, sees ".tzx" rather than ".zip").
// Anything else is opened directly and the name is the path.
//
// The returned reader owns the underlying file: the caller must Close it.
func Open(path string) (io.ReadCloser, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	if !strings.EqualFold(filepath.Ext(path), ".zip") {
		return f, path, nil
	}

	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, "", err
	}
	zr, err := zip.NewReader(io.NewSectionReader(f, 0, st.Size()), st.Size())
	if err != nil {
		f.Close()
		return nil, "", fmt.Errorf("reading zip %s: %w", path, err)
	}
	entry := pick(zr.File)
	if entry == nil {
		f.Close()
		return nil, "", fmt.Errorf("%s: no loadable file inside (%d entries)", path, len(zr.File))
	}
	rc, err := entry.Open()
	if err != nil {
		f.Close()
		return nil, "", fmt.Errorf("opening %s from %s: %w", entry.Name, path, err)
	}
	// The entry's reader does not own the file it was inflated from, so the two
	// closes are paired here: without this the caller would leak the descriptor,
	// and returning the entry reader alone after closing the file -- which this
	// used to do -- hands back a reader that fails on its first byte.
	return &zipReader{ReadCloser: rc, file: f}, entry.Name, nil
}

// zipReader pairs an archive entry's reader with the file the archive was read
// from, so one Close releases both.
type zipReader struct {
	io.ReadCloser
	file *os.File
}

func (z *zipReader) Close() error {
	err := z.ReadCloser.Close()
	if cerr := z.file.Close(); cerr != nil && err == nil {
		err = cerr
	}
	return err
}

// pick chooses the entry to load: the first with an extension we understand, and
// failing that the first regular file, so an archive holding something
// unexpected still gives a clear error from the loader rather than from here.
func pick(entries []*zip.File) *zip.File {
	for _, ext := range Extensions {
		for _, e := range entries {
			if !e.FileInfo().IsDir() && strings.EqualFold(filepath.Ext(e.Name), ext) {
				return e
			}
		}
	}
	for _, e := range entries {
		if !e.FileInfo().IsDir() {
			return e
		}
	}
	return nil
}
