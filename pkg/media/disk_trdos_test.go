package media

import "testing"

// TestTRDosDriveDetection replicates the routine that produced the "no disk"
// error. TR-DOS 5.x at 0x3DAD issues RESTORE, samples status bit 1, then polls
// it up to 65536 times waiting for the bit to CHANGE:
//
//	ld a,0x08 / call 0x3D9A   ; RESTORE, wait for INTRQ
//	ld de,0x0000
//	in a,(0x1F) / and 0x02 / ld b,a
//	l3dba: in a,(0x1F) / and 0x02 / cp b / ret nz
//	       inc de / ld a,e / or d / jr nz,l3dba
//	jp report_tape_loading_error_and_issue_no_disk_error
//
// Status bit 1 is the drive's index pulse in the Type I status layout. A
// controller that never toggles it looks like a drive that is not spinning, and
// TR-DOS reports "no disk" for every command.
func TestTRDosDriveDetection(t *testing.T) {
	const cpuHz = 3546900
	// The polling loop is roughly 48 T-states per pass.
	const ticksPerPass = 48

	ctrl := NewBetaDiskController()
	ctrl.SetClockHz(cpuHz)
	ctrl.MountDisk(NewTRDisk())

	// Select drive 0, side 0, no reset, head loaded -- as TR-DOS does before
	// probing the drive.
	ctrl.WritePort(0x00FF, 0x1C)

	ctrl.WritePort(0x001F, 0x08) // RESTORE (h=1: head load, r1r0=00: 6 ms/step)

	// RESTORE takes head-settling time (5 step times) before raising INTRQ.
	ctrl.SetTick(ctrl.typeICompleteTick)
	if !ctrl.GetInterruptRequest() {
		t.Fatal("RESTORE did not raise INTRQ; TR-DOS would hang before probing")
	}

	tick := ctrl.typeICompleteTick
	ctrl.SetTick(tick)
	first := ctrl.ReadPort(0x001F) & 0x02

	for i := 0; i < 65536; i++ {
		tick += ticksPerPass
		ctrl.SetTick(tick)
		if ctrl.ReadPort(0x001F)&0x02 != first {
			t.Logf("index pulse changed after %d polls (%d T-states, %.1f ms)",
				i+1, tick, float64(tick)*1000/cpuHz)
			return
		}
	}
	t.Fatalf("index pulse never changed in 65536 polls: TR-DOS reports \"no disk\"")
}

// TestTRDosDriveDetectionNoDisk is the other half of the contract: an empty
// drive must NOT produce an index pulse, so the same routine still reports
// "no disk".
func TestTRDosDriveDetectionNoDisk(t *testing.T) {
	ctrl := NewBetaDiskController()
	ctrl.SetClockHz(3546900)
	ctrl.WritePort(0x00FF, 0x1C)
	ctrl.WritePort(0x001F, 0x08) // RESTORE

	tick := int64(0)
	ctrl.SetTick(tick)
	first := ctrl.ReadPort(0x001F) & 0x02

	for i := 0; i < 65536; i++ {
		tick += 48
		ctrl.SetTick(tick)
		if ctrl.ReadPort(0x001F)&0x02 != first {
			t.Fatalf("empty drive produced an index pulse after %d polls", i+1)
		}
	}
}

