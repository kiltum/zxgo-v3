package emulator

import (
	"os"
	"testing"

	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/sound"
)

// newTestMachine builds the machine the CLI builds, without a window or a session.
func newTestMachine(t *testing.T, key string) *Emulator {
	t.Helper()
	cfg, ok := model.AllModels[key]
	if !ok {
		t.Fatalf("no model %q", key)
	}
	emu, err := NewFromModel(cfg, "roms", &sound.NullOutput{})
	if err != nil {
		t.Fatalf("building the machine: %v", err)
	}
	return emu
}

// A disk mounts from a path, reports itself, and comes out again: the whole path a
// disks window takes, through the loaders the command line uses so that the two
// cannot disagree about what a file is.
func TestMountDiskFromPathAndEject(t *testing.T) {
	const path = "../../testdata/1.scl"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("testdata %s not available: %v", path, err)
	}

	emu := newTestMachine(t, "48k")

	// A machine with nothing mounted still has a controller: every model builds one at
	// construction, because a machine with no disk has a controller that reports "no
	// disk" rather than a hole where the ports should be (emulator_layout.go).
	//
	// What the empty drive changes is the medium, which a window has to be able to show
	// without treating it as an error.
	empty, has := emu.FDC()
	if !has {
		t.Fatal("a machine reports no floppy controller at all")
	}
	if empty.Ready {
		t.Error("a machine with no disk mounted reports one")
	}
	if empty.Controller != "WD1793" {
		t.Errorf("controller = %q, want the Beta Disk", empty.Controller)
	}
	if _, mounted := emu.DiskInfo(); mounted {
		t.Error("a machine with no disk mounted reports one")
	}
	if emu.DiskPath() != "" {
		t.Errorf("disk path = %q with nothing mounted", emu.DiskPath())
	}

	info, err := emu.MountDiskFromPath(path)
	if err != nil {
		t.Fatalf("MountDiskFromPath: %v", err)
	}
	if info.Type != "SCL" && info.Type != "TRD" {
		t.Errorf("disk type = %q, want a TR-DOS image", info.Type)
	}
	if info.Tracks == 0 || info.SectorsPerTrack == 0 {
		t.Errorf("the disk reports no geometry: %+v", info)
	}

	fdc, has := emu.FDC()
	if !has {
		t.Fatal("mounting a disk did not add a controller")
	}
	if !fdc.Ready {
		t.Error("the controller reports no disk after a mount")
	}
	if fdc.Controller != "WD1793" {
		t.Errorf("controller = %q, want the Beta Disk", fdc.Controller)
	}
	if fdc.Disk.Name != info.Name {
		t.Errorf("the controller has %q, the mount reported %q", fdc.Disk.Name, info.Name)
	}
	if got := emu.DiskPath(); got != path {
		t.Errorf("disk path = %q, want %q", got, path)
	}

	// Eject: the disk goes, the controller stays, and the machine keeps running.
	emu.EjectDisk()
	fdc, has = emu.FDC()
	if !has {
		t.Error("ejecting took the controller with it")
	}
	if fdc.Ready {
		t.Error("the controller still reports a disk after an eject")
	}
	if _, mounted := emu.DiskInfo(); mounted {
		t.Error("a disk is still reported after an eject")
	}
	if emu.DiskPath() != "" {
		t.Errorf("disk path = %q after an eject", emu.DiskPath())
	}
}

// A file that is not a disk image is refused and nothing is mounted: a window that
// showed a disk the machine does not have would be worse than one that showed none.
func TestMountDiskFromPathRefusesRubbish(t *testing.T) {
	emu := newTestMachine(t, "48k")

	f, err := os.CreateTemp(t.TempDir(), "notadisk*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("this is not a disk image"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	if _, err := emu.MountDiskFromPath(f.Name()); err == nil {
		t.Error("a text file was accepted as a disk image")
	}
	// Nothing was mounted: the controller is there, as it always is, and its drive is
	// empty.
	fdc, has := emu.FDC()
	if !has {
		t.Error("the controller went away with the refused mount")
	}
	if fdc.Ready {
		t.Error("a refused mount left a disk in the drive")
	}
	if emu.DiskPath() != "" {
		t.Errorf("disk path = %q after a refused mount", emu.DiskPath())
	}
}

// The +2A/+3 has *two* controllers - a uPD765 built with the machine and the Beta Disk
// every model gets - and a mount goes to the uPD765, which is the one whose ports the
// +3 DOS uses.
func TestMountDiskOnThePlus3(t *testing.T) {
	const path = "../../testdata/Tetris (1988)(Mirrorsoft)(+3).dsk"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("testdata %s not available: %v", path, err)
	}

	emu := newTestMachine(t, "2a3")
	if _, has := emu.FDC(); !has {
		t.Error("a +3 reports no floppy controller before a disk is mounted")
	}

	if _, err := emu.MountDiskFromPath(path); err != nil {
		t.Fatalf("MountDiskFromPath: %v", err)
	}
	fdc, _ := emu.FDC()
	if fdc.Controller != "uPD765" {
		t.Errorf("controller = %q, want the uPD765", fdc.Controller)
	}
	if !fdc.Ready {
		t.Error("no disk reported after mounting one in a +3")
	}
}
