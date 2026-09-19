package media

import (
	"fmt"
	"log/slog"
)

// upd765Log is the optional logger for the uPD765 floppy controller.
var upd765Log *slog.Logger

// SetUPD765Logger wires the uPD765 logger.
func SetUPD765Logger(l *slog.Logger) { upd765Log = l }

// writeTarget records one sector of a WRITE DATA span (its ID and byte size) so
// finishData can commit the CPU-supplied bytes back sector-by-sector.
type writeTarget struct {
	sectorID uint8
	size     int
}

// UPD765 implements the NEC uPD765A floppy disk controller used by the
// ZX Spectrum +2A/+3.
//
// The +3 maps it to two I/O ports, both with A15=0, A14=0, A13=1, A1=0:
//
//	0x2FFD (A12=0): main status register (read)
//	0x3FFD (A12=1): data register (read/write)
//
// The distinguishing address line is A12 (0x1000), not A14: 0x2FFD and 0x3FFD
// both have A14 clear. The controller runs in non-DMA mode: the CPU polls the
// main status register and transfers each byte through the data register.
//
// Command phase: the first byte written to the data register is the command
// opcode, followed by its parameter bytes. params holds only the parameters
// (params[0] is the first parameter), matching the uPD765 data register and
// FUSE's data_register[0] indexing.
type UPD765 struct {
	disk  *Disk // drive A
	diskB *Disk // drive B (nil when not mounted)
	// driveSel is the currently selected drive (US bits of the last command),
	// so commands/status report the right disk.
	driveSel uint8

	curTrack  uint8
	curSide   uint8
	curSector uint8
	// Physical position of the current READ/WRITE DATA sector: curPCN is the
	// cylinder the head is on (from SEEK), curHead is the side selected by the
	// US/HD byte. The uPD765 matches a sector by its *ID* (C/H/R = curTrack/curSide/
	// curSector) at this physical position, so the two must be kept apart -
	// copy-protected disks record deliberately wrong ID C/H values (e.g. C=143 for
	// a sector on physical track 2).
	curPCN  uint8
	curHead uint8
	curN    uint8

	// command phase
	cmd        byte
	params     []byte
	paramCount int

	// result phase
	result    []byte
	resultIdx int

	// data transfer phase (READ/WRITE DATA)
	dataBuf []byte
	dataIdx int
	writing bool // true = WRITE DATA (CPU->FDC), false = READ DATA (FDC->CPU)
	delData bool // true = READ/WRITE DELETED DATA (command expects deleted sectors)
	// writePlan records the sector boundaries of a WRITE DATA span, so finishData
	// writes each sector back at its own length instead of the whole buffer as one.
	writePlan []writeTarget
	// writeProtect is set when a WRITE DATA targets a read-only disk; finishData
	// then reports NW (ST1 bit 1) and abnormal ST0 instead of committing.
	writeProtect bool
	// formatActive marks an in-progress FORMAT TRACK: the CPU writes 4*SC bytes
	// of sector IDs (C/H/R/N), and finishData then lays out the formatted track.
	formatActive bool
	formatFill   byte

	// phase: 0 = command/params, 1 = data transfer, 2 = result
	phase int

	st0         uint8
	pcn         uint8
	intrPending bool  // seek/recalibrate interrupt awaiting SENSE INTERRUPT STATUS
	fddBusy     uint8 // seek-in-progress bits (one per drive, D0-D3)
	lastMSR     uint8

	// Speedlock weak-bit hack (Fuse upd_fdc.c): some loaders re-read sector 2 /
	// track 0 / head 0 and expect the bytes to come back unstable.
	speedlock     int  // consecutive-read count
	lastSectorKey uint // (H&1)+(C<<1)+(R<<8) of the last single-sector read

	// Data-phase overrun (Fuse upd_fdc.c timeout_event). In non-DMA mode the CPU
	// must drain the sector's bytes before the disk would finish the transfer; if
	// it stops (e.g. a copy-protection loader reads only its 504-byte window then
	// polls the MSR), the real FDC raises ST1 overrun and terminates the data
	// phase. dataDeadline is the tick by which the whole phase must complete,
	// armed when the read-data phase starts.
	curTick       int64
	dataReadyTick int64 // tick at which the target sector is under the head (RQM rises)
	dataDeadline  int64

	// cpuHz is the emulator's T-state clock, used to convert the seek step rate
	// to T-states. seekPending marks an in-progress SEEK/RECALIBRATE;
	// seekCompleteTick is the tick at which it finishes (the interrupt rises).
	cpuHz            int64
	seekPending      bool
	seekCompleteTick int64

	// noTiming disables the seek/rotational latency so commands complete
	// instantly. Useful for big (non-copy-protected) images that load slowly.
	noTiming bool

	log *slog.Logger
}

