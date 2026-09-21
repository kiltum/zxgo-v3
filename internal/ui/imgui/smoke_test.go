package imgui

import (
	"strings"
	"testing"

	"github.com/kiltum/zxgo-v3/internal/frontend"
	"github.com/kiltum/zxgo-v3/pkg/media"
)

// The ImGui path has to boot with no display attached, which is what CI has. The
// dummy driver gives SDL a real window and a real renderer without a server, so
// this exercises the whole path - context per window, the frame, the screen upload,
// the menubar - and not just the C++ linking.
func TestBackendBootsHeadless(t *testing.T) {
	t.Setenv("SDL_VIDEODRIVER", "dummy")

	b := New()
	b.Vsync = false // nothing to synchronise with, and CI has no vblank
	b.IniDir = t.TempDir()

	if err := b.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer b.Destroy()

	if w, h := b.MainWindowSize(); w <= 0 || h <= 0 {
		t.Errorf("main window size = %dx%d, want something", w, h)
	}
	if displays := b.Displays(); len(displays) == 0 {
		t.Error("no displays reported, so a restored rect could not be clamped")
	}

	// A frame of the main window: menubar with all four menus, the status read-out
	// and a screen. A screen of 2x2 is enough - the point is that the upload and
	// the draw run without a display.
	app := frontend.New(nil)
	_ = app
	pix := []uint32{0xFF000000, 0xFFFF0000, 0xFF00FF00, 0xFF0000FF}
	b.PresentMain(frontend.MainView{
		Screen:      frontend.Screen{W: 2, H: 2, Pix: pix},
		Menu:        menuForTest(),
		Status:      "ZX Spectrum 48K | 50.0 fps",
		MenuVisible: true,
	})

	// A tool window: opened through the wanted set, then drawn.
	b.SyncWindows(frontend.WindowsView{
		Main:  frontend.WindowState{ID: frontend.ToolMain, Open: true, Rect: frontend.Rect{W: 768, H: 576}},
		Tools: []frontend.WindowState{{ID: frontend.ToolTape, Open: true, Rect: frontend.Rect{X: 900, Y: 100, W: 440, H: 320}}},
	})
	b.PresentTool(frontend.ToolTape, frontend.Views{})

	var events []frontend.Event
	if !b.Poll(&events) {
		t.Error("Poll reported a quit with nothing asking to quit")
	}
}

