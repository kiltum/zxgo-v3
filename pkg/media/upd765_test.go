package media

import (
	"bytes"
	"testing"
)

// newPlus3Disk builds a +3-style disk: 40 tracks, 2 sides, 9 sectors of 512
// bytes each, so the uPD765's 512-byte-sector data path can be exercised.
func newPlus3Disk() *Disk {
	d := &Disk{
		Type:            DiskTypeDSK,
		TracksPerSide:   40,
		Sides:           2,
		SectorsPerTrack: 9,
		SectorSize:      512,
		Tracks:          make([]DiskTrack, 40*2),
	}
	for ti := range d.Tracks {
		d.Tracks[ti].Sectors = make([]DiskSector, 9)
		track := uint8(ti % 40)
		side := uint8(ti / 40)
		for si := range d.Tracks[ti].Sectors {
			d.Tracks[ti].Sectors[si] = DiskSector{
				Track:    track,
				Side:     side,
				SectorID: uint8(si + 1),
				Data:     make([]byte, 512),
			}
		}
	}
	return d
}

// dataReady advances u's clock so a pending READ/WRITE DATA sector is under the
// head; the controller only delivers/accepts bytes once curTick >= dataReadyTick.
func dataReady(u *UPD765) { u.SetTick(u.dataReadyTick) }

// TestUPD765PortDecode pins the +3 FDC port decode: A15=0, A14=0, A13=1, A1=0.
// The status port (0x2FFD) and data port (0x3FFD) differ only in A12, and the
// paging ports (0x7FFD, 0x1FFD) must not be claimed.
func TestUPD765PortDecode(t *testing.T) {
	u := NewUPD765()
	for _, port := range []uint16{0x2FFD, 0x3FFD} {
		if !u.HandlesPort(port) {
			t.Errorf("0x%04X should be handled", port)
		}
	}
	// The paging ports (A14=1 for 0x7FFD, A13=0 for 0x1FFD) must not be claimed.
	for _, port := range []uint16{0x1FFD, 0x7FFD, 0x0FFD} {
		if u.HandlesPort(port) {
			t.Errorf("0x%04X must not be handled", port)
		}
	}
}

// TestUPD765RecalibrateDriveSelect pins the RECALIBRATE drive decode: the
// single parameter byte's low two bits select the unit, so unit 0 must yield
// fddBusy bit 0 (not bit 2).
func TestUPD765RecalibrateDriveSelect(t *testing.T) {
	u := NewUPD765()
	u.Write(0x3FFD, 0x07)         // RECALIBRATE opcode
	u.Write(0x3FFD, 0x00)         // unit 0
	u.SetTick(u.seekCompleteTick) // advance past the seek (0 steps from track 0)
	if u.fddBusy != 0x01 {
		t.Errorf("RECALIBRATE unit 0 -> fddBusy=0x%02X, want 0x01", u.fddBusy)
	}
	if u.st0 != 0x20 {
		t.Errorf("RECALIBRATE ST0=0x%02X, want 0x20", u.st0)
	}
	if !u.intrPending {
		t.Error("RECALIBRATE should leave an interrupt pending")
	}
}

// TestUPD765StatusVsData pins the A12 status/data distinction: writing to
// 0x3FFD feeds the command phase, while 0x2FFD is a status read only.
func TestUPD765StatusVsData(t *testing.T) {
	u := NewUPD765()
	// A write to the status port must be ignored (motor is via 0x1FFD).
	u.Write(0x2FFD, 0x07)
	if u.cmd != 0 {
		t.Errorf("write to 0x2FFD must be ignored, cmd=0x%02X", u.cmd)
	}
	// A write to the data port starts the command phase.
	u.Write(0x3FFD, 0x07)
	if u.cmd != 0x07 {
		t.Errorf("write to 0x3FFD should set cmd, got 0x%02X", u.cmd)
	}
}