// upd765OverrunTStates is how long the FDC allows a whole read-data phase to run
// before reporting an overrun (ST1 bit 4) and ending it. This mirrors Fuse's
// timeout_event, which is scheduled for 2 disk revolutions (400 ms at 3.5 MHz);
// a normal multi-sector read finishes well inside it, while the Erbe "Long 8K
// tracks" loader drains only its 504-byte header window and then polls the MSR,
// so the phase overruns and the FDC terminates, which the loader relies on.
const upd765OverrunTStates = 1418760

// NewUPD765 creates a uPD765 controller.
func NewUPD765() *UPD765 {
	return &UPD765{cpuHz: 3546900} // +2A/+3 3.5469 MHz (default)
}

// SetLogger wires an optional logger.
func (u *UPD765) SetLogger(l *slog.Logger) { u.log = l }

// SetClockHz sets the T-state clock the seek step rate is measured against.
func (u *UPD765) SetClockHz(hz int) {
	if hz > 0 {
		u.cpuHz = int64(hz)
	}
}

// SetNoTiming disables the seek/rotational latency so commands complete
// instantly (for big non-copy-protected images that load slowly).
func (u *UPD765) SetNoTiming(on bool) { u.noTiming = on }

// seekDelayTStates returns the seek/recalibrate completion delay: steps times a
// fixed 6 ms step rate (the +3 FDC runs at 1 MHz; a full SPECIFY SRT model is a
// later refinement).
func (u *UPD765) seekDelayTStates(steps int) int64 {
	if u.noTiming {
		return 0
	}
	return u.cpuHz * int64(6*steps) / 1000
}

// rotationalLatencyTStates returns how long a READ/WRITE DATA command waits for
// the target sector to rotate under the head. Modeled as half a revolution at
// 300 RPM (200 ms/rev), the average latency to reach an arbitrary sector; this
// is the same model the WD1793's Type II/III commands use. Before this elapses
// the data register stays unavailable (RQM low), so timing-based protections
// see a real delay instead of instant data.
func (u *UPD765) rotationalLatencyTStates() int64 {
	if u.noTiming {
		return 0
	}
	return u.cpuHz * 100 / 1000
}

// SetTick advances the FDC's view of the tick clock. During a read-data phase it
// checks for overrun: if the phase has not completed by the deadline (see
// upd765OverrunTStates), the transfer is terminated abnormally with ST1 bit 4
// (overrun) set, exactly like Fuse's timeout_event.
func (u *UPD765) SetTick(tick int64) {
	u.curTick = tick
	// Complete a pending SEEK/RECALIBRATE: raise the interrupt once the step
	// time has elapsed.
	if u.seekPending && tick >= u.seekCompleteTick {
		u.seekPending = false
		u.intrPending = true
	}
	if u.phase == 1 && !u.writing && u.dataDeadline != 0 && tick > u.dataDeadline {
		u.overrun()
	}
}

// overrun terminates the current read-data phase abnormally with an overrun
// error and moves straight to the result phase.
func (u *UPD765) overrun() {
	u.st0 |= 0x40 // abnormal termination
	st1 := uint8(0x80) | 0x10
	st2 := uint8(0)
	if u.curDisk() != nil {
		if s1, s2, ok := u.curDisk().SectorStatusByID(u.curPCN, u.curHead, u.curTrack, u.curSide, u.curSector); ok {
			st1 |= s1
			st2 = s2 &^ 0x40
			if u.curDisk().SectorDeletedByID(u.curPCN, u.curHead, u.curTrack, u.curSide, u.curSector) != u.delData {
				st2 |= 0x40
			}
		}
	}
	u.result = []byte{u.st0, st1, st2, u.curTrack, u.curSide, u.curSector, u.curN}
	u.resultIdx = 0
	u.phase = 2
	u.dataBuf = nil
	u.dataDeadline = 0
	if upd765Log != nil {
		upd765Log.Debug("upd765 overrun", "track", u.curTrack, "sector", u.curSector, "dataidx", u.dataIdx)
	}
}

