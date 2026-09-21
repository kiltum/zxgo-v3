package media

// WD1793Command represents commands for the WD1793 floppy disk controller
type WD1793Command byte

// WD1793 command encoding. The command is selected by the top three bits; see
// commandName for the full table and the comment on the read/write
// sector split is bit 5 rather than bit 6.

// WD1793Status represents the WD1793 status register
type WD1793Status byte

const (
	BetaDiskNotReady     WD1793Status = 0x80
	BetaDiskTypeIOverlap WD1793Status = 0x40
	BetaDiskCRCError     WD1793Status = 0x20
	BetaDiskSeekError    WD1793Status = 0x10
	BetaDiskRecNotFound  WD1793Status = 0x10
	BetaDiskRecordType   WD1793Status = 0x08
	BetaDiskTrack00      WD1793Status = 0x04 // bit2: TRACK00 / LOST DATA
	BetaDiskLostData     WD1793Status = 0x04
	BetaDiskDataRequest  WD1793Status = 0x02
	BetaDiskBusy         WD1793Status = 0x01
)

// sideCompareFlag is Type II command bit 1. On the VG93 it enables side
// compare; together with the multiple-record flag it also drives cross-track
// continuation of a multi-sector transfer.
const sideCompareFlag = 0x02

// WD1793 registers
const (
	WD1793StatusReg  = 0x00
	WD1793TrackReg   = 0x01
	WD1793SectorReg  = 0x02
	WD1793DataReg    = 0x03
	WD1793CommandReg = 0x00
)

// BetaDiskController represents the Beta Disk floppy controller (WD1793)
type BetaDiskController struct {
	status  WD1793Status
	track   uint8
	sector  uint8
	data    uint8
	command WD1793Command

	interruptPending bool
	dataRequest      bool

	// statusTypeI selects which of the two WD1793 status layouts the STATUS
	// register reports. Bits 1, 2 and 5 mean different things after a Type I
	// (seek/step) command than after a Type II/III (read/write) one; see
	// statusByte.
	statusTypeI bool

	// tick is the emulator's cumulative T-state count, pushed in by the main
	// loop before each instruction. It is the only time source the controller
	// has, and the index pulse is derived from it.
	tick  int64
	cpuHz int64

	driveSelect uint8
	sideSelect  uint8
	systemReset bool

	drives      [4]*Disk
	activeDrive int

	state       uint8
	byteCounter int
	buffer      []byte

	// multiSector is set when a READ/WRITE SECTOR command carries the multiple
	// flag (bit 4). When set, the controller transfers consecutive sectors --
	// incrementing the sector register after each -- until a sector ID is not
	// found, at which point it terminates with RECORD NOT FOUND and INTRQ.
	multiSector bool

	// WRITE TRACK state
	writeTrackActive bool

	// writeTrackStartTick is the tick at which the current WRITE TRACK began.
	// On real hardware WRITE TRACK streams until the index pulse (one 200 ms
	// revolution) then raises INTRQ, so a WRITE TRACK that receives no data
	// (or fewer than a full track) still completes instead of hanging.
	writeTrackStartTick int64

	// transferStartTick is the tick at which the current READ/WRITE SECTOR (or
	// READ TRACK / READ ADDRESS) transfer began. If the CPU never reads/writes
	// the data, the FD179x would set LOST DATA and raise INTRQ; the emulator
	// finalizes it after one revolution instead of hanging.
	transferStartTick int64

	// typeIPending marks an in-progress Type I command (RESTORE/SEEK/STEP), which
	// is not instantaneous: the head steps at the r1r0 step rate and settles for
	// 5 step times when the head-load flag is set. typeICompleteTick is the tick
	// at which it finishes (BUSY clears, INTRQ rises).
	typeIPending      bool
	typeICompleteTick int64

	// dataReadyTick is the tick at which a Type II/III command's data becomes
	// available (DRQ rises), after the rotational latency and any delay-bit wait.
	// 0 means no latency is pending. BUSY rises at command time; DRQ is deferred
	// until SetTick reaches this tick.
	dataReadyTick int64

	// noTiming disables the seek/rotational latency so commands complete
	// instantly. Useful for big (non-copy-protected) images that load slowly.
	noTiming bool
}

// Index pulse geometry. A 5.25"/3.5" drive spins at 300 RPM: one revolution
// every 200 ms, with the index hole under the sensor for roughly 4 ms of it.
// Unreal uses the same numbers (wd1793.cpp: (time+tshift) % (Z80FQ/FDD_RPS) <
// Z80FQ*4/1000); Fuse models it as a 10 ms pulse in a 200 ms period.
const (
	indexRevolutionMS = 200
	indexPulseMS      = 4
	defaultCPUHz      = 3500000
)

// stepRatesMS is the Type I (RESTORE/SEEK/STEP) step rate in milliseconds,
// indexed by the r1r0 bits (command bits 1-0), per the WD179x datasheet.
var stepRatesMS = [4]int64{6, 12, 20, 30}

// NewBetaDiskController creates a new Beta Disk controller
func NewBetaDiskController() *BetaDiskController {
	return &BetaDiskController{
		status:      BetaDiskNotReady | BetaDiskTrack00,
		track:       0,
		sector:      1,
		data:        0,
		command:     0,
		state:       0,
		activeDrive: 0,
		statusTypeI: true,
		cpuHz:       defaultCPUHz,
	}
}