// Every tool window has to draw without a display, and the control window is the one
// with contents: buttons whose labels follow the run state, and read-outs.
//
// A frame is also where a button press becomes an event, so this checks the other
// direction too: a window that draws but cannot report what was pressed is a window
// with a picture of a button in it.
func TestToolWindowsDrawAndReport(t *testing.T) {
	t.Setenv("SDL_VIDEODRIVER", "dummy")

	b := New()
	b.Vsync = false
	if err := b.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer b.Destroy()

	for _, id := range frontend.Tools() {
		b.SyncWindows(frontend.WindowsView{
			Main:  frontend.WindowState{ID: frontend.ToolMain, Open: true, Rect: frontend.Rect{W: 320, H: 240}},
			Tools: []frontend.WindowState{{ID: id, Open: true, Rect: frontend.Rect{X: 400, Y: 60, W: 420, H: 380}}},
		})

		views := frontend.Views{
			Status: frontend.StatusView{
				Run: frontend.StatePaused, Model: "ZX Spectrum 48K", CPUHz: 3500000,
				Ticks: 1234567, Frames: 17, FPS: 50.0,
				// On, so the tape window draws the Turbo button in its "on" state and
				// not only its "off" one.
				FastTape: true,
			},
			// A disks window with a disk in it and a controller doing something, so
			// the read-outs and the status lines are drawn rather than only the empty
			// case.
			Disks: frontend.DiskView{
				Mounted: true,
				Info: media.DiskInfo{
					Name: "ZXGO DISK", Type: media.DiskTypeTRD, Tracks: 80, Sides: 2,
					SectorsPerTrack: 16, SizeKB: 640, FreeSectors: 1200, FileCount: 7,
				},
				Path:   "/disks/game.trd",
				HasFDC: true,
				FDC: media.FDCState{
					Controller: "WD1793", Command: "READ SECTOR", Ready: true,
					Drive: 0, Track: 2, Sector: 3, Busy: true, DRQ: true, IndexPhase: 0.25,
				},
			},
			// A keyboard window with a key held: the held colour and a label carrying
			// all four legends are part of the drawing, and an idle keyboard draws
			// neither.
			Keyboard: frontend.KeyboardView{
				Down: map[frontend.Cell]bool{{Row: 2, Col: 0}: true},
			},
			// A tape window with a tape in it, so the drawing that names the current
			// block and lists the rest is exercised rather than only the empty case.
			Tape: media.TapeState{
				FileName: "game.tap", Format: "TAP", Playing: true,
				Pos: 400, Pulses: 1000, Percent: 0.4, Current: 1,
				Blocks: []media.TapeBlock{
					{Index: 0, Symbol: "P", Name: "BATTLECITY", Bytes: 19, StartsAt: 0},
					{Index: 1, Symbol: "d", Bytes: 49152, StartsAt: 300, Current: true},
				},
			},
			// A settings window with a change waiting for a relaunch, so the restart badge,
			// the row marked as not yet in force and the relaunch button are drawn. The rows
			// are plain data, so both kinds of switch - a row the machine agrees with and one
			// it does not - are here rather than only whichever one a real file would
			// produce, and both CPU rows are present so the radio drawing is exercised.
			Settings: frontend.SettingsView{
				Radios: []frontend.RadioGroup{
					{
						Label:   "Machine",
						Options: frontend.ModelChoices(frontend.Settings{Model: "pentagon"}, "48k"),
						Note:    "Pentagon 128 - applies at the next launch.",
					},
					{
						Label:   "Z80",
						Options: frontend.CPUChoices(true),
						Note:    "Applies now.",
					},
					{
						Label:   "MEMPTR on the repeating block I/O",
						Options: frontend.MEMPTRChoices(false),
					},
				},
				Toggles: []frontend.ToggleGroup{
					{
						Label: "Sound",
						Rows: []frontend.Toggle{
							{Label: "TurboSound", Action: frontend.ActTurboSoundToggle, On: false, Running: true},
							{Label: "Snow", Action: frontend.ActSnowToggle, On: false, Running: false},
						},
						Note: "These apply at the next launch.",
					},
					{
						Label: "Speed",
						Rows: []frontend.Toggle{
							{Label: "Fast tape (turbo)", Action: frontend.ActTurboToggle, On: true, Running: true},
						},
					},
				},
				Restart: true,
			},
		}
		b.PresentTool(id, views)

		// And again with the controller idle, so the branches that only appear when
		// nothing is happening - the "idle" line where the status flags would be, and
		// no rotation read-out - are drawn too. The settings window is redrawn with
		// nothing pending for the same reason: its badge and its relaunch button are
		// two more branches.
		idle := views
		idle.Disks.FDC.Busy = false
		idle.Disks.FDC.DRQ = false
		idle.Settings.Restart = false
		b.PresentTool(id, idle)

		// Nothing pressed: no events beyond what the drawing itself may have made
		// (there should be none, since ImGui only reports a click).
		var events []frontend.Event
		b.Poll(&events)
		for _, ev := range events {
			if ev.Kind == frontend.EvAction {
				t.Errorf("%v reported an action (%v) with nothing clicked", id, ev.Action)
			}
		}
	}
}

// menuForTest is a small menubar: one plain item, one checked item and a separator,
// which is every kind the drawing code branches on.
func menuForTest() []frontend.Menu {
	return []frontend.Menu{{
		Label: "File",
		Items: []frontend.MenuItem{
			{Label: "Quit", Action: frontend.ActQuit, Enabled: true, Shortcut: "cmd+q"},
			{Kind: frontend.MenuItemSeparator},
			{Label: "Tape", Action: frontend.ActToggleTape, Kind: frontend.MenuItemCheck, Checked: true, Enabled: true},
		},
	}}
}

// The label on an on-screen key is the key as it is printed on the machine: the keyword
// CAPS SHIFT gives above, the key with its SYMBOL SHIFT legend beside it, and the
// keywords both shifts give below. A key with nothing on a line has no line, which is
// what makes four legends fit on a button.
func TestKeyLabelCarriesTheLegends(t *testing.T) {
	q, ok := frontend.ZXKeyAt(2, 0)
	if !ok {
		t.Fatal("half-row 2 bit 0 is not a key")
	}
	got := keyLabel(q)
	for _, want := range []string{"PLOT", "Q", "<=", "SIN ASN"} {
		if !strings.Contains(got, want) {
			t.Errorf("the label of Q is %q, and does not carry %q", got, want)
		}
	}

	// A key with only its own legend gets one line.
	caps, _ := frontend.ZXKeyAt(0, 0)
	if got := keyLabel(caps); got != "CAPS SHIFT" {
		t.Errorf("the label of CAPS SHIFT is %q, want just its name", got)
	}

	// And the space bar carries its BREAK, which is the legend that is easiest to lose.
	space, _ := frontend.ZXKeyAt(7, 0)
	if got := keyLabel(space); !strings.Contains(got, "BREAK") || !strings.Contains(got, "SPACE") {
		t.Errorf("the label of SPACE is %q, want its BREAK too", got)
	}
}