// MountDisk mounts a disk image.
func (u *UPD765) MountDisk(disk *Disk) { u.disk = disk }

// MountDiskToDrive mounts a disk into a specific drive (0 = A, 1 = B).
func (u *UPD765) MountDiskToDrive(disk *Disk, drive int) {
	if drive == 1 {
		u.diskB = disk
	} else {
		u.disk = disk
	}
}

// Drives returns the disk in each drive, nil where none is mounted. The slice is
// freshly built, so a caller may keep it.
func (u *UPD765) Drives() []*Disk { return []*Disk{u.disk, u.diskB} }

// curDisk returns the disk selected by the current drive (US bits).
func (u *UPD765) curDisk() *Disk {
	if u.driveSel == 1 && u.diskB != nil {
		return u.diskB
	}
	return u.disk
}

// Reset clears the controller state.
func (u *UPD765) Reset() {
	u.curTrack = 0
	u.curSide = 0
	u.curSector = 0
	u.curPCN = 0
	u.curHead = 0
	u.curN = 2
	u.cmd = 0
	u.params = nil
	u.paramCount = 0
	u.result = nil
	u.resultIdx = 0
	u.dataBuf = nil
	u.dataIdx = 0
	u.writing = false
	u.delData = false
	u.writePlan = nil
	u.writeProtect = false
	u.formatActive = false
	u.formatFill = 0
	u.phase = 0
	u.st0 = 0
	u.pcn = 0
	u.intrPending = false
	u.fddBusy = 0
	u.driveSel = 0
	u.speedlock = 0
	u.lastSectorKey = 0
	u.dataReadyTick = 0
	u.dataDeadline = 0
	u.seekPending = false
	u.seekCompleteTick = 0
}

// HandlesPort reports whether the controller decodes the given port.
// The +3 FDC decodes A15=0, A14=0, A13=1, A1=0; A12 selects status vs data.
// Requiring A14=0 keeps this from colliding with the 0x7FFD paging port.
func (u *UPD765) HandlesPort(port uint16) bool {
	return (port & 0xE002) == 0x2000
}

// mainStatus composes the uPD765 main status register.
func (u *UPD765) mainStatus() uint8 {
	msr := u.fddBusy // FDD busy (seek) bits D0-D3
	switch u.phase {
	case 1: // data transfer (execution phase)
		msr |= 0x10 | 0x20 // CB (busy) + EXECUTION
		if !u.writing {
			msr |= 0x40 // DIO: FDC -> CPU
		}
		// RQM stays low until the target sector has spun under the head; the CPU
		// polls this bit before every byte, so this is what imposes the
		// rotational latency.
		if u.curTick >= u.dataReadyTick {
			msr |= 0x80
		}
	case 2: // result phase
		msr |= 0x10 | 0x40 | 0x80 // CB (busy) + DIO + RQM
	default: // command/parameter phase
		msr |= 0x80 // RQM: ready for a command or parameter byte
	}
	return msr
}

// Read handles the uPD765 ports.
func (u *UPD765) Read(port uint16) uint8 {
	if (port & 0x1000) == 0 {
		// 0x2FFD (A12=0): main status register
		msr := u.mainStatus()
		if msr != u.lastMSR && upd765Log != nil {
			upd765Log.Debug("upd765 msr", "msr", fmt.Sprintf("%02X", msr), "phase", u.phase)
			u.lastMSR = msr
		}
		return msr
	}
	// 0x3FFD (A12=1): data register
	if u.phase == 1 && !u.writing {
		// READ DATA: deliver sector bytes, but only once the rotational latency
		// has elapsed (the sector is under the head).
		if u.curTick >= u.dataReadyTick {
			if u.dataIdx < len(u.dataBuf) {
				b := u.dataBuf[u.dataIdx]
				u.dataIdx++
				if u.dataIdx >= len(u.dataBuf) {
					u.finishData()
				}
				return b
			}
		}
		return 0
	}
	if u.phase == 2 && u.resultIdx < len(u.result) {
		b := u.result[u.resultIdx]
		u.resultIdx++
		if u.resultIdx >= len(u.result) {
			u.phase = 0
			u.result = nil
		}
		return b
	}
	return 0
}