// SetTick publishes the emulator's cumulative T-state count. The main loop
// calls this before each instruction, exactly like Beeper.SetTick and
// AY8912.SetTick, so the controller can derive drive rotation from it. It also
// finalizes a WRITE TRACK once a full revolution has elapsed, matching the
// index-pulse completion of the real WD1793.
func (c *BetaDiskController) SetTick(tick int64) {
	c.tick = tick

	// Complete a pending Type I command (RESTORE/SEEK/STEP) once its step rate
	// and head-settling time have elapsed: BUSY clears, INTRQ rises.
	if c.typeIPending && tick >= c.typeICompleteTick {
		c.typeIPending = false
		c.status &^= BetaDiskBusy
		c.interruptPending = true
	}

	// Make a pending Type II/III transfer's data available (DRQ rises) once the
	// rotational latency has elapsed.
	if c.dataReadyTick != 0 && tick >= c.dataReadyTick {
		c.dataReadyTick = 0
		c.dataRequest = true
		c.status |= BetaDiskDataRequest
		c.transferStartTick = tick
	}

	period := c.cpuHz * indexRevolutionMS / 1000
	if period <= 0 {
		return
	}

	if c.writeTrackActive {
		if c.tick-c.writeTrackStartTick >= period {
			c.finalizeWriteTrack()
		}
		return
	}

	// READ/WRITE SECTOR (and READ TRACK / READ ADDRESS): a stalled transfer --
	// the CPU never reads/writes the DATA register -- would set LOST DATA and
	// raise INTRQ on real hardware. Finalize it after one revolution instead of
	// hanging with DRQ asserted forever.
	if c.dataRequest && (c.status&BetaDiskBusy) != 0 && c.tick-c.transferStartTick >= period {
		c.dataRequest = false
		c.status &^= BetaDiskDataRequest
		c.status &^= BetaDiskBusy
		c.status |= BetaDiskLostData
		c.interruptPending = true
		c.buffer = nil
	}
}

// SetClockHz sets the CPU clock the tick counter is measured in (3500000 for
// 48K, 3546900 for 128K). Only the index pulse period depends on it.
func (c *BetaDiskController) SetClockHz(hz int) {
	if hz > 0 {
		c.cpuHz = int64(hz)
	}
}

// SetNoTiming disables the seek/rotational latency so commands complete
// instantly (for big non-copy-protected images that load slowly).
func (c *BetaDiskController) SetNoTiming(on bool) { c.noTiming = on }

// typeIStepTStates returns the per-step delay in T-states for a Type I command,
// from its r1r0 bits (command bits 1-0: 6/12/20/30 ms).
func (c *BetaDiskController) typeIStepTStates(b byte) int64 {
	return c.cpuHz * stepRatesMS[b&0x03] / 1000
}

// typeIDelayTStates returns the total delay in T-states for a Type I command
// that steps the head `steps` tracks: steps times the step rate, plus the
// head-settling time (5 step times) when the head-load flag (h, bit 3) is set.
func (c *BetaDiskController) typeIDelayTStates(b byte, steps int) int64 {
	if c.noTiming {
		return 0
	}
	step := c.typeIStepTStates(b)
	delay := step * int64(steps)
	if b&0x08 != 0 {
		delay += step * 5
	}
	return delay
}

// typeIIILatencyTStates returns how long a Type II/III command waits before its
// data is available: the average rotational latency to find the sector (half a
// revolution) plus a 15 ms command delay when the delay bit (e, bit 2) is set.
func (c *BetaDiskController) typeIIILatencyTStates(b byte) int64 {
	if c.noTiming {
		return 0
	}
	latency := c.cpuHz * int64(indexRevolutionMS/2) / 1000 // ~100 ms rotation
	if b&0x04 != 0 {
		latency += c.cpuHz * 15 / 1000 // 15 ms delay bit
	}
	return latency
}

// indexPulse reports whether the index hole is currently under the sensor.
//
// This is what makes a disk look present to TR-DOS. Its drive-detect routine
// (TR-DOS 5.x at 0x3DAD) issues RESTORE, samples status bit 1, then polls it up
// to 65536 times waiting for the bit to CHANGE:
//
//	ld a,0x08 / call 0x3D9A   ; RESTORE, wait for INTRQ
//	ld de,0x0000
//	in a,(0x1F) / and 0x02 / ld b,a
//	l3dba: in a,(0x1F) / and 0x02 / cp b / ret nz
//	       inc de / ld a,e / or d / jr nz,l3dba
//	jp report_tape_loading_error_and_issue_no_disk_error
//
// A bit that never toggles means a drive that is not spinning, and the routine
// falls through to the "no disk" error. The loop is ~48 T-states per pass, so
// it samples about 3.1 M T-states -- around 15 revolutions -- and any correct
// pulse width is caught.
func (c *BetaDiskController) indexPulse() bool {
	if c.drives[c.activeDrive] == nil {
		return false // no disk: nothing rotating, the bit stays put
	}
	period := c.cpuHz * indexRevolutionMS / 1000
	width := c.cpuHz * indexPulseMS / 1000
	if period <= 0 {
		return false
	}
	return c.tick%period < width
}

// statusByte assembles the STATUS register. The WD1793 reports two different
// layouts depending on the last command, and conflating them is why a
// leftover TRACK00 bit used to show up as LOST DATA after a sector read:
//
//	bit  Type I (seek/step)   Type II/III (read/write)
//	0    BUSY                 BUSY
//	1    INDEX PULSE          DATA REQUEST
//	2    TRACK 00             LOST DATA
//	3    CRC ERROR            CRC ERROR
//	4    SEEK ERROR           RECORD NOT FOUND
//	5    HEAD LOADED          RECORD TYPE
//	6    WRITE PROTECT        WRITE PROTECT
//	7    NOT READY            NOT READY
func (c *BetaDiskController) statusByte() uint8 {
	s := uint8(c.status)
	if c.statusTypeI {
		s &^= 0x02
		if c.indexPulse() {
			s |= 0x02
		}
		s &^= 0x04
		if c.track == 0 {
			s |= 0x04
		}
	}
	return s
}

