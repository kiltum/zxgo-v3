package media

import "fmt"

// FDCState is what a front end shows about a floppy controller: which disk is in the
// selected drive, where the head is, what it was last told to do, and the status lines
// a user watching a load wants to see (UI_DESIGN.md section 6.2).
//
// One struct for two controllers that share almost nothing. The WD1793 reports a single
// status byte whose bits change meaning with the command type; the uPD765 has a main
// status register with a layout of its own and a separate result phase. What the two do
// share is what a user asks about, so the names here are the user's and each
// StateSnapshot is where the controllers disagree.
type FDCState struct {
	// Controller names the chip: "WD1793" (the Beta Disk) or "uPD765" (the +2A/+3).
	Controller string
	// Command is the last command, named where there is a name for it and as a byte
	// where there is not.
	Command string

	// Disk is what is in the selected drive, and Ready says whether there is anything
	// there at all. The rest of the disk fields are zero when it is false.
	Disk     DiskInfo
	DiskPath string
	Ready    bool
	Drive    int

	// Track and Sector are where the head is: the physical position, not the identity
	// a sector's header claims, which copy-protected disks deliberately get wrong.
	Track  uint8
	Sector uint8

	// The status lines. Which of them can be raised at once depends on the controller
	// and on the command type - see each StateSnapshot - so a front end draws the ones
	// its controller can set.
	Busy           bool
	DRQ            bool
	INTRQ          bool
	LostData       bool
	WriteProtected bool
	NotReady       bool
	TrackZero      bool
	RecordNotFound bool
	CRCError       bool

	// IndexPhase is how far the rotation is through one revolution, 0 to 1, or -1 when
	// there is no rotation to report. The emulator has no motor-on state: the drive
	// turns whenever a disk is in it, so this is a phase rather than a speed.
	IndexPhase float64
}

// FDCStater is implemented by both floppy controllers, so a front end can ask either
// one what it is doing without knowing which it has (UI_DESIGN.md section 6.2).
type FDCStater interface {
	StateSnapshot() FDCState
}

// StateSnapshot reports the controller's state as plain data.
func (c *BetaDiskController) StateSnapshot() FDCState {
	s := FDCState{
		Controller: "WD1793",
		Command:    commandName(byte(c.command)),
		Drive:      c.activeDrive,
		Track:      c.track,
		Sector:     c.sector,
		INTRQ:      c.interruptPending,
		IndexPhase: c.indexPhase(),
	}

	if disk := c.drives[c.activeDrive]; disk != nil {
		s.Disk = disk.GetDetailedInfo()
		s.Ready = true
		s.WriteProtected = disk.ReadOnly
	}

	// The bits mean different things in the two layouts the WD1793 reports, which this
	// controller's own statusByte documents:
	//
	//	bit  Type I (seek/step)   Type II/III (read/write)
	//	1    INDEX PULSE          DATA REQUEST
	//	2    TRACK 00             LOST DATA
	//	4    SEEK ERROR           RECORD NOT FOUND
	//
	// So only the lines that mean the same thing in both - BUSY, CRC ERROR, WRITE
	// PROTECT, NOT READY - are read unconditionally, and the rest are read from the
	// layout the controller says it is in. Reporting TRACK 00 after a sector read is
	// how the bit at 2 shows up as a phantom LOST DATA.
	status := c.statusByte()
	s.Busy = status&byte(BetaDiskBusy) != 0
	s.CRCError = status&byte(BetaDiskCRCError) != 0
	s.NotReady = status&byte(BetaDiskNotReady) != 0

	if c.statusTypeI {
		s.TrackZero = status&0x04 != 0
	} else {
		s.DRQ = status&byte(BetaDiskDataRequest) != 0
		s.LostData = status&byte(BetaDiskLostData) != 0
		s.RecordNotFound = status&byte(BetaDiskRecNotFound) != 0
	}
	return s
}

// indexPhase is how far the rotation is through one revolution, 0 to 1, or -1 when no
// disk is in the selected drive. It is computed from the same tick and period the index
// pulse is, so the read-out and the status bit cannot disagree - and it is the status
// bit that TR-DOS's drive detection polls (see indexPulse).
func (c *BetaDiskController) indexPhase() float64 {
	if c.drives[c.activeDrive] == nil {
		return -1
	}
	period := c.cpuHz * indexRevolutionMS / 1000
	if period <= 0 {
		return -1
	}
	return float64(c.tick%period) / float64(period)
}

// StateSnapshot reports the controller's state as plain data.
func (u *UPD765) StateSnapshot() FDCState {
	s := FDCState{
		Controller: "uPD765",
		Command:    upd765CommandName(u.cmd),
		Drive:      int(u.driveSel),
		Track:      u.curPCN,
		Sector:     u.curSector,
		// The uPD765 has no rotation phase of its own: its rotational latency is a
		// deadline for the current transfer rather than a continuous position.
		IndexPhase: -1,
	}

	if disk := u.curDisk(); disk != nil {
		s.Disk = disk.GetDetailedInfo()
		s.Ready = true
		s.WriteProtected = disk.ReadOnly
	}
	s.NotReady = !s.Ready

	// The main status register, whose layout is nothing like the WD1793's: RQM says a
	// byte can be moved, CB says a command is executing, and the phase says which
	// direction.
	msr := u.mainStatus()
	s.Busy = msr&0x10 != 0
	// A byte is ready to move during the transfer and result phases - either direction,
	// which is why DIO is not part of it.
	s.DRQ = (u.phase == 1 || u.phase == 2) && msr&0x80 != 0
	// INTRQ after a seek or a recalibrate, which is what this controller models as a
	// separate line: the interrupt the CPU reads back with SENSE INTERRUPT STATUS.
	s.INTRQ = u.intrPending
	return s
}

// upd765CommandName names the commands a user is likely to see. Anything else is
// reported as its byte: a wrong name would be worse than a number.
func upd765CommandName(cmd byte) string {
	switch cmd {
	case 0x04:
		return "SENSE DRIVE STATUS"
	case 0x05:
		return "WRITE DATA"
	case 0x06:
		return "READ DATA"
	case 0x07:
		return "RECALIBRATE"
	case 0x08:
		return "SENSE INTERRUPT STATUS"
	case 0x09:
		return "WRITE DELETED DATA"
	case 0x0C:
		return "READ DELETED DATA"
	case 0x0D:
		return "FORMAT TRACK"
	case 0x0F:
		return "SEEK"
	case 0x00:
		return "none"
	}
	return fmt.Sprintf("command 0x%02X", cmd)
}