// Write handles the uPD765 ports.
func (u *UPD765) Write(port uint16, value uint8) {
	if (port & 0x1000) == 0 {
		// 0x2FFD (A12=0) write: not used (motor control is via 0x1FFD).
		return
	}
	// 0x3FFD (A12=1): data register
	if u.phase == 1 && u.writing {
		// WRITE DATA: accept sector bytes, but only once the rotational latency
		// has elapsed.
		if u.curTick >= u.dataReadyTick {
			if u.dataIdx < len(u.dataBuf) {
				u.dataBuf[u.dataIdx] = value
				u.dataIdx++
				if u.dataIdx >= len(u.dataBuf) {
					u.finishData()
				}
			}
		}
		return
	}
	// command phase: first byte is the opcode, the rest are parameters.
	if u.cmd == 0 {
		u.cmd = value
		u.paramCount = u.paramCountFor(u.cmd)
		u.params = u.params[:0]
		if u.paramCount == 0 {
			u.execute()
		}
		return
	}
	u.params = append(u.params, value)
	if len(u.params) == u.paramCount {
		u.execute()
	}
}

// paramCountFor returns the number of parameter bytes (excluding the command
// opcode) for a command, per the uPD765 data sheet.
func (u *UPD765) paramCountFor(cmd byte) int {
	switch cmd & 0x1F {
	case 0x03: // SPECIFY
		return 2
	case 0x04: // SENSE DRIVE STATUS
		return 1
	case 0x05, 0x06, 0x09, 0x0C: // WRITE/READ DATA, WRITE/READ DELETED DATA
		return 8
	case 0x0D: // FORMAT TRACK (5 params: US/HD, N, SC, GPL, D)
		return 5
	case 0x07: // RECALIBRATE
		return 1
	case 0x08: // SENSE INTERRUPT STATUS
		return 0
	case 0x0A: // READ ID
		return 1
	case 0x0F: // SEEK
		return 2
	default:
		return 0 // unknown: no params
	}
}

// clearCommand resets the command phase, ready for the next command opcode.
func (u *UPD765) clearCommand() {
	u.cmd = 0
	u.params = nil
	u.paramCount = 0
}

