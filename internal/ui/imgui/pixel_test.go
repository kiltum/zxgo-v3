package imgui

import (
	"bytes"
	"testing"
	"unsafe"

	"github.com/kiltum/zxgo-v3/internal/frontend"
)

// TestScreenIsPixelIdenticalAt1x is M2's third *done when*: at 1x what the ImGui
// path presents is the machine's framebuffer, pixel for pixel.
//
// It compares the rendered frame against the memory of the source slice rather than
// against a hand-written table of channels: the claim is "the same bytes", and
// writing the expected bytes out by hand would encode the test's own idea of the
// pixel format instead of checking the one that reaches the screen. Getting that
// wrong still produces a picture, with red and blue swapped.
//
// The window is sized to the picture so the integer scale is exactly 1, and the
// menubar is hidden so it takes none of the client area.
func TestScreenIsPixelIdenticalAt1x(t *testing.T) {
	t.Setenv("SDL_VIDEODRIVER", "dummy")

	b := New()
	b.Vsync = false
	if err := b.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer b.Destroy()

	// 32x32 is not a ZX picture, it is the smallest window ImGui will make: it
	// clamps a window to style.WindowMinSize (32x32 by default), so anything smaller
	// is grown behind the caller's back and the scale is no longer 1. Every pixel is
	// distinct, so a shift, a scale or a swap shows up as a mismatch.
	const w, h = 32, 32
	src := make([]uint32, w*h)
	for i := range src {
		src[i] = 0xFF000000 | uint32(i*2654435761)
		src[i] |= 0xFF000000
	}

	b.SyncWindows(frontend.WindowsView{
		Main: frontend.WindowState{ID: frontend.ToolMain, Open: true, Rect: frontend.Rect{W: w, H: h}},
	})

	if got := b.viewRect(frontend.ToolMain); got.W != w || got.H != h {
		t.Skipf("the dummy driver gave a %dx%d window, and this needs exactly %dx%d",
			got.W, got.H, w, h)
	}

	b.PresentMain(frontend.MainView{
		Screen:      frontend.Screen{W: w, H: h, Pix: src},
		MenuVisible: false,
	})

	got := make([]byte, w*h*4)
	if !b.readPixels(b.main, got, w, h) {
		t.Fatal("could not read the frame back")
	}

	// The framebuffer's own bytes, which is what "identical" has to mean: the
	// machine's picture is an ARGB8888 slice and the texture is ARGB8888, so the
	// two are the same 32-bit words in the same order.
	want := unsafe.Slice((*byte)(unsafe.Pointer(&src[0])), w*h*4)
	for i := 0; i < w*h; i++ {
		gotPx := got[i*4 : i*4+4]
		wantPx := want[i*4 : i*4+4]
		if !bytes.Equal(gotPx, wantPx) {
			t.Errorf("pixel %d (x=%d y=%d): got % x, want % x",
				i, i%w, i/w, gotPx, wantPx)
		}
	}
}

// TestMenuBarOverlaysWithoutMovingThePicture is the property the menubar has to have,
// and it was wrong twice before it was right.
//
// The bar must not take room from the screen. Reserving room means the picture is
// scaled to fit what is left, so showing the bar shrinks the picture and hiding it
// grows it back - and when the window is exactly an integer scale of the screen, the
// bar's nineteen pixels are the whole difference between 2x and 1x. An overlay leaves
// the picture where it is and covers its top edge instead, which for a ZX screen is
// border.
//
// The test renders the same frame twice, with the bar hidden and shown, and requires
// everything below the bar to be byte-identical.
func TestMenuBarOverlaysWithoutMovingThePicture(t *testing.T) {
	t.Setenv("SDL_VIDEODRIVER", "dummy")

	b := New()
	b.Vsync = false
	if err := b.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer b.Destroy()

	const scale = 2
	const w, h = 64, 32
	src := make([]uint32, w*h)
	for i := range src {
		src[i] = 0xFF000000 | uint32(i*2654435761)
	}
	winW, winH := w*scale, h*scale

	// Exactly what a first launch asks for: twice the screen, with nothing reserved.
	b.SyncWindows(frontend.WindowsView{
		Main: frontend.WindowState{ID: frontend.ToolMain, Open: true, Rect: frontend.Rect{W: winW, H: winH}},
	})
	if got := b.viewRect(frontend.ToolMain); got.W != winW || got.H != winH {
		t.Skipf("the dummy driver gave a %dx%d window, want %dx%d", got.W, got.H, winW, winH)
	}

	view := frontend.MainView{
		Screen: frontend.Screen{W: w, H: h, Pix: src},
		Menu:   menuForTest(),
		Status: "48K | 50.0 fps",
	}

	view.MenuVisible = false
	hidden := b.renderFrame(view, winW, winH)
	view.MenuVisible = true
	shown := b.renderFrame(view, winW, winH)

	rowBytes := winW * 4
	// The first row the two agree on: below the bar, and it must not be the top one,
	// or nothing was drawn.
	first := -1
	for row := 0; row < winH; row++ {
		if bytes.Equal(hidden[row*rowBytes:(row+1)*rowBytes], shown[row*rowBytes:(row+1)*rowBytes]) {
			first = row
			break
		}
	}
	if first <= 0 {
		t.Fatalf("the menubar changed nothing at the top of the window (first equal row %d)", first)
	}
	for row := first; row < winH; row++ {
		if !bytes.Equal(hidden[row*rowBytes:(row+1)*rowBytes], shown[row*rowBytes:(row+1)*rowBytes]) {
			t.Fatalf("row %d differs between the bar hidden and shown: the picture moved", row)
		}
	}

	// And the picture really is at 2x in both - unchanged is not enough, since two
	// equally wrong frames would also be unchanged. It is checked in the frame without
	// the bar, where nothing covers it: the bar sits over the picture's first rows, so
	// the pattern for row 0 is not visible in the other one.
	row := make([]byte, rowBytes)
	for x := 0; x < w; x++ {
		px := src[x]
		for d := 0; d < scale; d++ {
			i := (x*scale + d) * 4
			row[i+0] = byte(px)
			row[i+1] = byte(px >> 8)
			row[i+2] = byte(px >> 16)
			row[i+3] = byte(px >> 24)
		}
	}
	if !bytes.Equal(hidden[:rowBytes], row) {
		t.Error("the picture is not at the top of the window at 2x with the menubar hidden")
	}
}
