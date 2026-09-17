package archive

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// writeZip builds a zip in a temp dir and returns its path.
func writeZip(t *testing.T, name string, entries map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	// Deterministic order so the "first regular file" fallback is testable.
	names := make([]string, 0, len(entries))
	for k := range entries {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, entry := range names {
		w, err := zw.Create(entry)
		if err != nil {
			t.Fatalf("create entry %s: %v", entry, err)
		}
		if _, err := w.Write([]byte(entries[entry])); err != nil {
			t.Fatalf("write entry %s: %v", entry, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return path
}

// TestOpenPlainFile: a file that is not a zip is handed back as-is, and the name
// is the path, so extension-based detection keeps working.
func TestOpenPlainFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "game.tap")
	if err := os.WriteFile(path, []byte("TAPDATA"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	r, name, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	if name != path {
		t.Errorf("name = %q, want the path %q", name, path)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "TAPDATA" {
		t.Errorf("data = %q, want TAPDATA", data)
	}
}

// TestOpenZipReturnsEntryName is the contract the loaders rely on: the name that
// comes back is the entry's, so ".zip" never reaches a format switch.
func TestOpenZipReturnsEntryName(t *testing.T) {
	path := writeZip(t, "game.zip", map[string]string{"Exolon.tzx": "TZXDATA"})

	r, name, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	if filepath.Ext(name) != ".tzx" {
		t.Errorf("name = %q, want a .tzx entry name", name)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "TZXDATA" {
		t.Errorf("data = %q, want TZXDATA", data)
	}
}

// TestPickPrefersLoadableExtension: a release shipping its tape beside a readme
// must load the tape, whatever order the archive lists them in.
func TestPickPrefersLoadableExtension(t *testing.T) {
	path := writeZip(t, "game.zip", map[string]string{
		"README.txt":     "not a tape",
		"Exolon.tzx":     "TZXDATA",
		"notes/info.txt": "also not a tape",
	})

	r, name, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	if name != "Exolon.tzx" {
		t.Errorf("picked %q, want Exolon.tzx", name)
	}
}

// TestPickFallsBackToFirstFile: an archive holding nothing we recognise still
// yields a file, so the loader reports an unknown format rather than an empty
// read.
func TestPickFallsBackToFirstFile(t *testing.T) {
	path := writeZip(t, "game.zip", map[string]string{"a.txt": "one", "b.txt": "two"})

	r, name, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	if name == "" || filepath.Ext(name) != ".txt" {
		t.Errorf("picked %q, want a .txt fallback", name)
	}
}

// TestPickSkipsDirectories: a zip whose first entry is a directory must not
// hand back a directory as the payload.
func TestPickSkipsDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "game.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	zw := zip.NewWriter(f)
	if _, err := zw.Create("Exolon/"); err != nil {
		t.Fatalf("create dir: %v", err)
	}
	w, err := zw.Create("Exolon/game.z80")
	if err != nil {
		t.Fatalf("create entry: %v", err)
	}
	if _, err := w.Write([]byte("SNAP")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}

	r, name, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	if name != "Exolon/game.z80" {
		t.Errorf("picked %q, want Exolon/game.z80", name)
	}
}

// TestOpenEmptyZip: no entry to pick is an error naming the archive, not a nil
// reader the caller would trip over.
func TestOpenEmptyZip(t *testing.T) {
	path := writeZip(t, "empty.zip", map[string]string{})

	r, _, err := Open(path)
	if err == nil {
		r.Close()
		t.Fatal("expected an error for an archive with nothing loadable inside")
	}
}

// TestOpenNotAZip: a .zip that is not actually a zip reports the archive, and
// does not leak the file handle.
func TestOpenNotAZip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.zip")
	if err := os.WriteFile(path, []byte("this is not a zip file"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if r, _, err := Open(path); err == nil {
		r.Close()
		t.Fatal("expected an error for a corrupt archive")
	}
}

// TestOpenCaseInsensitiveExtension: releases use ".ZIP" as often as ".zip".
func TestOpenCaseInsensitiveExtension(t *testing.T) {
	path := writeZip(t, "game.ZIP", map[string]string{"Tape.TAP": "TAPDATA"})

	r, name, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	if name != "Tape.TAP" {
		t.Errorf("picked %q, want Tape.TAP", name)
	}
}

// TestOpenMissingFile reports the failure rather than an empty reader.
func TestOpenMissingFile(t *testing.T) {
	if r, _, err := Open(filepath.Join(t.TempDir(), "nope.zip")); err == nil {
		r.Close()
		t.Fatal("expected an error for a missing file")
	}
}

// TestExtensionsCoverLoaders guards the list against a loader gaining a format
// that the pick order does not know about: a ".rom" archive holding one would
// otherwise fall back to the first regular file.
func TestExtensionsCoverLoaders(t *testing.T) {
	entries := []string{"a.trd", "a.trd0", "a.trd1", "a.scl", "a.dsk", "a.edsk", "a.tap", "a.tzx", "a.sna", "a.z80", "a.rom"}
	for _, entry := range entries {
		ext := filepath.Ext(entry)
		found := false
		for _, known := range Extensions {
			if known == ext {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("extension %s is not in Extensions", ext)
		}
	}
}

// TestOpenZipStreams a large entry back whole: the reader is streamed out of the
// archive rather than truncated or buffered away.
func TestOpenZipStreams(t *testing.T) {
	payload := bytes.Repeat([]byte("T"), 1<<20)
	path := writeZip(t, "big.zip", map[string]string{"big.tap": string(payload)})

	r, name, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(data) != len(payload) || name != "big.tap" {
		t.Fatalf("read %d bytes from %q, want %d from big.tap", len(data), name, len(payload))
	}
}