// execute runs the command and sets the result or data-transfer phase.
func (u *UPD765) execute() {
	if upd765Log != nil {
		upd765Log.Debug("upd765 cmd", "cmd", fmt.Sprintf("%02X", u.cmd), "params", u.params)
	}
	switch u.cmd & 0x1F {
	case 0x03: // SPECIFY
		u.clearCommand()
		u.phase = 0
	case 0x07: // RECALIBRATE
		us := u.params[0] & 0x03
		u.driveSel = us
		u.fddBusy |= 1 << us
		steps := int(u.pcn) // steps back to track 0
		u.curTrack = 0
		u.pcn = 0
		u.st0 = 0x20 | us
		u.seekPending = true
		u.seekCompleteTick = u.curTick + u.seekDelayTStates(steps)
		u.clearCommand()
		u.phase = 0
	case 0x0F: // SEEK
		us := u.params[0] & 0x03
		u.driveSel = us
		ncn := u.params[1]
		u.fddBusy |= 1 << us
		// PCN follows the requested cylinder even past the last data track; a
		// later READ ID / READ DATA on such a track reports "no data", which is
		// how Speedlock-style loaders probe the drive geometry.
		var steps int
		if ncn >= u.pcn {
			steps = int(ncn - u.pcn)
		} else {
			steps = int(u.pcn - ncn)
		}
		u.curTrack = ncn
		u.pcn = ncn
		u.st0 = 0x20 | us
		u.seekPending = true
		u.seekCompleteTick = u.curTick + u.seekDelayTStates(steps)
		u.clearCommand()
		u.phase = 0
	case 0x08: // SENSE INTERRUPT STATUS
		if u.intrPending {
			u.result = []byte{u.st0, u.pcn}
			u.intrPending = false
			u.fddBusy = 0
		} else {
			// No pending interrupt: invalid command per the uPD765 spec.
			u.result = []byte{0x80}
		}
		u.resultIdx = 0
		u.phase = 2
		u.clearCommand()
	case 0x04: // SENSE DRIVE STATUS
		// ST3: track 0 + ready.
		st3 := uint8(0x28) // bit 5 (ready) + bit 3 (track 0)
		if u.curTrack != 0 {
			st3 &^= 0x08
		}
		u.result = []byte{st3}
		u.resultIdx = 0
		u.phase = 2
		u.clearCommand()
	case 0x0A: // READ ID
		// Return the current ID field (ST0, ST1, ST2, C, H, R, N). The +3 uses
		// R's high bits to detect the disk format (01-09 = +3, 41-49 = CPC
		// system, C1-C9 = CPC data) and C/H to detect the drive geometry. The
		// unit/head byte carries the head select in bit 2.
		head := (u.params[0] >> 2) & 0x01
		u.driveSel = u.params[0] & 0x03
		u.curSide = head
		// Fuse resets ST0 to the unit/head bits before every data/ID command, so
		// a READ ID never inherits the abnormal (0x40) or seek-end (0x20) bits
		// left over from a previous READ DATA / SEEK.
		u.st0 = u.params[0] & 0x07
		r, ok := uint8(1), false
		n := uint8(2)
		if u.curDisk() != nil {
			r, ok = u.curDisk().FirstSectorID(u.curTrack, head)
			if nn, nok := u.curDisk().FirstSectorN(u.curTrack, head); nok {
				n = nn
			}
		}
		if !ok {
			// No ID field on this cylinder (past the last data track or an
			// unformatted track): ST0 abnormal + ST1 missing-AM / no-data.
			u.result = []byte{u.st0 | 0x40, 0x05, 0x00, u.curTrack, head, 1, 2}
			u.resultIdx = 0
			u.phase = 2
			u.clearCommand()
			if upd765Log != nil {
				upd765Log.Debug("upd765 readid", "no_data", true, "track", u.curTrack, "st0", u.st0|0x40)
			}
			return
		}
		u.result = []byte{
			u.st0,      // ST0: unit/head bits (reset above, per Fuse)
			0x80,       // ST1: end-of-cylinder (always set on +3)
			0x00,       // ST2
			u.curTrack, // C
			head,       // H
			r,          // R: first sector ID (detects the sector-ID convention)
			n,          // N: sector size code (e.g. 6 for protected 8192-byte sectors)
		}
		u.resultIdx = 0
		u.phase = 2
		u.clearCommand()
		if upd765Log != nil {
			upd765Log.Debug("upd765 readid", "no_data", false, "track", u.curTrack, "r", r, "n", n, "st0", u.st0)
		}
	case 0x06, 0x0C: // READ DATA, READ DELETED DATA
		u.speedlockDetect()
		u.delData = u.cmd&0x08 != 0
		u.startReadWrite(false)
	case 0x05, 0x09: // WRITE DATA, WRITE DELETED DATA
		u.delData = u.cmd&0x08 != 0
		u.startReadWrite(true)
	case 0x0D: // FORMAT TRACK
		// params: [US/HD, N, SC, GPL, D]. The CPU then supplies 4 bytes
		// (C, H, R, N) per sector through the DATA register; finishData lays
		// out the freshly formatted track.
		u.curHead = (u.params[0] >> 2) & 1
		u.driveSel = u.params[0] & 0x03
		u.curSide = u.curHead
		u.curTrack = u.pcn
		u.curPCN = u.pcn
		u.curN = u.params[1]
		sc := int(u.params[2])
		u.formatFill = u.params[4]
		u.st0 = u.params[0] & 0x07
		u.formatActive = true
		u.writing = true
		u.delData = false
		u.writePlan = nil
		u.dataBuf = make([]byte, 4*sc)
		u.dataIdx = 0
		u.dataReadyTick = 0 // FORMAT has no rotational latency in this model
		u.phase = 1
		u.clearCommand()
	default:
		// unknown command: invalid, set ST0 and return empty.
		u.result = []byte{0x80} // invalid command
		u.resultIdx = 0
		u.phase = 2
		u.clearCommand()
	}
}