// MountDisk attaches a disk image to drive 0
func (c *BetaDiskController) MountDisk(disk *Disk) {
	c.MountDiskToDrive(disk, 0)
}

// MountDiskToDrive attaches a disk image to the specified drive (0-3)
func (c *BetaDiskController) MountDiskToDrive(disk *Disk, drive int) {
	if drive < 0 || drive > 3 {
		if diskLog != nil {
			diskLogWarn("invalid drive number", "drive", drive)
		}
		return
	}
	c.drives[drive] = disk
	if diskLog != nil {
		diskLogInfo("disk mounted", "drive", drive, "tracksPerSide", disk.TracksPerSide,
			"sectorsPerTrack", disk.SectorsPerTrack, "sectorSize", disk.SectorSize)
	}

	if drive == c.activeDrive && disk != nil {
		c.status &^= BetaDiskNotReady
	}
}

// ActiveDisk returns the currently selected drive's disk
// Drives returns the disk in each of the four drives, nil where none is
// mounted. The slice is the controller's own, so a caller must not modify it;
// the state coordinator reads it to embed the mounted media in a session.
func (c *BetaDiskController) Drives() []*Disk { return c.drives[:] }

func (c *BetaDiskController) ActiveDisk() *Disk {
	return c.drives[c.activeDrive]
}

// UnmountDisk detaches the disk from the current drive
func (c *BetaDiskController) UnmountDisk() {
	c.drives[c.activeDrive] = nil
	c.status |= BetaDiskNotReady
}

// ReadPort handles reading from WD1793 ports
func (c *BetaDiskController) ReadPort(port uint16) uint8 {
	p := byte(port)

	// SYSTEM register read: any port with bit7=1 (Fuse: beta_sp_read)
	if (p & 0x80) != 0 {
		rqs := uint8(0)
		if c.interruptPending {
			rqs |= 0x80 // INTRQ
		}
		if c.dataRequest {
			rqs |= 0x40 // DRQ
		}
		return rqs | 0x3F
	}

	switch p {
	case 0x1F: // STATUS register
		c.interruptPending = false // reading STATUS clears INTRQ (Fuse: wd_fdc_sr_read)
		return c.statusByte()
	case 0x3F: // TRACK register
		return c.track
	case 0x5F: // SECTOR register
		return c.sector
	case 0x7F: // DATA register
		return c.readData()
	default:
		return 0xFF
	}
}

// readData serves the next byte of a sector read.
//
// The DATA register is the only thing that advances a read: TR-DOS transfers a
// sector with an INI loop that watches the system register and stops on INTRQ
// (TR-DOS 5.x at 0x3FE5):
//
//	l3fe5: in a,(0xFF) / and 0xC0 / jr z,l3fe5   ; wait for DRQ or INTRQ
//	       ret m                                 ; INTRQ -> transfer finished
//	       ini / jr l3fe5                        ; DRQ  -> take one byte
//
// So INTRQ must stay LOW for the whole transfer and rise only after the last
// byte. Asserting DRQ and INTRQ together at command time -- which is what this
// controller used to do -- makes the very first "ret m" succeed and the loop
// returns having moved zero bytes.
func (c *BetaDiskController) readData() uint8 {
	if !c.dataRequest || c.byteCounter >= len(c.buffer) {
		return c.data
	}

	c.data = c.buffer[c.byteCounter]
	c.byteCounter++

	if c.byteCounter >= len(c.buffer) {
		// Last byte of this sector. A multi-sector read continues to the next
		// sector; a single-sector read (or a multi-sector read that has run off
		// the end of the track) finishes here.
		if c.multiSector && c.advanceMultiRead() {
			return c.data
		}

		// Drop DRQ, clear BUSY, raise INTRQ.
		c.dataRequest = false
		c.status &^= BetaDiskDataRequest
		c.status &^= BetaDiskBusy
		c.interruptPending = true
		if diskCtrlLog != nil {
			diskCtrlLogDebug("RD_DATA done", "bytes", len(c.buffer))
		}
	}
	return c.data
}

// advanceMultiRead advances a multi-sector read to the next sector. It returns
// true when the next sector exists and its data has been loaded for transfer;
// otherwise it sets RECORD NOT FOUND and returns false.
func (c *BetaDiskController) advanceMultiRead() bool {
	if !c.advanceSector() {
		return false
	}

	disk := c.ActiveDisk()
	data, err := disk.ReadSector(c.track, c.sideSelect, c.sector)
	if err != nil {
		c.status |= BetaDiskRecNotFound
		return false
	}

	c.buffer = data
	c.byteCounter = 0
	return true
}

// advanceSector increments the sector register for a multi-sector transfer.
// When the side-compare flag (command bit 1) is set, the VG93 steps the head
// across the track boundary: after the last sector it switches sides, and after
// the second side it advances to the next track. Returns false when the transfer
// must end (no side compare at the track end, or the disk ran out).
func (c *BetaDiskController) advanceSector() bool {
	disk := c.ActiveDisk()
	if disk == nil {
		c.status |= BetaDiskNotReady
		return false
	}

	perTrack := uint8(disk.SectorsPerTrack)
	c.sector++
	if c.sector <= perTrack {
		return true
	}

	// Past the last sector of this track. Without side compare the transfer ends
	// here (RECORD NOT FOUND); with it the VG93 continues across the boundary.
	if c.state&sideCompareFlag == 0 {
		c.status |= BetaDiskRecNotFound
		return false
	}

	c.sector = 1
	if c.sideSelect == 0 {
		c.sideSelect = 1
	} else {
		c.sideSelect = 0
		c.track++
		// Past the last cylinder: the disk ends here.
		if c.track >= uint8(disk.TracksPerSide) {
			c.status |= BetaDiskRecNotFound
			return false
		}
	}
	return true
}