// TestUPD765CommandSequence verifies the command phase resets between commands:
// a SPECIFY (3 bytes) followed by a RECALIBRATE (2 bytes) must both parse.
func TestUPD765CommandSequence(t *testing.T) {
	u := NewUPD765()
	// SPECIFY: opcode + 2 params.
	u.Write(0x3FFD, 0x03)
	u.Write(0x3FFD, 0xAF)
	u.Write(0x3FFD, 0x07)
	if u.cmd != 0 {
		t.Errorf("SPECIFY should clear cmd, got 0x%02X", u.cmd)
	}
	// RECALIBRATE: opcode + 1 param.
	u.Write(0x3FFD, 0x07)
	u.Write(0x3FFD, 0x00)
	if u.fddBusy != 0x01 {
		t.Errorf("RECALIBRATE after SPECIFY: fddBusy=0x%02X, want 0x01", u.fddBusy)
	}
}

// TestUPD765SenseInterruptStatus verifies SENSE INT returns ST0+PCN for a
// pending seek interrupt and 0x80 (invalid command) once acknowledged.
func TestUPD765SenseInterruptStatus(t *testing.T) {
	u := NewUPD765()
	u.Write(0x3FFD, 0x07) // RECALIBRATE
	u.Write(0x3FFD, 0x00)
	u.SetTick(u.seekCompleteTick) // advance past the seek (0 steps from track 0)

	u.Write(0x3FFD, 0x08) // SENSE INTERRUPT STATUS
	if u.phase != 2 {
		t.Fatal("SENSE INT should enter the result phase")
	}
	r0 := u.Read(0x3FFD)
	r1 := u.Read(0x3FFD)
	if r0 != 0x20 || r1 != 0x00 {
		t.Errorf("SENSE INT result = [%02X %02X], want [20 00]", r0, r1)
	}

	// No interrupt pending now: a second SENSE INT returns 0x80.
	u.Write(0x3FFD, 0x08)
	if r := u.Read(0x3FFD); r != 0x80 {
		t.Errorf("SENSE INT with no pending interrupt = 0x%02X, want 0x80", r)
	}
}

// TestUPD765ReadDataIndexing verifies READ DATA matches the sector by its ID
// (C/H/R = params[1..3]) at the physical position set by SEEK (the PCN).
func TestUPD765ReadDataIndexing(t *testing.T) {
	disk := newPlus3Disk()
	want := []byte("+3 SECTOR 7")
	copy(disk.Tracks[5+1*40].Sectors[6].Data, want) // track 5, side 1, sector 7

	u := NewUPD765()
	u.MountDisk(disk)
	// SEEK to track 5 (sets the physical PCN), then READ DATA matching the ID.
	u.Write(0x3FFD, 0x0F) // SEEK
	u.Write(0x3FFD, 0x00) // unit 0
	u.Write(0x3FFD, 0x05) // NCN = track 5
	// READ DATA (MFM): opcode 0x46, then [drive/head, C, H, R, N, EOT, GPL, DTL].
	u.Write(0x3FFD, 0x46)
	u.Write(0x3FFD, 0x04) // drive 0, head 1 (HD bit selects the side)
	u.Write(0x3FFD, 0x05) // C = track 5
	u.Write(0x3FFD, 0x01) // H = side 1
	u.Write(0x3FFD, 0x07) // R = sector 7
	u.Write(0x3FFD, 0x02) // N = 512 bytes
	u.Write(0x3FFD, 0x09) // EOT
	u.Write(0x3FFD, 0x1B) // GPL
	u.Write(0x3FFD, 0xFF) // DTL

	if u.phase != 1 {
		t.Fatalf("READ DATA should enter data phase, phase=%d", u.phase)
	}
	dataReady(u)
	got := make([]byte, 512)
	for i := range got {
		got[i] = u.Read(0x3FFD)
	}
	if !bytes.Equal(got[:len(want)], want) {
		t.Errorf("READ DATA sector data mismatch: got %q, want %q", got[:len(want)], want)
	}
}