// startReadWrite begins a READ or WRITE DATA command. The +3DOS uses MFM with
// 512-byte sectors (N=2). The uPD765 reads sectors R..EOT in one command; we
// assemble the whole span so custom loaders that bypass the +3DOS (e.g. the
// Speedlock-protected boot loaders) get the full multi-sector transfer.
func (u *UPD765) startReadWrite(writing bool) {
	// params: [US/HD, C, H, R, N, EOT, GPL, DTL]
	pcn := u.pcn                   // physical cylinder (head position, from SEEK)
	head := (u.params[0] >> 2) & 1 // physical side (US/HD bit 2)
	u.driveSel = u.params[0] & 0x03
	c := u.params[1]      // expected ID cylinder
	h := u.params[2]      // expected ID head
	sector := u.params[3] // R (first sector)
	eot := u.params[5]    // EOT (end of track)

	u.curPCN = pcn
	u.curHead = head
	u.curTrack = c
	u.curSide = h
	u.curSector = sector

	u.writePlan = u.writePlan[:0]
	u.writeProtect = false
	if u.curDisk() == nil {
		u.dataBuf = make([]byte, 512) // no disk: zeros
	} else if writing {
		// WRITE DATA: size a buffer for sectors R..EOT (inclusive) and record the
		// per-sector boundaries so finishData commits the CPU-supplied bytes back
		// sector-by-sector. Do not pre-read - the CPU fills the buffer through the
		// data register.
		var total int
		for s := sector; ; s++ {
			size, ok := u.curDisk().SectorSizeByID(pcn, head, c, h, s)
			if !ok {
				break
			}
			u.writePlan = append(u.writePlan, writeTarget{sectorID: s, size: size})
			total += size
			if s == eot {
				break
			}
		}
		if total == 0 {
			// sector not found: ST1 bit 2 (no data) + ST0 abnormal.
			u.result = []byte{0x40, 0x04, 0x00, c, h, sector, 2}
			u.resultIdx = 0
			u.phase = 2
			u.clearCommand()
			return
		}
		u.dataBuf = make([]byte, total)
		if n, ok := u.curDisk().SectorNByID(pcn, head, c, h, sector); ok {
			u.curN = n
		}
	} else {
		// READ DATA: assemble sectors R..EOT (inclusive), matching each sector's ID
		// (C/H/R) at the physical position. Stop early if a sector is missing
		// (unformatted tail); report "no data" only if none were read at all.
		var data []byte
		for s := sector; ; s++ {
			secData, ok := u.curDisk().ReadSectorByID(pcn, head, c, h, s)
			if !ok {
				break
			}
			data = append(data, secData...)
			if s == eot {
				break
			}
		}
		if len(data) == 0 {
			// sector not found: ST1 bit 2 (no data) + ST0 abnormal.
			u.result = []byte{0x40, 0x04, 0x00, c, h, sector, 2}
			u.resultIdx = 0
			u.phase = 2
			u.clearCommand()
			return
		}
		u.dataBuf = data
		if n, ok := u.curDisk().SectorNByID(pcn, head, c, h, sector); ok {
			u.curN = n
		}
		if upd765Log != nil {
			nSize := 128 << (u.params[4] & 0x07)
			upd765Log.Debug("upd765 readwrite", "track", c, "sector", sector, "n", u.params[4], "nsize", nSize, "datalen", len(data))
		}
	}

	// The Speedlock hack perturbs only NON-weak sectors; a weak sector already
	// reads back random via ReadSectorByID, and perturbing it again would
	// conflict (Fuse upd_fdc.c: "do not conflict with fdd weak reads").
	if u.curDisk() == nil || !u.curDisk().SectorWeak(pcn, head, c, h, sector) {
		u.applySpeedlock()
	}

	u.dataIdx = 0
	u.writing = writing
	u.phase = 1
	u.dataReadyTick = u.curTick + u.rotationalLatencyTStates()
	if !writing {
		// Overrun is measured from when the data becomes available, not from
		// command start, so the rotational latency does not eat into the budget.
		u.dataDeadline = u.dataReadyTick + upd765OverrunTStates
	}
	u.st0 = u.params[0] & 0x07 // ST0 device/head bits (US0, US1, HD)
	u.clearCommand()
}

// speedlockDetect tracks consecutive single-sector reads of sector 2 / track 0 /
// head 0 (key 0x200), exactly like Fuse's uPD765 Speedlock hack. The loader
// re-reads that sector and checks that the bytes come back different each time.
func (u *UPD765) speedlockDetect() {
	c := u.params[1] // C (track)
	h := u.params[2] // H (head)
	r := u.params[3] // R (sector)
	eot := u.params[5]

	key := uint(h&1) + (uint(c) << 1) + (uint(r) << 8)
	if r == eot && key == 0x200 {
		if key == u.lastSectorKey {
			u.speedlock++
		} else {
			u.speedlock = 0
			u.lastSectorKey = key
		}
	} else {
		u.speedlock = 0
		u.lastSectorKey = 0
	}
}

