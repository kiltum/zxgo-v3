package media

import "testing"

// A controller with no disk reports a controller, and no disk: a window asks every
// frame it is drawn, so "nothing in the drive" is an answer rather than a case the
// caller has to check for first.
func TestFDCStateWithoutADisk(t *testing.T) {
	beta := NewBetaDiskController()
	beta.SetClockHz(3500000)
	beta.SetTick(0)

	state := beta.StateSnapshot()
	if state.Controller != "WD1793" {
		t.Errorf("controller = %q", state.Controller)
	}
	if state.Ready {
		t.Error("a controller with no disk reports one")
	}
	if state.IndexPhase != -1 {
		t.Errorf("index phase = %v with no disk, want -1", state.IndexPhase)
	}
	if state.NotReady {
		// NOT READY is the drive's answer to a command, not a fact about the drive
		// being empty: with no command issued the bit is clear. Worth stating, because
		// the two are easy to conflate in a window.
		t.Log("NOT READY is set with no command issued")
	}
}

// A mounted disk is reported as being there, with its own details, and the index phase
// comes from the same tick and period the index pulse does - which is what TR-DOS's
// drive detection polls, so the read-out and the machine cannot disagree.
func TestFDCStateWithADisk(t *testing.T) {
	disk := &Disk{
		Type:            DiskTypeTRD,
		TracksPerSide:   80,
		Sides:           2,
		SectorsPerTrack: 16,
		SectorSize:      256,
	}
	beta := NewBetaDiskController()
	beta.SetClockHz(3500000)
	beta.MountDisk(disk)

	state := beta.StateSnapshot()
	if !state.Ready {
		t.Fatal("a mounted disk is not reported")
	}
	// The registers start where a fresh chip points them: drive 0, track 0, and the
	// sector register at 1 rather than 0.
	if state.Drive != 0 || state.Track != 0 || state.Sector != 1 {
		t.Errorf("drive/track/sector = %d/%d/%d at rest, want 0/0/1",
			state.Drive, state.Track, state.Sector)
	}

	// The phase sweeps a revolution and wraps, and the index pulse is the first part of
	// it: the pulse is what TR-DOS's drive detection polls, so the two have to be the
	// same rotation counted the same way.
	period := int64(3500000) * indexRevolutionMS / 1000

	beta.SetTick(0)
	if got := beta.StateSnapshot().IndexPhase; got != 0 {
		t.Errorf("phase at tick 0 = %v, want 0", got)
	}
	if !beta.indexPulse() {
		t.Error("the index pulse is not set at the start of a revolution")
	}

	beta.SetTick(period / 2)
	if got := beta.StateSnapshot().IndexPhase; got < 0.49 || got > 0.51 {
		t.Errorf("phase halfway = %v, want about 0.5", got)
	}
	if beta.indexPulse() {
		t.Error("the index pulse is still set halfway through a revolution")
	}

	// A revolution later it is back to the start: the phase is a position, not a count.
	beta.SetTick(period)
	if got := beta.StateSnapshot().IndexPhase; got != 0 {
		t.Errorf("phase after a full revolution = %v, want 0", got)
	}
	if !beta.indexPulse() {
		t.Error("the index pulse is not set a revolution later")
	}
}

// The two status layouts are read as their own: a TRACK 00 bit left over from a seek
// must not come out as LOST DATA after a sector read, which is the bug the controller's
// own statusByte documents.
func TestFDCStateReadsBothStatusLayouts(t *testing.T) {
	disk := &Disk{Type: DiskTypeTRD, SectorsPerTrack: 16}
	beta := NewBetaDiskController()
	beta.SetClockHz(3500000)
	beta.MountDisk(disk)

	// Type I with the head at track 0: TRACK 00, and no LOST DATA.
	beta.statusTypeI = true
	beta.track = 0
	beta.SetTick(0)

	state := beta.StateSnapshot()
	if !state.TrackZero {
		t.Error("the head is at track 0 and the state does not say so")
	}
	if state.LostData {
		t.Error("a Type I status reported LOST DATA, which is the other layout's bit")
	}
	if state.DRQ {
		t.Error("a Type I status reported a data request, which is the other layout's bit")
	}

	// Type II/III: DRQ and LOST DATA are meaningful, TRACK 00 is not.
	beta.statusTypeI = false
	beta.status = BetaDiskDataRequest
	state = beta.StateSnapshot()
	if !state.DRQ {
		t.Error("a Type II status with DATA REQUEST set did not report DRQ")
	}
	if state.TrackZero {
		t.Error("a Type II status reported TRACK 00, which is the other layout's bit")
	}
}

// The uPD765 reports its own register's lines, named the same way for a user.
func TestUPD765StateSnapshot(t *testing.T) {
	u := NewUPD765()
	u.SetClockHz(3500000)

	state := u.StateSnapshot()
	if state.Controller != "uPD765" {
		t.Errorf("controller = %q", state.Controller)
	}
	if state.Ready {
		t.Error("a controller with no disk reports one")
	}
	if !state.NotReady {
		t.Error("an empty drive is not reported as not ready")
	}
	if state.Command != "none" {
		t.Errorf("command = %q before any command, want none", state.Command)
	}

	// A disk, and a command with a name.
	disk := &Disk{Type: DiskTypeDSK}
	u.MountDisk(disk)
	u.cmd = 0x06 // READ DATA
	state = u.StateSnapshot()
	if !state.Ready || state.NotReady {
		t.Error("a mounted disk is not reported as ready")
	}
	if state.Command != "READ DATA" {
		t.Errorf("command = %q, want READ DATA", state.Command)
	}

	// A command with no name is reported as its byte rather than guessed at.
	u.cmd = 0x33
	if got := u.StateSnapshot().Command; got != "command 0x33" {
		t.Errorf("command = %q, want the byte", got)
	}
}