// WritePort handles writing to WD1793 ports
func (c *BetaDiskController) WritePort(port uint16, value uint8) {
	p := byte(port)

	// SYSTEM register write: any port with bit7=1 (Fuse: beta_sp_write)
	if (p & 0x80) != 0 {
		c.driveSelect = value & 0x03
		c.activeDrive = int(c.driveSelect)

		// Side select is inverted: Fuse fdd_set_head(d, (b & 0x10) ? 0 : 1),
		// Unreal side = 1 & ~(v >> 4).
		if (value & 0x10) != 0 {
			c.sideSelect = 0
		} else {
			c.sideSelect = 1
		}

		// Bit 2 low resets the controller (Unreal: if(!(v & 0x04)) ...).
		if (value & 0x04) == 0 {
			c.systemReset = true
			c.status = BetaDiskNotReady
			c.statusTypeI = true
			c.interruptPending = true
			c.dataRequest = false
			c.state = 0
			c.byteCounter = 0
			c.buffer = nil
		} else {
			c.systemReset = false
		}

		if c.drives[c.activeDrive] != nil {
			c.status &^= BetaDiskNotReady
		} else {
			c.status |= BetaDiskNotReady
		}

		if diskCtrlLog != nil {
			diskCtrlLogDebug("WR_SYS", "val", value, "drv", c.activeDrive,
				"side", c.sideSelect, "dden", (value&0x20) != 0,
				"hlt", (value&0x08) != 0, "notrdy", (c.status&BetaDiskNotReady) != 0)
		}
		return
	}

	switch p {
	case 0x1F: // COMMAND register
		// The full port is logged because "out (0x1F),a" puts A on the high
		// address byte too: a command whose high byte does not match its value
		// came from "out (c),r" and so from a different piece of code.
		// diskCtrlLogDebug("WD1793 port write", "port", port, "val", value)
		c.ExecuteCommand(WD1793Command(value))
	case 0x3F: // TRACK register
		c.track = value
	case 0x5F: // SECTOR register
		c.sector = value
	case 0x7F: // DATA register
		c.data = value
		c.handleDataWrite()
	}
}

// commandName decodes a WD179x command byte to its mnemonic. The command is
// selected by the top three bits, per the WD179x datasheet:
//
//	0xxxxxxx  Type I    RESTORE / SEEK / STEP
//	100xxxxx  Type II   READ SECTOR
//	101xxxxx  Type II   WRITE SECTOR
//	1100xxxx  Type III  READ ADDRESS
//	1101xxxx  Type IV   FORCE INTERRUPT
//	1110xxxx  Type III  READ TRACK
//	1111xxxx  Type III  WRITE TRACK
func commandName(b byte) string {
	switch {
	case b&0x80 == 0x00:
		switch {
		case b&0x10 == 0:
			return "RESTORE"
		case b&0x40 == 0:
			return "SEEK"
		case b&0x20 == 0:
			return "STEP-IN"
		default:
			return "STEP-OUT"
		}
	case b&0xE0 == 0x80:
		return "READ_SECTOR"
	case b&0xE0 == 0xA0:
		return "WRITE_SECTOR"
	case b&0xF0 == 0xC0:
		return "READ_ADDR"
	case b&0xF0 == 0xD0:
		return "FORCE_INT"
	case b&0xF0 == 0xE0:
		return "READ_TRACK"
	default:
		return "WRITE_TRACK"
	}
}

// ExecuteCommand processes a WD1793 command.
//
// The previous dispatch chain got two ranges wrong. WRITE SECTOR was
// unreachable -- its guard was "b&0xC0 == 0xA0", which no byte can satisfy --
// so 0xA0-0xBF fell into the READ SECTOR case above it and every disk write
// was executed as a read. And 0xE0-0xFF matched nothing at all, landing in the
// default case, which left INTRQ low: TR-DOS then waits for INTRQ forever
// ("in a,(0xFF) / and 0x80 / jr z,...") and the machine hangs.
func (c *BetaDiskController) ExecuteCommand(cmd WD1793Command) {
	c.command = cmd
	b := byte(cmd)
	name := commandName(b)

	switch name {
	case "RESTORE", "SEEK", "STEP-IN", "STEP-OUT":
		c.executeTypeI(b)
	case "READ_SECTOR":
		c.executeReadSector(b)
	case "WRITE_SECTOR":
		c.executeWriteSector(b)
	case "FORCE_INT":
		c.executeForceInterrupt()
	default: // READ_ADDR, READ_TRACK, WRITE_TRACK
		c.executeTypeIII(b)
	}

	if diskCtrlLog != nil {
		diskCtrlLogDebug("WD1793 exec", "cmd", name, "trk", c.track, "sec", c.sector,
			"side", c.sideSelect, "int", c.interruptPending, "drq", c.dataRequest,
			"busy", (c.status&BetaDiskBusy) != 0, "buf", len(c.buffer))
	}
}

// executeTypeI handles RESTORE / SEEK / STEP. These are not instantaneous: the
// head steps at the r1r0 step rate and settles for 5 step times when the
// head-load flag (h, bit 3) is set. The track register updates immediately (the
// head position is all this controller models), but BUSY stays set and INTRQ
// rises only once SetTick reaches typeICompleteTick. TRACK00 (status bit 2) and
// the index pulse (bit 1) are composed on read by statusByte.
func (c *BetaDiskController) executeTypeI(b byte) {
	c.statusTypeI = true
	c.status |= BetaDiskBusy
	c.status &^= BetaDiskSeekError
	c.dataRequest = false
	c.status &^= BetaDiskDataRequest
	c.interruptPending = false

	var steps int
	switch {
	case b&0x10 == 0:
		steps = int(c.track) // RESTORE: wind back to track 0
		c.track = 0
	case b&0x40 == 0:
		if c.data >= c.track {
			steps = int(c.data - c.track)
		} else {
			steps = int(c.track - c.data)
		}
		c.track = c.data // SEEK: target track comes from the DATA register
	case b&0x20 == 0:
		if c.track < 85 { // STEP IN: towards the spindle
			c.track++
			steps = 1
		}
	default:
		if c.track > 0 { // STEP OUT: towards track 0
			c.track--
			steps = 1
		}
	}

	c.typeIPending = true
	c.typeICompleteTick = c.tick + c.typeIDelayTStates(b, steps)
}