// TestTRDosSectorTransfer replicates TR-DOS's sector-read loop at 0x3FE5:
//
//	l3fe5: in a,(0xFF) / and 0xC0 / jr z,l3fe5   ; wait for DRQ or INTRQ
//	       ret m                                 ; INTRQ -> transfer finished
//	       ini / jr l3fe5                        ; DRQ  -> take one byte
//
// INTRQ must stay low for the whole transfer and rise only after the last byte,
// otherwise the very first "ret m" succeeds and the loop moves zero bytes.
func TestTRDosSectorTransfer(t *testing.T) {
	disk := NewTRDisk()

	// Put a recognisable pattern in track 3, side 1, sector 5.
	sec := disk.sectorRef(3, 1, 5)
	if sec == nil {
		t.Fatal("sector 5 of track 3 side 1 missing")
	}
	for i := range sec.Data {
		sec.Data[i] = byte(i)
	}

	ctrl := NewBetaDiskController()
	ctrl.SetClockHz(3546900)
	ctrl.MountDisk(disk)

	ctrl.WritePort(0x00FF, 0x0C) // drive 0, side 1 (bit 4 clear), no reset
	ctrl.WritePort(0x003F, 3)    // TRACK
	ctrl.WritePort(0x005F, 5)    // SECTOR
	ctrl.WritePort(0x001F, 0x80) // READ SECTOR
	ctrl.SetTick(ctrl.dataReadyTick) // advance past the rotational latency

	var got []byte
	for i := 0; i < 4096; i++ {
		sys := ctrl.ReadPort(0x00FF) & 0xC0
		if sys == 0 {
			t.Fatal("neither DRQ nor INTRQ asserted during transfer")
		}
		if sys&0x80 != 0 { // INTRQ: transfer complete
			break
		}
		got = append(got, ctrl.ReadPort(0x007F))
	}

	if len(got) != 256 {
		t.Fatalf("transferred %d bytes, want 256", len(got))
	}
	for i, b := range got {
		if b != byte(i) {
			t.Fatalf("byte %d: got 0x%02X, want 0x%02X", i, b, byte(i))
		}
	}

	// TR-DOS then checks "in a,(0x1F) / and 0x7F / ret z": every error bit and
	// BUSY must be clear, or it reports a disk error.
	if s := ctrl.ReadPort(0x001F) & 0x7F; s != 0 {
		t.Errorf("status after successful read = 0x%02X, want 0x00", s)
	}
}

// TestBetaDiskPortDecode pins the address decode. A0-A4 must all be high; the
// controller must not answer for the ULA, the 128K paging port or the AY.
func TestBetaDiskPortDecode(t *testing.T) {
	ctrl := NewBetaDiskController()

	claimed := []uint16{0x001F, 0x003F, 0x005F, 0x007F, 0x00FF, 0xFF1F, 0x5FFF}
	for _, p := range claimed {
		if !ctrl.HandlesPort(p) {
			t.Errorf("port 0x%04X should be claimed", p)
		}
	}

	// 0xFE = ULA (border, keyboard, EAR), 0x7FFD = 128K paging,
	// 0xFFFD/0xBFFD = AY register select and data.
	rejected := []uint16{0x00FE, 0x7FFE, 0x7FFD, 0xFFFD, 0xBFFD, 0x00F6, 0x00F7}
	for _, p := range rejected {
		if ctrl.HandlesPort(p) {
			t.Errorf("port 0x%04X must not be claimed by the Beta Disk", p)
		}
	}
}

