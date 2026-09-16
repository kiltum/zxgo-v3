// Package ui holds the display interface and the parts of presentation that do
// not belong to any particular backend. Only the screenshot writer lives here
// today: it is backend-independent (the framebuffer is the same ARGB8888 slice
// whichever UI is driving it) and worth testing without a window.
package ui

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"time"
)

// ScreenshotPrefix is the base name of a saved screenshot.
const ScreenshotPrefix = "screenshot"

// ScreenshotStamp is the timestamp layout inside a screenshot's file name. It
// carries milliseconds so that pressing the shortcut repeatedly produces one
// file per press: a second-resolution stamp would silently overwrite when two
// presses land in the same second.
const ScreenshotStamp = "20060102-150405.000"

// SavePNG writes a framebuffer to a timestamped PNG in dir and returns the path
// it wrote. screen is the UI's ARGB8888 framebuffer (0xAARRGGBB), w by h
// pixels; no scaling or cropping is done, so what is saved is the whole
// rendered picture, borders included.
func SavePNG(dir string, screen []uint32, w, h int) (string, error) {
	if w <= 0 || h <= 0 {
		return "", fmt.Errorf("screenshot: bad size %dx%d", w, h)
	}
	if len(screen) < w*h {
		return "", fmt.Errorf("screenshot: %dx%d needs %d pixels, have %d", w, h, w*h, len(screen))
	}

	// NRGBA, not RGBA: the framebuffer holds straight (non-premultiplied)
	// ARGB, and image.RGBA would write those channels as if they were already
	// multiplied by alpha - invisible while every pixel is opaque, wrong the
	// moment one is not.
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			px := screen[y*w+x]
			i := img.PixOffset(x, y)
			img.Pix[i+0] = uint8(px >> 16) // R
			img.Pix[i+1] = uint8(px >> 8)  // G
			img.Pix[i+2] = uint8(px)       // B
			img.Pix[i+3] = uint8(px >> 24) // A
		}
	}

	path := filepath.Join(dir, ScreenshotPrefix+"-"+time.Now().Format(ScreenshotStamp)+".png")
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return "", err
	}
	return path, nil
}
