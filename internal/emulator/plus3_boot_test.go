package emulator

import (
	"os"
	"strings"
	"testing"

	"github.com/kiltum/zxgo-v3/pkg/media"
	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/sound"
)

// typeKey drives one ZX keyboard matrix key with a press/release and a few
// frames in between, so the ROM key-scan sees both edges.
func typeKey(e *Emulator, row, col int) {
	e.PressKey(row, col)
	for i := 0; i < 3; i++ {
		e.RunFrame()
	}
	e.ReleaseKey(row, col)
	for i := 0; i < 3; i++ {
		e.RunFrame()
	}
}

// typeCapsShifted presses CAPS SHIFT together with another matrix key (used for
// the +3 menu cursor keys: CAPS SHIFT + 6 = down, +7 = up).
func typeCapsShifted(e *Emulator, row, col int) {
	e.PressKey(0, 0) // CAPS SHIFT
	e.PressKey(row, col)
	for i := 0; i < 3; i++ {
		e.RunFrame()
	}
	e.ReleaseKey(row, col)
	e.ReleaseKey(0, 0)
	for i := 0; i < 3; i++ {
		e.RunFrame()
	}
}

// typeTextPlus3 types an ASCII string through the +3 keyboard matrix. Letters
// are unshifted (lowercase in the editor); ENTER is newline.
func typeTextPlus3(e *Emulator, s string) {
	key := map[byte][2]int{
		'a': {1, 0}, 'b': {7, 4}, 'c': {0, 3}, 'd': {1, 2}, 'e': {2, 2},
		'f': {1, 3}, 'g': {1, 4}, 'h': {6, 4}, 'i': {5, 2}, 'j': {6, 3},
		'k': {6, 2}, 'l': {6, 1}, 'm': {7, 2}, 'n': {7, 3}, 'o': {5, 1},
		'p': {5, 0}, 'q': {2, 0}, 'r': {2, 3}, 's': {1, 1}, 't': {2, 4},
		'u': {5, 3}, 'v': {0, 4}, 'w': {2, 1}, 'x': {0, 2}, 'y': {5, 4},
		'z': {0, 1}, '\n': {6, 0},
	}
	for i := 0; i < len(s); i++ {
		if rc, ok := key[s[i]]; ok {
			typeKey(e, rc[0], rc[1])
		}
	}
}

// TestPlus3BootAndCat boots the +3 with a DSK image and runs the CAT command,
// verifying the uPD765 controller and +3 paging cooperate enough to list the
// disk directory. This is an end-to-end gate for the +3 FDC port decode, the
// command state machine, READ ID, and the DSK parser.
//
// It runs against two disks: one with sector IDs 01-09 and one with C1-C9, so
// the drive-geometry detection (READ ID) and sector mapping cover both
// conventions.
func TestPlus3BootAndCat(t *testing.T) {
	disks := []string{
		"Alien Storm (1991)(U.S. Gold)(+3).dsk", // 40/2/9, sector IDs 01-09
		"Tetris (1988)(Mirrorsoft)(+3).dsk",     // 40/1/9, sector IDs C1-C9
	}
	for _, name := range disks {
		name := name
		t.Run(strings.ReplaceAll(name, ".dsk", ""), func(t *testing.T) {
			runPlus3Cat(t, name)
		})
	}
}

func runPlus3Cat(t *testing.T, diskName string) {
	t.Helper()
	e, err := NewFromModel(model.Spectrum2A3, "", &sound.NullOutput{})
	if err != nil {
		t.Fatalf("NewFromModel: %v", err)
	}

	f, err := os.Open("../../testdata/" + diskName)
	if err != nil {
		t.Skipf("no +3 DSK test image: %v", err)
	}
	defer f.Close()
	disk, err := media.LoadDSK(f)
	if err != nil {
		t.Fatalf("LoadDSK: %v", err)
	}
	if err := e.LoadDisk(disk); err != nil {
		t.Fatalf("LoadDisk: %v", err)
	}

	// Boot for ~3 seconds.
	for i := 0; i < 150; i++ {
		e.RunFrame()
	}

	// The boot menu starts with "Loader" highlighted. Move the cursor down one
	// (CAPS SHIFT + 6) to "+3 BASIC", then select it with ENTER.
	typeCapsShifted(e, 4, 4) // cursor down
	typeKey(e, 6, 0)         // ENTER (select +3 BASIC)
	for i := 0; i < 50; i++ {
		e.RunFrame()
	}

	// Type CAT + ENTER, then run for ~4 seconds.
	typeTextPlus3(e, "cat\n")
	for i := 0; i < 200; i++ {
		e.RunFrame()
	}
	after := e.ScreenText()
	t.Logf("after CAT:\n%s", after)

	// A working directory listing shows a filename and no disk error.
	for _, errMsg := range []string{"bad format", "no data", "unknown error", "unsuitable", "seek fail"} {
		if strings.Contains(after, errMsg) {
			t.Fatalf("CAT hit disk error %q; screen:\n%s", errMsg, after)
		}
	}
	if !strings.Contains(after, ".") {
		t.Fatalf("CAT did not list the directory; screen:\n%s", after)
	}
}

// TestPlus3Loader boots the +3 with a game disk and selects the "Loader" menu
// option. The +3DOS reads the boot sector (track 0, logical sector 0), checksums
// it, copies it to $FE00 and jumps to $FE10, which loads and runs the game. This
// is the standard way to load commercial software, so it gates the full
// boot-sector read + checksum + execute path (in addition to CAT above).
func TestPlus3Loader(t *testing.T) {
	e, err := NewFromModel(model.Spectrum2A3, "", &sound.NullOutput{})
	if err != nil {
		t.Fatalf("NewFromModel: %v", err)
	}

	f, err := os.Open("../../testdata/Tetris (1988)(Mirrorsoft)(+3).dsk")
	if err != nil {
		t.Skipf("no +3 DSK test image: %v", err)
	}
	defer f.Close()
	disk, err := media.LoadDSK(f)
	if err != nil {
		t.Fatalf("LoadDSK: %v", err)
	}
	if err := e.LoadDisk(disk); err != nil {
		t.Fatalf("LoadDisk: %v", err)
	}

	for i := 0; i < 150; i++ {
		e.RunFrame()
	}

	// "Loader" is the default highlighted menu option; ENTER selects it.
	typeKey(e, 6, 0)
	for i := 0; i < 400; i++ {
		e.RunFrame()
	}
	screen := e.ScreenText()
	t.Logf("after Loader:\n%s", screen)

	// The boot sector must checksum and execute: the game takes over the screen
	// (leaving the menu) and must not report a boot/disk error.
	if strings.Contains(screen, "bootable") {
		t.Fatalf("Loader reported a non-bootable disk; screen:\n%s", screen)
	}
	if strings.Contains(screen, "Retry") {
		t.Fatalf("Loader hit a disk error; screen:\n%s", screen)
	}
	if strings.Contains(screen, "Loader") || strings.Contains(screen, "Amstrad") {
		t.Fatalf("Loader did not leave the boot menu; screen:\n%s", screen)
	}
}