// TestUPD765ReadDeletedDataMultiSector verifies READ DELETED DATA (0x4C) is
// treated as a read and that the R..EOT span is assembled into one transfer.
// Custom boot loaders (Speedlock) bypass the +3DOS and read several sectors in
// a single command.
func TestUPD765ReadDeletedDataMultiSector(t *testing.T) {
	disk := newPlus3Disk()
	for i := 2; i <= 5; i++ {
		copy(disk.Tracks[0].Sectors[i-1].Data, []byte("SECTOR "+string(rune('0'+i))))
	}

	u := NewUPD765()
	u.MountDisk(disk)
	// READ DELETED DATA (MFM): opcode 0x4C, params [US/HD, C, H, R, N, EOT, GPL, DTL].
	u.Write(0x3FFD, 0x4C)
	u.Write(0x3FFD, 0x00) // drive/head
	u.Write(0x3FFD, 0x00) // C = track 0
	u.Write(0x3FFD, 0x00) // H = side 0
	u.Write(0x3FFD, 0x02) // R = sector 2
	u.Write(0x3FFD, 0x02) // N = 512
	u.Write(0x3FFD, 0x05) // EOT = 5
	u.Write(0x3FFD, 0x2A) // GPL
	u.Write(0x3FFD, 0xFF) // DTL

	if u.phase != 1 {
		t.Fatalf("READ DELETED DATA should enter data phase, phase=%d", u.phase)
	}
	dataReady(u)
	// 4 sectors (2..5) x 512 bytes.
	got := make([]byte, 4*512)
	for i := range got {
		got[i] = u.Read(0x3FFD)
	}
	if string(got[0:8]) != "SECTOR 2" {
		t.Errorf("sector 2 data wrong: %q", got[0:8])
	}
	if string(got[512:520]) != "SECTOR 3" {
		t.Errorf("sector 3 data wrong: %q", got[512:520])
	}
	if string(got[3*512:3*512+8]) != "SECTOR 5" {
		t.Errorf("sector 5 data wrong: %q", got[3*512:3*512+8])
	}
}

// TestUPD765WriteDataMultiSector verifies WRITE DATA commits the CPU-supplied
// bytes back to each sector in the R..EOT span, instead of silently dropping the
// whole multi-sector buffer through a single-sector write.
func TestUPD765WriteDataMultiSector(t *testing.T) {
	disk := newPlus3Disk()

	u := NewUPD765()
	u.MountDisk(disk)
	// WRITE DATA (MFM): opcode 0x45, params [US/HD, C, H, R, N, EOT, GPL, DTL].
	u.Write(0x3FFD, 0x45)
	u.Write(0x3FFD, 0x00) // drive/head 0
	u.Write(0x3FFD, 0x00) // C = 0
	u.Write(0x3FFD, 0x00) // H = 0
	u.Write(0x3FFD, 0x02) // R = 2
	u.Write(0x3FFD, 0x02) // N = 512
	u.Write(0x3FFD, 0x05) // EOT = 5
	u.Write(0x3FFD, 0x2A) // GPL
	u.Write(0x3FFD, 0xFF) // DTL

	if u.phase != 1 {
		t.Fatalf("WRITE DATA should enter data phase, phase=%d", u.phase)
	}
	dataReady(u)

	// Write 4 sectors (2..5) x 512 bytes of distinct data.
	payload := make([]byte, 4*512)
	for s := 0; s < 4; s++ {
		for i := 0; i < 512; i++ {
			payload[s*512+i] = byte(s*37 + i)
		}
	}
	for i := 0; i < len(payload); i++ {
		u.Write(0x3FFD, payload[i])
	}

	// Every sector in the span must now hold its slice of the payload.
	for s := 2; s <= 5; s++ {
		want := payload[(s-2)*512 : (s-1)*512]
		if got := disk.Tracks[0].Sectors[s-1].Data; !bytes.Equal(got, want) {
			t.Errorf("sector %d not written correctly (len %d, want len %d)", s, len(got), len(want))
		}
	}

	// The result phase reports the LAST sector (R=5), as real hardware does.
	if u.phase != 2 {
		t.Fatalf("WRITE DATA should finish in the result phase, phase=%d", u.phase)
	}
	r := make([]byte, 7)
	for i := range r {
		r[i] = u.Read(0x3FFD)
	}
	if r[5] != 5 {
		t.Errorf("result R = %d, want 5 (last sector)", r[5])
	}
}