// applySpeedlock perturbs the read data to fake weak bits, matching Fuse's
// uPD765 Speedlock "random" sector hack (XOR the byte with its 1-based offset
// at every 29th position).
func (u *UPD765) applySpeedlock() {
	if u.speedlock <= 0 {
		return
	}
	for i := range u.dataBuf {
		dataOffset := i + 1 // Fuse's data_offset is 1-based
		if dataOffset < 64 && u.dataBuf[i] != 0xE5 {
			u.speedlock = 2 // W.E.C Le Mans style
		} else if (u.speedlock > 1 || dataOffset < 64) && dataOffset%29 == 0 {
			u.dataBuf[i] ^= byte(dataOffset)
		}
	}
}

// finishData completes the sector data transfer and moves to the result phase.
func (u *UPD765) finishData() {
	// FORMAT TRACK: dataBuf holds 4*SC bytes of sector IDs (C, H, R, N); lay
	// out the freshly formatted track and finish.
	if u.formatActive {
		u.formatActive = false
		if u.curDisk() != nil && !u.curDisk().ReadOnly {
			u.curDisk().FormatTrack(u.curPCN, u.curHead, u.curN, u.formatFill, u.dataBuf)
		} else if u.curDisk() != nil && u.curDisk().ReadOnly {
			u.writeProtect = true
			u.st0 |= 0x40
		}
		u.result = []byte{u.st0, 0x80, 0x00, u.curTrack, u.curSide, 1, u.curN}
		u.resultIdx = 0
		u.phase = 2
		u.dataBuf = nil
		u.dataDeadline = 0
		return
	}

	// Commit writes sector-by-sector. dataBuf holds the CPU-supplied bytes for
	// the whole R..EOT span; writePlan records each sector's boundary, so each
	// sector is written at its own recorded length instead of the whole buffer
	// being forced through a single-sector write.
	if u.writing && u.curDisk() != nil {
		if u.curDisk().ReadOnly {
			// Write-protected disk: refuse the write and report it via ST1 bit 1
			// (NW) + abnormal ST0, exactly as the FDC would. The CPU-supplied bytes
			// are discarded.
			u.writeProtect = true
			u.st0 |= 0x40 // abnormal termination
		} else {
			off := 0
			last := u.curSector
			for _, wt := range u.writePlan {
				_ = u.curDisk().WriteSectorByID(u.curPCN, u.curHead, u.curTrack, u.curSide, wt.sectorID, u.dataBuf[off:off+wt.size])
				off += wt.size
				last = wt.sectorID
			}
			u.curSector = last // report the last sector's status, as real hardware does
		}
	}
	// Result: ST0, ST1, ST2, C, H, R, N.
	// ST1 bit 7 (end-of-cylinder) is always set on the +3: the ROM's result
	// handler accepts only ST1==0x80 as a clean read (see +3 ROM2 l204a,
	// "bit 7 always set on +3"). ST1/ST2 error bits (e.g. ST1 bit 5 DE = CRC
	// error, ST2 bit 5 DD) come from the image so extended-DSK copy protection
	// (truncated sectors) reports the error the loader expects. ST2 bit 6 (CM)
	// is set only when the sector's deleted-data mark mismatches the command
	// (READ DATA on a deleted sector, or READ DELETED DATA on a normal sector);
	// a matching mark leaves CM clear.
	st1 := uint8(0x80)
	if u.writeProtect {
		st1 |= 0x02 // NW: not writable (write-protected disk)
	}
	st2 := uint8(0)
	if u.curDisk() != nil {
		if s1, s2, ok := u.curDisk().SectorStatusByID(u.curPCN, u.curHead, u.curTrack, u.curSide, u.curSector); ok {
			st1 |= s1
			st2 = s2 &^ 0x40 // recompute CM from the command below
			if u.curDisk().SectorDeletedByID(u.curPCN, u.curHead, u.curTrack, u.curSide, u.curSector) != u.delData {
				st2 |= 0x40
			}
		}
	}
	u.result = []byte{
		u.st0, st1, st2,
		u.curTrack, u.curSide, u.curSector, u.curN,
	}
	u.resultIdx = 0
	u.phase = 2
	u.dataBuf = nil
	u.dataDeadline = 0
}