func (c *BetaDiskController) executeReadSector(b byte) {
	c.statusTypeI = false
	c.state = b
	c.multiSector = (b & 0x10) != 0
	c.byteCounter = 0
	c.buffer = nil
	c.dataRequest = false
	c.status &^= BetaDiskDataRequest
	c.status &^= BetaDiskRecNotFound
	c.status &^= BetaDiskLostData

	disk := c.ActiveDisk()
	if disk == nil {
		diskLogWarn("READ_SECTOR: no disk in drive", "drv", c.activeDrive)
		c.status |= BetaDiskNotReady
		c.status &^= BetaDiskBusy
		c.interruptPending = true
		return
	}

	data, err := disk.ReadSector(c.track, c.sideSelect, c.sector)
	if err != nil {
		diskLogWarn("READ_SECTOR failed", "trk", c.track, "side", c.sideSelect, "sec", c.sector, "err", err)
		c.status |= BetaDiskRecNotFound
		c.status &^= BetaDiskBusy
		c.interruptPending = true
		return
	}

	// Command accepted: BUSY rises now, but DRQ is deferred until the rotational
	// latency elapses (the FDC must wait for the sector to spin under the head).
	c.buffer = data
	c.byteCounter = 0
	c.dataRequest = false
	c.status &^= BetaDiskDataRequest
	c.status |= BetaDiskBusy
	c.interruptPending = false
	c.dataReadyTick = c.tick + c.typeIIILatencyTStates(b)
}

func (c *BetaDiskController) executeWriteSector(b byte) {
	c.statusTypeI = false
	c.state = b
	c.multiSector = (b & 0x10) != 0
	c.byteCounter = 0
	c.buffer = nil
	c.dataRequest = false
	c.status &^= BetaDiskDataRequest
	c.status &^= BetaDiskRecNotFound
	c.status &^= BetaDiskLostData

	disk := c.ActiveDisk()
	if disk == nil {
		diskLogWarn("WRITE_SECTOR: no disk in drive", "drv", c.activeDrive)
		c.status |= BetaDiskNotReady
		c.status &^= BetaDiskBusy
		c.interruptPending = true
		return
	}
	if disk.ReadOnly {
		c.status |= WD1793Status(0x40) // WRITE PROTECT
		c.status &^= BetaDiskBusy
		c.interruptPending = true
		return
	}

	c.buffer = make([]byte, disk.SectorSize)
	c.dataRequest = false
	c.status &^= BetaDiskDataRequest
	c.status |= BetaDiskBusy
	c.interruptPending = false
	c.dataReadyTick = c.tick + c.typeIIILatencyTStates(b)
}

func (c *BetaDiskController) executeTypeIII(b byte) {
	c.statusTypeI = false

	// READ ADDRESS (0xC0-0xCF)
	if (b & 0xF0) == 0xC0 {
		c.executeReadAddress()
		return
	}

	// READ TRACK (0xE0-0xEF)
	if (b & 0xF0) == 0xE0 {
		c.executeReadTrack()
		return
	}

	// WRITE TRACK / FORMAT (0xF0-0xFF)
	if (b & 0xF0) == 0xF0 {
		c.executeWriteTrack()
		return
	}

	// Unknown Type III command
	diskLogWarn("WD1793 unknown Type III", "cmd", commandName(b))
	c.status &^= BetaDiskBusy
	c.dataRequest = false
	c.status &^= BetaDiskDataRequest
	c.interruptPending = true
}

// executeReadTrack implements READ TRACK (0xE0-0xEF).
// Synthesizes a standard MFM track image from decoded sectors and streams it.
func (c *BetaDiskController) executeReadTrack() {
	disk := c.ActiveDisk()
	if disk == nil {
		c.status |= BetaDiskNotReady
		c.status &^= BetaDiskBusy
		c.interruptPending = true
		return
	}

	// Synthesize standard MFM track from sectors
	trackData := synthesizeTrackMFM(disk, c.track, c.sideSelect)

	c.buffer = trackData
	c.byteCounter = 0
	c.dataRequest = true
	c.transferStartTick = c.tick
	c.status |= BetaDiskDataRequest
	c.status |= BetaDiskBusy
	c.status &^= BetaDiskLostData
	c.status &^= BetaDiskCRCError
	c.interruptPending = false

	if diskLog != nil {
		diskLogInfo("READ_TRACK", "trk", c.track, "side", c.sideSelect, "bytes", len(trackData))
	}
}