// TestUPD765WriteProtect verifies a WRITE DATA to a read-only disk is refused:
// the sector data is unchanged, and the result reports NW (ST1 bit 1) + abnormal
// ST0 (bit 6) instead of a clean write.
func TestUPD765WriteProtect(t *testing.T) {
	disk := newPlus3Disk()
	disk.ReadOnly = true
	orig := append([]byte(nil), disk.Tracks[0].Sectors[1].Data...)

	u := NewUPD765()
	u.MountDisk(disk)
	// WRITE DATA (0x45) sector 2 (R==EOT), 512 bytes.
	u.Write(0x3FFD, 0x45)
	u.Write(0x3FFD, 0x00)
	u.Write(0x3FFD, 0x00)
	u.Write(0x3FFD, 0x00)
	u.Write(0x3FFD, 0x02)
	u.Write(0x3FFD, 0x02)
	u.Write(0x3FFD, 0x02)
	u.Write(0x3FFD, 0x2A)
	u.Write(0x3FFD, 0xFF)
	dataReady(u)
	for i := 0; i < 512; i++ {
		u.Write(0x3FFD, 0xAA)
	}

	// Data must be unchanged.
	if !bytes.Equal(disk.Tracks[0].Sectors[1].Data, orig) {
		t.Error("write-protected disk: sector data was modified")
	}
	// Result: ST0 abnormal (bit 6), ST1 NW (bit 1).
	r := make([]byte, 7)
	for i := range r {
		r[i] = u.Read(0x3FFD)
	}
	if r[0]&0x40 == 0 {
		t.Errorf("ST0 = 0x%02X, want abnormal bit (0x40) set", r[0])
	}
	if r[1]&0x02 == 0 {
		t.Errorf("ST1 = 0x%02X, want NW bit (0x02) set", r[1])
	}
}

// TestUPD765ResultMSR verifies the main status register during the result phase
// sets RQM (bit 7), DIO (bit 6) and CB (bit 4). The +3 boot loaders poll CB to
// decide whether more result bytes are available.
func TestUPD765ResultMSR(t *testing.T) {
	u := NewUPD765()
	u.Write(0x3FFD, 0x08) // SENSE INTERRUPT STATUS -> result phase
	if got := u.Read(0x2FFD); got&0xD0 != 0xD0 {
		t.Errorf("result-phase MSR = %02X, want RQM|DIO|CB (0xD0) bits", got)
	}
}

// TestUPD765DeletedSectorST2 verifies that READ DATA on a sector carrying the
// deleted data mark reports it via ST2 bit 6 (CM). The mark mismatches the
// command (READ DATA expects a normal sector), so CM is set.
func TestUPD765DeletedSectorST2(t *testing.T) {
	disk := newPlus3Disk()
	disk.Tracks[0].Sectors[1].Deleted = true // sector 2

	u := NewUPD765()
	u.MountDisk(disk)
	// READ DATA (0x46) sector 2 (single sector, R==EOT).
	u.Write(0x3FFD, 0x46)
	u.Write(0x3FFD, 0x00)
	u.Write(0x3FFD, 0x00)
	u.Write(0x3FFD, 0x00)
	u.Write(0x3FFD, 0x02) // R = 2
	u.Write(0x3FFD, 0x02) // N
	u.Write(0x3FFD, 0x02) // EOT = 2
	u.Write(0x3FFD, 0x2A) // GPL
	u.Write(0x3FFD, 0xFF) // DTL
	dataReady(u)
	for i := 0; i < 512; i++ {
		u.Read(0x3FFD)
	}
	// result: ST0, ST1, ST2, C, H, R, N
	r := []byte{u.Read(0x3FFD), u.Read(0x3FFD), u.Read(0x3FFD)}
	if r[2]&0x40 == 0 {
		t.Errorf("deleted sector result ST2=%02X, want bit 6 (0x40) set", r[2])
	}
}

