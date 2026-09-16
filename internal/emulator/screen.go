package emulator

import (
	"strings"

	"github.com/kiltum/zxgo-v3/pkg/rom"
)

// ScreenText reconstructs the 32x24 text grid from the display file and the
// ROM font. It returns 24 newline-terminated rows of 32 characters each.
// Characters whose 8x8 glyph does not match any ROM-font glyph (block graphics,
// user-defined graphics) are rendered as '?'.
func (e *Emulator) ScreenText() string {
	m := e.Mapper()

	// ROM font: 96 glyphs for ASCII 32-127, 8 bytes each, at 0x3D00. The font
	// is identical across models, but the 128K editor ROM has code at 0x3D00
	// (the font lives in the menu ROM), so read it from the embedded 48K ROM
	// rather than the currently paged bank.
	fontROM := rom.Get("48k/48.rom")
	var font [96][8]byte
	for i := 0; i < 96; i++ {
		off := 0x3D00 + i*8
		copy(font[i][:], fontROM[off:off+8])
	}

	var sb strings.Builder
	for row := 0; row < 24; row++ {
		third := row / 8
		within := row % 8
		for col := 0; col < 32; col++ {
			// The 8 scanlines of a character cell are 256 bytes apart, and the
			// 32 columns are contiguous within a scanline (see CLAUDE.md / the
			// standard ZX Spectrum display-file layout).
			base := 0x4000 + third*0x800 + within*0x20 + col
			var glyph [8]byte
			for j := 0; j < 8; j++ {
				glyph[j] = m.ReadByte(uint16(base + j*0x100))
			}
			ch := byte('?')
			for i := 0; i < 96; i++ {
				if font[i] == glyph {
					ch = byte(i + 32)
					break
				}
			}
			sb.WriteByte(ch)
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// ScreenASCII downsamples the framebuffer to a coarse ASCII brightness map, for
// a quick sense of the screen layout without a display.
func (e *Emulator) ScreenASCII() string {
	scr := e.ULA().GetScreen()
	w := e.ULA().Width()
	h := e.ULA().Height()
	if w == 0 || h == 0 || len(scr) < w*h {
		return ""
	}

	const cols = 96
	rows := cols * h / w / 2 // halve for the 2:1 pixel aspect
	if rows < 1 {
		rows = 1
	}

	const ramp = " .:-=+*#%@"
	var sb strings.Builder
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			x := c * w / cols
			y := r * h / rows
			p := scr[y*w+x]
			lum := (int(p>>16&0xFF) + int(p>>8&0xFF) + int(p&0xFF)) / 3
			sb.WriteByte(ramp[lum*(len(ramp)-1)/255])
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}