// executeWriteTrack implements WRITE TRACK (0xF0-0xFF) and FORMAT TRACK.
// Receives raw MFM track data, parses it, and writes decoded sectors to disk.
//
// TR-DOS FORMAT sends special bytes:
//
//	0xF5 -> 0xA1 with clock mark (start CRC calculation)
//	0xF6 -> 0xC2 with clock mark
//	0xF7 -> write CRC, stop CRC calculation
//	Other bytes -> write as-is
func (c *BetaDiskController) executeWriteTrack() {
	disk := c.ActiveDisk()
	if disk == nil {
		c.status |= BetaDiskNotReady
		c.status &^= BetaDiskBusy
		c.interruptPending = true
		return
	}
	if disk.ReadOnly {
		c.status |= WD1793Status(0x40) // WRITE PROTECT
		c.status &^= BetaDiskBusy
		c.interruptPending = true
		return
	}

	// Prepare to receive track data from DATA register writes
	c.buffer = make([]byte, 0, 6400)
	c.byteCounter = 0
	c.dataRequest = true
	c.status |= BetaDiskDataRequest
	c.status |= BetaDiskBusy
	c.status &^= BetaDiskLostData
	c.interruptPending = false
	c.writeTrackActive = true
	c.writeTrackStartTick = c.tick

	if diskLog != nil {
		diskLogInfo("WRITE_TRACK start", "trk", c.track, "side", c.sideSelect)
	}
}

// finalizeWriteTrack is called when WRITE TRACK completes (track buffer filled
// or index pulse returns). Parses the MFM stream and writes decoded sectors.
func (c *BetaDiskController) finalizeWriteTrack() {
	disk := c.ActiveDisk()
	if disk == nil {
		c.status &^= BetaDiskBusy
		c.interruptPending = true
		c.writeTrackActive = false
		return
	}

	// Parse MFM stream and extract sectors
	sectors := parseTrackMFM(c.buffer, c.track, c.sideSelect)

	if diskLog != nil {
		diskLogInfo("WRITE_TRACK parse", "trk", c.track, "side", c.sideSelect,
			"bytes", len(c.buffer), "sectors", len(sectors))
	}

	// Write sectors to disk
	for _, sec := range sectors {
		if err := disk.WriteSector(sec.Track, sec.Side, sec.SectorID, sec.Data); err != nil {
			if diskLog != nil {
				diskLogWarn("WRITE_TRACK sector failed", "trk", sec.Track,
					"side", sec.Side, "sec", sec.SectorID, "err", err)
			}
		}
	}

	c.dataRequest = false
	c.status &^= BetaDiskDataRequest
	c.status &^= BetaDiskBusy
	c.interruptPending = true
	c.writeTrackActive = false
	c.buffer = nil
}