// TestUPD765ReadDeletedDataNoCM verifies READ DELETED DATA on a deleted sector
// does NOT set ST2 bit 6 (CM): the mark matches the command, so the result is a
// clean read. Speedlock loaders read their deleted payload sectors this way and
// fail on an unexpected CM bit.
func TestUPD765ReadDeletedDataNoCM(t *testing.T) {
	disk := newPlus3Disk()
	disk.Tracks[0].Sectors[1].Deleted = true // sector 2

	u := NewUPD765()
	u.MountDisk(disk)
	// READ DELETED DATA (0x4C) sector 2.
	u.Write(0x3FFD, 0x4C)
	u.Write(0x3FFD, 0x00)
	u.Write(0x3FFD, 0x00) // C = 0
	u.Write(0x3FFD, 0x00) // H = 0
	u.Write(0x3FFD, 0x02) // R = 2
	u.Write(0x3FFD, 0x02) // N
	u.Write(0x3FFD, 0x02) // EOT = 2
	u.Write(0x3FFD, 0x2A) // GPL
	u.Write(0x3FFD, 0xFF) // DTL
	dataReady(u)
	for i := 0; i < 512; i++ {
		u.Read(0x3FFD)
	}
	r := []byte{u.Read(0x3FFD), u.Read(0x3FFD), u.Read(0x3FFD)}
	if r[2]&0x40 != 0 {
		t.Errorf("READ DELETED DATA on deleted sector ST2=%02X, want bit 6 clear", r[2])
	}
}

// testReadData issues a READ DATA (opcode 0x46) for sectors r..eot of track 0 and
// returns the three status bytes of the result phase.
func testReadData(t *testing.T, u *UPD765, r, eot uint8) []byte {
	t.Helper()
	u.Write(0x3FFD, 0x46)
	u.Write(0x3FFD, 0x00) // US/HD
	u.Write(0x3FFD, 0x00) // C
	u.Write(0x3FFD, 0x00) // H
	u.Write(0x3FFD, r)
	u.Write(0x3FFD, 0x02) // N
	u.Write(0x3FFD, eot)
	u.Write(0x3FFD, 0x2A) // GPL
	u.Write(0x3FFD, 0xFF) // DTL
	dataReady(u)
	for i := 0; i < 512*int(eot-r+1); i++ {
		u.Read(0x3FFD)
	}
	return []byte{u.Read(0x3FFD), u.Read(0x3FFD), u.Read(0x3FFD)}
}

// TestUPD765EndOfCylinderResult verifies the end-of-cylinder termination: a READ
// DATA whose span runs through its EOT sector ends abnormally with EN (ST1 bit 7)
// set and ST2 clean, because the FDC has tried to step past the last sector of
// the track (Fuse upd_fdc.c abort_read_data). Protected loaders probe for exactly
// ST0=0x40 / ST1=0x80 / ST2=0x00: bad2.dsk reads a deliberately mis-numbered
// sector with EOT == R and only accepts the read as "end of track" when it gets
// that triple (ZEsarUX pd765.c carries the same triple for Wec Le Mans).
func TestUPD765EndOfCylinderResult(t *testing.T) {
	u := NewUPD765()
	u.MountDisk(newPlus3Disk())

	// Single sector: R == EOT.
	if r := testReadData(t, u, 2, 2); r[0] != 0x40 || r[1] != 0x80 || r[2] != 0x00 {
		t.Errorf("single-sector EOT result ST0/ST1/ST2 = %02X/%02X/%02X, want 40/80/00", r[0], r[1], r[2])
	}
	// Multi-sector span ending at EOT terminates the same way.
	if r := testReadData(t, u, 2, 4); r[0] != 0x40 || r[1] != 0x80 || r[2] != 0x00 {
		t.Errorf("multi-sector EOT result ST0/ST1/ST2 = %02X/%02X/%02X, want 40/80/00", r[0], r[1], r[2])
	}
}

