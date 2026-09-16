package media

import "testing"

// TestBetaDiskCommandDecode pins the WD179x command decode. WRITE SECTOR used
// to be unreachable (its guard "b&0xC0 == 0xA0" is unsatisfiable), so writes
// were executed as reads; and 0xE0-0xFF matched no case at all and left INTRQ
// low, which hangs TR-DOS.
func TestBetaDiskCommandDecode(t *testing.T) {
	for _, tc := range []struct {
		cmd  byte
		want string
	}{
		{0x00, "RESTORE"}, {0x08, "RESTORE"},
		{0x18, "SEEK"}, {0x50, "STEP-IN"}, {0x70, "STEP-OUT"},
		{0x80, "READ_SECTOR"}, {0x9F, "READ_SECTOR"},
		{0xA0, "WRITE_SECTOR"}, {0xBF, "WRITE_SECTOR"},
		{0xC0, "READ_ADDR"}, {0xCF, "READ_ADDR"},
		{0xD0, "FORCE_INT"}, {0xDF, "FORCE_INT"},
		{0xE0, "READ_TRACK"}, {0xEF, "READ_TRACK"},
		{0xF0, "WRITE_TRACK"}, {0xFC, "WRITE_TRACK"}, {0xFF, "WRITE_TRACK"},
	} {
		if got := commandName(tc.cmd); got != tc.want {
			t.Errorf("cmd 0x%02X decoded as %s, want %s", tc.cmd, got, tc.want)
		}
	}
}

// TestWriteTrackCompletesWithoutData: TR-DOS 5.03 (and some loaders) issue
// WRITE TRACK (0xFC) but send no track data, then wait for INTRQ. On real
// hardware the command finishes at the index pulse (one 200 ms revolution), so
// it must not hang waiting for 6250 bytes.
func TestWriteTrackCompletesWithoutData(t *testing.T) {
	ctrl := NewBetaDiskController()
	ctrl.SetClockHz(3584000)
	ctrl.MountDisk(NewTRDisk())
	ctrl.WritePort(0x00FF, 0x1C) // drive 0, no reset

	ctrl.WritePort(0x001F, 0xFC) // WRITE TRACK, no data written
	if ctrl.GetInterruptRequest() {
		t.Fatal("WRITE TRACK should not raise INTRQ before completion")
	}

	// Advance past one revolution (200 ms = cpuHz*200/1000 T-states).
	ctrl.SetTick(3584000 * 200 / 1000)

	if !ctrl.GetInterruptRequest() {
		t.Fatal("WRITE TRACK with no data did not complete (no INTRQ) after one revolution")
	}
}

// TestReadSectorCompletesWithoutData: a multi-sector READ SECTOR (0x98) whose
// data the CPU never reads would set LOST DATA and raise INTRQ on real hardware
// (the FD179x keeps reading and loses each byte). The emulator must not hang
// with DRQ asserted forever.
func TestReadSectorCompletesWithoutData(t *testing.T) {
	ctrl := NewBetaDiskController()
	ctrl.SetClockHz(3584000)
	ctrl.MountDisk(NewTRDisk())
	ctrl.WritePort(0x00FF, 0x1C) // drive 0, no reset
	ctrl.WritePort(0x005F, 0x01) // sector 1

	ctrl.WritePort(0x001F, 0x98) // READ SECTOR (multi), no data read
	if ctrl.GetInterruptRequest() {
		t.Fatal("READ SECTOR should not raise INTRQ before completion")
	}

	// Advance past the rotational latency (DRQ rises), then one more revolution
	// so the unserviced transfer is finalized as LOST DATA + INTRQ.
	ready := ctrl.dataReadyTick
	ctrl.SetTick(ready)
	ctrl.SetTick(ready + 3584000*200/1000)

	if !ctrl.GetInterruptRequest() {
		t.Fatal("READ SECTOR with no data read did not complete (no INTRQ) after one revolution")
	}
	if ctrl.GetStatus()&uint8(BetaDiskLostData) == 0 {
		t.Fatal("READ SECTOR with no data read should set LOST DATA")
	}
}

// TestBetaDiskTypeITiming verifies RESTORE/SEEK/STEP are not instantaneous:
// BUSY stays set while the head steps at the r1r0 rate (plus head-settling time
// when the h bit is set), and INTRQ rises only once the step completes.
func TestBetaDiskTypeITiming(t *testing.T) {
	ctrl := NewBetaDiskController()
	ctrl.SetClockHz(3500000)
	ctrl.MountDisk(NewTRDisk())

	ctrl.WritePort(0x7F, 10)   // data register = target track 10
	ctrl.WritePort(0x1F, 0x10) // SEEK (h=0, r1r0=00 -> 6 ms/step, 10 steps)

	if ctrl.GetStatus()&uint8(BetaDiskBusy) == 0 {
		t.Fatal("SEEK should assert BUSY while stepping")
	}
	if ctrl.GetInterruptRequest() {
		t.Fatal("SEEK should not raise INTRQ until the step completes")
	}

	// Partway through the seek the controller is still busy.
	ctrl.SetTick(ctrl.typeICompleteTick / 2)
	if ctrl.GetStatus()&uint8(BetaDiskBusy) == 0 {
		t.Fatal("SEEK should still be busy partway through the step")
	}

	// Past the step + head-settle time it completes.
	ctrl.SetTick(ctrl.typeICompleteTick)
	if ctrl.GetStatus()&uint8(BetaDiskBusy) != 0 {
		t.Fatal("SEEK should clear BUSY once the step completes")
	}
	if !ctrl.GetInterruptRequest() {
		t.Fatal("SEEK should raise INTRQ once the step completes")
	}
	if ctrl.GetTrack() != 10 {
		t.Fatalf("SEEK should have reached track 10, got %d", ctrl.GetTrack())
	}
}

func TestEveryCommandRaisesIntrq(t *testing.T) {
	for cmd := 0; cmd <= 0xFF; cmd++ {
		ctrl := NewBetaDiskController()
		ctrl.SetClockHz(3546900)
		// A single-cylinder disk keeps the cross-track continuation (multi-sector
		// + side compare) to 32 sectors so the fixed drain bound below is enough.
		ctrl.MountDisk(newBlankTRDOSDisk(1, 2))
		ctrl.WritePort(0x00FF, 0x1C) // drive 0, no reset

		ctrl.WritePort(0x001F, uint8(cmd))

		// Complete any pending latency before draining/checking: Type I
		// (RESTORE/SEEK/STEP) step + settle, and Type II/III rotational latency.
		if ctrl.typeIPending {
			ctrl.SetTick(ctrl.typeICompleteTick)
		}
		if ctrl.dataReadyTick != 0 {
			ctrl.SetTick(ctrl.dataReadyTick)
		}

		// Commands that transfer data assert DRQ and raise INTRQ when done.
		// WRITE commands (WRITE SECTOR, WRITE TRACK) need writes to DATA register.
		// READ commands (READ SECTOR, READ TRACK, READ ADDRESS) need reads from DATA.
		isWriteCommand := (cmd >= 0xA0 && cmd <= 0xBF) || (cmd >= 0xF0 && cmd <= 0xFF)

		for i := 0; ctrl.dataRequest && i < 8192; i++ {
			if isWriteCommand {
				ctrl.WritePort(0x007F, 0xE5) // Write dummy data
			} else {
				ctrl.ReadPort(0x007F) // Read and discard
			}
		}

		if !ctrl.GetInterruptRequest() {
			t.Errorf("cmd 0x%02X (%s) completed without INTRQ: TR-DOS would hang",
				cmd, commandName(uint8(cmd)))
		}
	}
}