// TestTRDosMultiSectorRead verifies the multi-sector flag (command bit 4). A
// READ SECTOR with bit 4 set transfers consecutive sectors, incrementing the
// sector register, and terminates with RECORD NOT FOUND + INTRQ when the next
// sector ID does not exist (sector 17 past the end of a 16-sector track).
func TestTRDosMultiSectorRead(t *testing.T) {
	disk := NewTRDisk()

	// Fill track 0 side 0 sectors 1..16 with a recognisable pattern so we can
	// tell which sector each byte came from.
	for sid := uint8(1); sid <= 16; sid++ {
		sec := disk.sectorRef(0, 0, sid)
		for i := range sec.Data {
			sec.Data[i] = sid // every byte of sector N is 0xN
		}
	}

	ctrl := NewBetaDiskController()
	ctrl.SetClockHz(3546900)
	ctrl.MountDisk(disk)
	ctrl.WritePort(0x00FF, 0x1C) // drive 0, side 0
	ctrl.WritePort(0x003F, 0)    // TRACK 0
	ctrl.WritePort(0x005F, 1)    // SECTOR 1
	ctrl.WritePort(0x001F, 0x90) // READ SECTOR, multiple flag set
	ctrl.SetTick(ctrl.dataReadyTick) // advance past the rotational latency

	// Drain the transfer the way TR-DOS does: wait for DRQ or INTRQ, stop on
	// INTRQ, take one byte on DRQ.
	var got []byte
	for i := 0; i < 8192; i++ {
		sys := ctrl.ReadPort(0x00FF) & 0xC0
		if sys == 0 {
			t.Fatal("neither DRQ nor INTRQ during multi-sector read")
		}
		if sys&0x80 != 0 {
			break
		}
		got = append(got, ctrl.ReadPort(0x007F))
	}

	// 16 sectors * 256 bytes.
	if len(got) != 16*256 {
		t.Fatalf("multi-sector read transferred %d bytes, want %d", len(got), 16*256)
	}
	// Sector N fills 256 consecutive bytes, all equal to N.
	for sid := uint8(1); sid <= 16; sid++ {
		base := int(sid-1) * 256
		for i := 0; i < 256; i++ {
			if got[base+i] != sid {
				t.Fatalf("byte %d: got 0x%02X, want 0x%02X (sector %d)",
					base+i, got[base+i], sid, sid)
			}
		}
	}

	// The command must have ended with RECORD NOT FOUND and INTRQ.
	if !ctrl.GetInterruptRequest() {
		t.Error("multi-sector read did not raise INTRQ")
	}
	if s := ctrl.ReadPort(0x001F); s&uint8(BetaDiskRecNotFound) == 0 {
		t.Errorf("multi-sector read status 0x%02X: RECORD NOT FOUND not set", s)
	}
}

// TestTRDosMultiSectorReadCrossTrack verifies the VG93 cross-track continuation:
// with the side-compare flag (command bit 1) set, a multi-sector read steps from
// the last sector of a track to the first sector of the next side, and from the
// second side to the next track, instead of terminating with RECORD NOT FOUND.
func TestTRDosMultiSectorReadCrossTrack(t *testing.T) {
	disk := newBlankTRDOSDisk(2, 2) // 2 cylinders * 2 sides = 4 tracks of 16 sectors

	// Paint each track a distinct byte so the read order is observable.
	paint := func(track, side uint8, val byte) {
		for sid := uint8(1); sid <= 16; sid++ {
			sec := disk.sectorRef(track, side, sid)
			for i := range sec.Data {
				sec.Data[i] = val
			}
		}
	}
	paint(0, 0, 0xAA)
	paint(0, 1, 0xBB)
	paint(1, 0, 0xCC)
	paint(1, 1, 0xDD)

	ctrl := NewBetaDiskController()
	ctrl.SetClockHz(3546900)
	ctrl.MountDisk(disk)
	ctrl.WritePort(0x00FF, 0x1C) // drive 0, side 0
	ctrl.WritePort(0x003F, 0)    // TRACK 0
	ctrl.WritePort(0x005F, 1)    // SECTOR 1
	ctrl.WritePort(0x001F, 0x92) // READ SECTOR, multiple + side compare
	ctrl.SetTick(ctrl.dataReadyTick) // advance past the rotational latency

	var got []byte
	for i := 0; i < 20000; i++ {
		sys := ctrl.ReadPort(0x00FF) & 0xC0
		if sys == 0 {
			t.Fatal("neither DRQ nor INTRQ during cross-track multi-sector read")
		}
		if sys&0x80 != 0 {
			break
		}
		got = append(got, ctrl.ReadPort(0x007F))
	}

	// 4 tracks * 16 sectors * 256 bytes.
	if len(got) != 4*16*256 {
		t.Fatalf("cross-track multi-sector read transferred %d bytes, want %d", len(got), 4*16*256)
	}
	// Expected order: track 0 side 0 (AA), track 0 side 1 (BB), track 1 side 0
	// (CC), track 1 side 1 (DD).
	want := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	for trk := 0; trk < 4; trk++ {
		base := trk * 16 * 256
		for i := 0; i < 16*256; i++ {
			if got[base+i] != want[trk] {
				t.Fatalf("track %d byte %d: got 0x%02X, want 0x%02X", trk, i, got[base+i], want[trk])
			}
		}
	}

	if !ctrl.GetInterruptRequest() {
		t.Error("cross-track multi-sector read did not raise INTRQ")
	}
	if s := ctrl.ReadPort(0x001F); s&uint8(BetaDiskRecNotFound) == 0 {
		t.Errorf("cross-track multi-sector read status 0x%02X: RECORD NOT FOUND not set", s)
	}
}