// TestUPD765EndOfCylinderSuppressedByError verifies the end-of-cylinder status is
// only reported for an otherwise clean transfer: when the image carries a sector
// error (ST1 bit 5 DE, ST2 bit 5 DD) that error is reported instead, and ST0 keeps
// only its unit/head bits.
func TestUPD765EndOfCylinderSuppressedByError(t *testing.T) {
	disk := newPlus3Disk()
	s := &disk.Tracks[0].Sectors[1]
	s.ST1 = 0x20
	s.ST2 = 0x20

	u := NewUPD765()
	u.MountDisk(disk)
	r := testReadData(t, u, 2, 2)
	if r[0] != 0x00 {
		t.Errorf("ST0 = 0x%02X, want 0x00 (EN suppressed by the sector's error)", r[0])
	}
	if r[1] != 0xA0 { // EOC (0x80) | DE (0x20)
		t.Errorf("ST1 = 0x%02X, want 0xA0", r[1])
	}
}

// TestUPD765EndOfCylinderWrite verifies WRITE DATA gets the same termination: a
// write that completes at EOT ends abnormally with EN set (Fuse abort_write_data
// sets it unconditionally there).
func TestUPD765EndOfCylinderWrite(t *testing.T) {
	u := NewUPD765()
	u.MountDisk(newPlus3Disk())

	u.Write(0x3FFD, 0x45) // WRITE DATA
	u.Write(0x3FFD, 0x00) // US/HD
	u.Write(0x3FFD, 0x00) // C
	u.Write(0x3FFD, 0x00) // H
	u.Write(0x3FFD, 0x02) // R
	u.Write(0x3FFD, 0x02) // N
	u.Write(0x3FFD, 0x02) // EOT
	u.Write(0x3FFD, 0x2A) // GPL
	u.Write(0x3FFD, 0xFF) // DTL
	dataReady(u)
	for i := 0; i < 512; i++ {
		u.Write(0x3FFD, 0xAA)
	}
	r := []byte{u.Read(0x3FFD), u.Read(0x3FFD)}
	if r[0] != 0x40 || r[1] != 0x80 {
		t.Errorf("WRITE DATA at EOT ST0/ST1 = %02X/%02X, want 40/80", r[0], r[1])
	}
}

// TestUPD765SpeedlockWeakBits verifies the Speedlock weak-bit hack: re-reading
// sector 2 / track 0 / head 0 perturbs the data so each read differs (the
// loader checks the sector is "unstable").
func TestUPD765SpeedlockWeakBits(t *testing.T) {
	disk := newPlus3Disk()
	for i := range disk.Tracks[0].Sectors[1].Data {
		disk.Tracks[0].Sectors[1].Data[i] = 0xE5 // sector 2 filled with 0xE5
	}

	u := NewUPD765()
	u.MountDisk(disk)

	readSector2 := func() []byte {
		// READ DATA (0x46) sector 2, track 0, head 0, single sector (R==EOT).
		u.Write(0x3FFD, 0x46)
		u.Write(0x3FFD, 0x00)
		u.Write(0x3FFD, 0x00) // C = 0
		u.Write(0x3FFD, 0x00) // H = 0
		u.Write(0x3FFD, 0x02) // R = 2
		u.Write(0x3FFD, 0x02) // N
		u.Write(0x3FFD, 0x02) // EOT = 2
		u.Write(0x3FFD, 0x2A)
		u.Write(0x3FFD, 0xFF)
		dataReady(u)
		data := make([]byte, 512)
		for i := range data {
			data[i] = u.Read(0x3FFD)
		}
		return data
	}

	first := readSector2()
	second := readSector2()
	// The second read XORs bytes at 1-based offsets 29 and 58 (every 29, <64).
	if first[28] == second[28] || first[57] == second[57] {
		t.Errorf("Speedlock hack did not perturb the re-read (offsets 28/57 unchanged)")
	}
}