// crcCCITT computes the MFM ID-field CRC (x^16 + x^12 + x^5 + 1) with the
// 0xCDB4 seed that follows the three 0xA1 sync bytes and the 0xFE address mark
// (Unreal wd1793.cpp: crc_initial = 0xcdb4).
func crcCCITT(data []byte) uint16 {
	crc := uint16(0xCDB4)
	for _, v := range data {
		crc ^= uint16(v) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// sectorIDCRC returns the ID-field CRC for a sector, computing and caching it on
// first use so READ ADDRESS reports a stored per-sector CRC rather than a fresh
// synthesis each time. The MFM ID field is [track, side, sector, length code].
func sectorIDCRC(sec *DiskSector, lengthCode byte) uint16 {
	if sec.IDCRC != 0 {
		return sec.IDCRC
	}
	id := []byte{sec.Track, sec.Side, sec.SectorID, lengthCode}
	sec.IDCRC = crcCCITT(id)
	return sec.IDCRC
}

// executeReadAddress implements the Type III READ ADDRESS command: the six
// bytes of the next ID field -- track, head, sector, length code, CRC high,
// CRC low -- are handed over through the DATA register under DRQ, and the
// SECTOR register is loaded with the track address that was found
// (Fuse wd_fdc.c wd_fdc_dr_read, WD_FDC_STATE_READID).
//
// TR-DOS uses this to identify a disk, and it is the second route to a "no
// disk" error: its handler at 0x3EB5 polls the system register 3*65536 times
// waiting for DRQ or INTRQ, then gives up:
//
//	ld a,0xC0 / out (0x1F),a
//	l3ece: in a,(0xFF) / and 0xC0 / jr nz,l3ef2
//	       inc de / ld a,e / or d / jr nz,l3ece
//	       djnz l3ece
//	... jp report_tape_loading_error_and_issue_no_disk_error
//
// A stub that only raised INTRQ satisfied the poll but handed back a garbage
// track ID, so the caller decided the disk was unreadable.
func (c *BetaDiskController) executeReadAddress() {
	c.byteCounter = 0
	c.buffer = nil
	c.dataRequest = false
	c.status &^= BetaDiskDataRequest
	c.status &^= BetaDiskRecNotFound
	c.status &^= BetaDiskLostData

	disk := c.ActiveDisk()
	if disk == nil {
		c.status |= BetaDiskNotReady
		c.status &^= BetaDiskBusy
		c.interruptPending = true
		return
	}

	// Any sector on the current track will do: TR-DOS wants the track and side
	// the head is actually over, which is what identifies the format.
	sectorID := c.sector
	sec := disk.sectorRef(c.track, c.sideSelect, sectorID)
	if sec == nil {
		sectorID = 1
		sec = disk.sectorRef(c.track, c.sideSelect, sectorID)
		if sec == nil {
			diskLogWarn("READ_ADDRESS: no ID field", "trk", c.track, "side", c.sideSelect)
			c.status |= BetaDiskRecNotFound
			c.status &^= BetaDiskBusy
			c.interruptPending = true
			return
		}
	}

	// Length code: 0 = 128, 1 = 256, 2 = 512, 3 = 1024 bytes.
	lengthCode := byte(1)
	switch disk.SectorSize {
	case 128:
		lengthCode = 0
	case 256:
		lengthCode = 1
	case 512:
		lengthCode = 2
	case 1024:
		lengthCode = 3
	}

	id := []byte{c.track, c.sideSelect, sectorID, lengthCode}
	crc := sectorIDCRC(sec, lengthCode)
	c.buffer = append(id, byte(crc>>8), byte(crc))

	// The found track address is copied into the SECTOR register.
	c.sector = c.track

	c.dataRequest = true
	c.transferStartTick = c.tick
	c.status |= BetaDiskDataRequest
	c.status |= BetaDiskBusy
	c.interruptPending = false
}

func (c *BetaDiskController) executeForceInterrupt() {
	// Fuse wd_fdc.c wd_fdc_cr_write Type IV:
	//   status &= ~(BUSY | WRPROT | CRCERR | IDX_DRQ)
	//   state = NONE, reset DRQ, set INTRQ
	//   NEVER sets NOT_READY -- that is determined by disk_ready()
	// Force Interrupt also puts the status register back into Type I layout.
	c.statusTypeI = true
	c.status &^= BetaDiskBusy
	c.status &^= BetaDiskDataRequest
	c.status &^= BetaDiskRecNotFound
	c.dataRequest = false
	c.interruptPending = true
	c.state = 0
	c.byteCounter = 0
	c.buffer = nil
}

// handleDataWrite absorbs one byte of a WRITE SECTOR transfer. It mirrors
// readData: DRQ stays up until the sector is full, then the sector is committed
// and INTRQ rises.
func (c *BetaDiskController) handleDataWrite() {
	// WRITE TRACK: accumulate bytes until track complete
	if c.writeTrackActive {
		c.buffer = append(c.buffer, c.data)
		c.dataRequest = true
		c.status |= BetaDiskDataRequest

		// Check if track complete (simplified: 6250 bytes = full track)
		// In real hardware, this continues until the index pulse returns
		if len(c.buffer) >= 6250 {
			c.finalizeWriteTrack()
		}
		return
	}

	// WRITE SECTOR: original logic
	if (c.state & 0xE0) != 0xA0 {
		return
	}
	if !c.dataRequest || c.byteCounter >= len(c.buffer) {
		return
	}

	c.buffer[c.byteCounter] = c.data
	c.byteCounter++

	// Clear DRQ after accepting byte
	c.dataRequest = false
	c.status &^= BetaDiskDataRequest

	// If more bytes needed, set DRQ again
	if c.byteCounter < len(c.buffer) {
		c.dataRequest = true
		c.status |= BetaDiskDataRequest
		return
	}

	// Transfer complete: commit this sector.
	disk := c.ActiveDisk()
	writeOK := false
	if disk == nil {
		c.status |= BetaDiskNotReady
	} else if err := disk.WriteSector(c.track, c.sideSelect, c.sector, c.buffer); err != nil {
		diskLogWarn("WRITE_SECTOR failed", "trk", c.track, "side", c.sideSelect, "sec", c.sector, "err", err)
		c.status |= BetaDiskRecNotFound
	} else {
		diskLogInfo("WRITE_SECTOR ok", "trk", c.track, "side", c.sideSelect, "sec", c.sector, "bytes", len(c.buffer))
		writeOK = true
	}

	// A multi-sector write continues to the next sector; only a failed write or
	// a missing next sector ends the command here.
	if writeOK && c.multiSector && c.advanceMultiWrite() {
		// Next sector ready: keep BUSY, raise DRQ for its first byte.
		c.dataRequest = true
		c.status |= BetaDiskDataRequest
		return
	}

	c.byteCounter = 0
	c.status &^= BetaDiskBusy
	c.interruptPending = true
}

// advanceMultiWrite advances a multi-sector write to the next sector. It returns
// true when the next sector exists and a fresh buffer is ready; otherwise it
// sets RECORD NOT FOUND and returns false.
func (c *BetaDiskController) advanceMultiWrite() bool {
	if !c.advanceSector() {
		return false
	}

	disk := c.ActiveDisk()
	if disk.sectorRef(c.track, c.sideSelect, c.sector) == nil {
		c.status |= BetaDiskRecNotFound
		return false
	}

	c.buffer = make([]byte, disk.SectorSize)
	c.byteCounter = 0
	return true
}

func (c *BetaDiskController) GetInterruptRequest() bool { return c.interruptPending }
func (c *BetaDiskController) ClearInterrupt()           { c.interruptPending = false }
func (c *BetaDiskController) GetStatus() uint8          { return c.statusByte() }
func (c *BetaDiskController) GetTrack() uint8           { return c.track }
func (c *BetaDiskController) GetSector() uint8          { return c.sector }

func (c *BetaDiskController) Reset() {
	c.status = BetaDiskNotReady
	c.track = 0
	c.sector = 1
	c.data = 0
	c.command = 0
	c.state = 0
	c.statusTypeI = true
	c.byteCounter = 0
	c.buffer = nil
	c.multiSector = false
	c.writeTrackActive = false
	c.writeTrackStartTick = 0
	c.transferStartTick = 0
	c.typeIPending = false
	c.typeICompleteTick = 0
	c.dataReadyTick = 0
	c.interruptPending = false
	c.dataRequest = false
	c.activeDrive = 0
	c.driveSelect = 0
	c.sideSelect = 0

	// A reset does not eject the disks, so NOT_READY must reflect what is
	// still in the selected drive.
	if c.drives[c.activeDrive] != nil {
		c.status &^= BetaDiskNotReady
	}
}

// synthesizeTrackMFM generates a standard MFM track image from decoded sectors.
// This is used by READ TRACK to stream a plausible track structure.
//
// Standard TR-DOS track format (16 sectors of 256 bytes):
//
//	GAP1: 80x0x4E (pre-index)
//	For each sector (1-16):
//	  SYNC: 12x0x00, 3x0xA1 (with clock marks), 0xFE (ID Address Mark)
//	  ID: C, H, R, N (cylinder, head, record, size code)
//	  CRC: 2 bytes (ID field CRC)
//	  GAP2: 22x0x4E (ID-to-data gap)
//	  SYNC: 12x0x00, 3x0xA1, 0xFB (Data Address Mark)
//	  DATA: 256 bytes
//	  CRC: 2 bytes (data CRC)
//	  GAP3: 24x0x4E (inter-sector gap)
//	GAP4: fill rest with 0x4E
func synthesizeTrackMFM(disk *Disk, track, side uint8) []byte {
	sectorSize := disk.SectorSize
	if sectorSize <= 0 {
		sectorSize = 256
	}
	numSectors := disk.SectorsPerTrack
	if numSectors <= 0 {
		numSectors = 16
	}
	// Size code N: 128 << N == sectorSize.
	sizeCode := byte(1)
	switch sectorSize {
	case 128:
		sizeCode = 0
	case 256:
		sizeCode = 1
	case 512:
		sizeCode = 2
	case 1024:
		sizeCode = 3
	}

	// Per-sector MFM overhead is 86 bytes (16 ID sync+mark, 4 ID, 2 ID CRC, 22
	// GAP2, 16 data sync+mark, 2 data CRC, 24 GAP3). GAP1 is 80 bytes and GAP4 is
	// 700, so a 16x256 TR-DOS track stays ~6250 bytes as before.
	trackSize := 80 + numSectors*(86+sectorSize) + 700
	buf := make([]byte, trackSize)
	pos := 0

	// GAP1: pre-index gap
	for i := 0; i < 80; i++ {
		buf[pos] = 0x4E
		pos++
	}

	// Write the disk's sectors (not a hardcoded 16).
	for secID := uint8(1); secID <= uint8(numSectors); secID++ {
		// ID Address Mark
		for i := 0; i < 12; i++ {
			buf[pos] = 0x00
			pos++
		}
		for i := 0; i < 3; i++ {
			buf[pos] = 0xA1 // In real hardware, these have special clock marks
			pos++
		}
		buf[pos] = 0xFE // ID Address Mark
		pos++

		// ID field: C, H, R, N
		idStart := pos
		buf[pos] = track
		pos++
		buf[pos] = side
		pos++
		buf[pos] = secID
		pos++
		buf[pos] = sizeCode // N: sector size code (128 << N bytes)
		pos++

		// ID CRC
		crc := crcCCITT(buf[idStart:pos])
		buf[pos] = byte(crc >> 8)
		pos++
		buf[pos] = byte(crc)
		pos++

		// GAP2: ID-to-data gap
		for i := 0; i < 22; i++ {
			buf[pos] = 0x4E
			pos++
		}

		// Data Address Mark
		for i := 0; i < 12; i++ {
			buf[pos] = 0x00
			pos++
		}
		for i := 0; i < 3; i++ {
			buf[pos] = 0xA1
			pos++
		}
		buf[pos] = 0xFB // Data Address Mark (normal data)
		pos++

		// Sector data
		dataStart := pos
		secData, err := disk.ReadSector(track, side, secID)
		if err == nil && len(secData) == sectorSize {
			copy(buf[pos:], secData)
		} else {
			// Empty sector (formatted but no data)
			for i := 0; i < sectorSize; i++ {
				buf[pos+i] = 0xE5 // TR-DOS empty byte
			}
		}
		pos += sectorSize

		// Data CRC
		crc = crcCCITT(buf[dataStart:pos])
		buf[pos] = byte(crc >> 8)
		pos++
		buf[pos] = byte(crc)
		pos++

		// GAP3: inter-sector gap
		for i := 0; i < 24; i++ {
			buf[pos] = 0x4E
			pos++
		}
	}

	// GAP4: fill rest with 0x4E
	for pos < trackSize {
		buf[pos] = 0x4E
		pos++
	}

	return buf
}

// parseTrackMFM extracts sectors from a raw MFM track image written by WRITE TRACK.
// Returns decoded sectors that can be written to the disk.
func parseTrackMFM(trackData []byte, track, side uint8) []DiskSector {
	sectors := []DiskSector{}

	// Scan for ID Address Marks (0xA1, 0xA1, 0xA1, 0xFE)
	for i := 0; i < len(trackData)-4; i++ {
		if trackData[i] == 0xA1 && trackData[i+1] == 0xA1 &&
			trackData[i+2] == 0xA1 && trackData[i+3] == 0xFE {
			// Found ID mark
			if i+4+4 > len(trackData) {
				break
			}
			idField := trackData[i+4 : i+4+4] // C, H, R, N
			sectorID := idField[2]
			sizeCode := idField[3] & 3
			sectorSize := 128 << sizeCode

			// Find Data Address Mark after ID (within next 100 bytes)
			dataStart := findDataMark(trackData, i+4+4, 100)
			if dataStart == -1 || dataStart+sectorSize > len(trackData) {
				continue
			}

			sectorData := make([]byte, sectorSize)
			copy(sectorData, trackData[dataStart:dataStart+sectorSize])

			sectors = append(sectors, DiskSector{
				Track:    track,
				Side:     side,
				SectorID: sectorID,
				Data:     sectorData,
			})

			// Skip past this sector to avoid re-detecting
			i = dataStart + sectorSize
		}
	}

	return sectors
}

// findDataMark searches for a Data Address Mark (0xA1, 0xA1, 0xA1, 0xFB/0xF8)
// starting from the given position, within maxDistance bytes.
func findDataMark(data []byte, start, maxDistance int) int {
	end := start + maxDistance
	if end > len(data) {
		end = len(data)
	}

	for i := start; i < end-4; i++ {
		if data[i] == 0xA1 && data[i+1] == 0xA1 && data[i+2] == 0xA1 {
			// 0xFB = normal data, 0xF8 = deleted data
			if data[i+3] == 0xFB || data[i+3] == 0xF8 {
				return i + 4
			}
		}
	}
	return -1
}
