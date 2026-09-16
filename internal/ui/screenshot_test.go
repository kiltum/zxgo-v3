package ui

import (
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A saved screenshot must be a PNG of the whole framebuffer with the channels
// in the right order - the framebuffer is ARGB8888, PNG wants RGBA, and getting
// that wrong gives a picture that still decodes but with red and blue swapped.
func TestSavePNGChannelsAndSize(t *testing.T) {
	dir := t.TempDir()
	screen := []uint32{
		0xFF112233, 0x80445566, 0x00000000, 0xFFFFFFFF,
		0x01020304, 0x05060708, 0x090A0B0C, 0x0D0E0F10,
	}
	path, err := SavePNG(dir, screen, 4, 2)
	if err != nil {
		t.Fatalf("SavePNG: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if img.Bounds().Dx() != 4 || img.Bounds().Dy() != 2 {
		t.Fatalf("size = %v, want 4x2", img.Bounds())
	}

	// Every pixel, taken from the source rather than a hand-written table: a
	// typo in the table would look exactly like an encoder bug, and getting the
	// channel order wrong is the mistake this is here to catch (the framebuffer
	// is ARGB8888, PNG wants RGBA).
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			px := screen[y*4+x]
			want := color.NRGBA{R: uint8(px >> 16), G: uint8(px >> 8), B: uint8(px), A: uint8(px >> 24)}
			got := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if got != want {
				t.Errorf("pixel (%d,%d) = %v, want %v", x, y, got, want)
			}
		}
	}
}

// Two presses must be two files, which is the whole reason the name carries a
// timestamp.
func TestSavePNGRepeatedPressesMakeSeparateFiles(t *testing.T) {
	dir := t.TempDir()
	screen := make([]uint32, 4)
	for i := 0; i < 2; i++ {
		if _, err := SavePNG(dir, screen, 2, 2); err != nil {
			t.Fatalf("SavePNG %d: %v", i, err)
		}
		time.Sleep(2 * time.Millisecond) // the stamp has millisecond resolution
	}
	names, err := filepath.Glob(filepath.Join(dir, "*.png"))
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Errorf("saved %d files (%v), want 2", len(names), names)
	}
}

// A framebuffer that does not match the claimed size is a caller bug, not a
// truncated image.
func TestSavePNGRejectsBadSize(t *testing.T) {
	dir := t.TempDir()
	if _, err := SavePNG(dir, make([]uint32, 4), 4, 4); err == nil {
		t.Error("want an error for a 4x4 claim on 4 pixels")
	}
	if _, err := SavePNG(dir, nil, 0, 0); err == nil {
		t.Error("want an error for a zero size")
	}
}