// TestUPD765CRCErrorResult verifies extended-DSK copy protection: a sector with
// ST1 bit 5 (DE / CRC error) and ST2 bit 5 (DD) reports those errors in the
// result, so a loader that reads a truncated (short) sector sees the CRC error
// instead of a clean read.
func TestUPD765CRCErrorResult(t *testing.T) {
	disk := newPlus3Disk()
	// Mark sector 2 of track 0 as deleted with a CRC error (truncated sector),
	// exactly like Alien Storm's protected tracks.
	s := &disk.Tracks[0].Sectors[1]
	s.Deleted = true
	s.ST1 = 0x20 // DE: CRC error
	s.ST2 = 0x60 // CM + DD

	u := NewUPD765()
	u.MountDisk(disk)
	// READ DELETED DATA (0x4C): the deleted mark matches, so CM stays clear;
	// the CRC error (ST1 0x20, ST2 0x20) still surfaces.
	u.Write(0x3FFD, 0x4C)
	u.Write(0x3FFD, 0x00)
	u.Write(0x3FFD, 0x00)
	u.Write(0x3FFD, 0x00)
	u.Write(0x3FFD, 0x02) // R = 2
	u.Write(0x3FFD, 0x02) // N
	u.Write(0x3FFD, 0x02) // EOT = 2
	u.Write(0x3FFD, 0x2A)
	u.Write(0x3FFD, 0xFF)
	dataReady(u)
	for i := 0; i < 512; i++ {
		u.Read(0x3FFD)
	}
	r := []byte{u.Read(0x3FFD), u.Read(0x3FFD), u.Read(0x3FFD)}
	if r[1] != 0xA0 { // ST1: EOC (0x80) | DE (0x20)
		t.Errorf("ST1=%02X, want 0xA0 (EOC|DE)", r[1])
	}
	if r[2] != 0x20 { // ST2: DD only, CM clear (mark matched READ DELETED DATA)
		t.Errorf("ST2=%02X, want 0x20 (DD, no CM)", r[2])
	}
}

// TestUPD765RotationalLatency verifies READ DATA data is not available until
// the rotational latency elapses: the MSR's RQM bit stays low and reads return 0
// until the tick reaches dataReadyTick, then the sector flows.
func TestUPD765RotationalLatency(t *testing.T) {
	disk := newPlus3Disk()
	copy(disk.Tracks[0].Sectors[1].Data, []byte("SECTOR 2"))

	u := NewUPD765()
	u.MountDisk(disk)
	// READ DATA (0x46) sector 2, single sector (R==EOT).
	u.Write(0x3FFD, 0x46)
	u.Write(0x3FFD, 0x00)
	u.Write(0x3FFD, 0x00)
	u.Write(0x3FFD, 0x00)
	u.Write(0x3FFD, 0x02)
	u.Write(0x3FFD, 0x02)
	u.Write(0x3FFD, 0x02)
	u.Write(0x3FFD, 0x2A)
	u.Write(0x3FFD, 0xFF)

	if u.phase != 1 {
		t.Fatalf("READ DATA should enter data phase, phase=%d", u.phase)
	}

	// Before the latency elapses, RQM is low and the data register returns 0.
	if msr := u.Read(0x2FFD); msr&0x80 != 0 {
		t.Errorf("before latency: MSR RQM set (0x%02X), want clear", msr)
	}
	if got := u.Read(0x3FFD); got != 0 {
		t.Errorf("before latency: data read = 0x%02X, want 0", got)
	}

	// Just before dataReadyTick, still not ready.
	u.SetTick(u.dataReadyTick - 1)
	if msr := u.Read(0x2FFD); msr&0x80 != 0 {
		t.Errorf("tick=dataReadyTick-1: RQM set (0x%02X), want clear", msr)
	}

	// At dataReadyTick the sector is under the head.
	u.SetTick(u.dataReadyTick)
	if msr := u.Read(0x2FFD); msr&0x80 == 0 {
		t.Error("at dataReadyTick: RQM should be set")
	}
	if got := u.Read(0x3FFD); got != 'S' {
		t.Errorf("at dataReadyTick: first data byte = 0x%02X, want 'S'", got)
	}
}