// TestTRDosMultiSectorWriteCrossTrack verifies the WRITE side of cross-track
// continuation: with side compare set, a multi-sector write steps across the
// side and track boundaries instead of stopping at the track end.
func TestTRDosMultiSectorWriteCrossTrack(t *testing.T) {
	disk := newBlankTRDOSDisk(2, 2)

	ctrl := NewBetaDiskController()
	ctrl.SetClockHz(3546900)
	ctrl.MountDisk(disk)
	ctrl.WritePort(0x00FF, 0x1C) // drive 0, side 0
	ctrl.WritePort(0x003F, 0)    // TRACK 0
	ctrl.WritePort(0x005F, 1)    // SECTOR 1
	ctrl.WritePort(0x001F, 0xB2) // WRITE SECTOR, multiple + side compare
	ctrl.SetTick(ctrl.dataReadyTick) // advance past the rotational latency

	written := 0
	for i := 0; i < 20000; i++ {
		sys := ctrl.ReadPort(0x00FF) & 0xC0
		if sys&0x80 != 0 {
			break
		}
		if sys&0x40 == 0 {
			t.Fatal("neither DRQ nor INTRQ during cross-track multi-sector write")
		}
		ctrl.WritePort(0x007F, byte(i%256))
		written++
	}

	if written != 4*16*256 {
		t.Fatalf("cross-track multi-sector write accepted %d bytes, want %d", written, 4*16*256)
	}
	if !ctrl.GetInterruptRequest() {
		t.Error("cross-track multi-sector write did not raise INTRQ")
	}

	// The write must have reached the second side and the second cylinder.
	if disk.sectorRef(0, 1, 1) == nil {
		t.Error("track 0 side 1 sector 1 was not written")
	}
	if disk.sectorRef(1, 0, 1) == nil {
		t.Error("track 1 side 0 sector 1 was not written")
	}
}

// TestTRDosMultiSectorWrite verifies the multi-sector flag on WRITE SECTOR: it
// writes consecutive sectors and stops with RECORD NOT FOUND at the track end.
func TestTRDosMultiSectorWrite(t *testing.T) {
	disk := NewTRDisk()

	ctrl := NewBetaDiskController()
	ctrl.SetClockHz(3546900)
	ctrl.MountDisk(disk)
	ctrl.WritePort(0x00FF, 0x1C) // drive 0, side 0
	ctrl.WritePort(0x003F, 0)    // TRACK 0
	ctrl.WritePort(0x005F, 1)    // SECTOR 1
	ctrl.WritePort(0x001F, 0xB0) // WRITE SECTOR, multiple flag set
	ctrl.SetTick(ctrl.dataReadyTick) // advance past the rotational latency

	// Feed the transfer the way TR-DOS does: wait for DRQ, OUTI one byte.
	written := 0
	for i := 0; i < 8192; i++ {
		sys := ctrl.ReadPort(0x00FF) & 0xC0
		if sys&0x80 != 0 {
			break
		}
		if sys&0x40 == 0 {
			t.Fatal("neither DRQ nor INTRQ during multi-sector write")
		}
		ctrl.WritePort(0x007F, byte(i%256))
		written++
	}

	if written != 16*256 {
		t.Fatalf("multi-sector write accepted %d bytes, want %d", written, 16*256)
	}
	if !ctrl.GetInterruptRequest() {
		t.Error("multi-sector write did not raise INTRQ")
	}

	// Every sector should now hold the written pattern; verify sector 3.
	sec, err := disk.ReadSector(0, 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	for i := range sec {
		if sec[i] != byte(i) {
			t.Fatalf("sector 3 byte %d: got 0x%02X, want 0x%02X", i, sec[i], byte(i))
		}
	}
}
