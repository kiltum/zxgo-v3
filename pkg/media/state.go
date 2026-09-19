package media

import (
	"errors"
	"fmt"
	"hash/crc32"

	"github.com/kiltum/zxgo-v3/pkg/state"
)

// Native state format for the storage devices (STATE_DESIGN.md).
//
// Two things make this more than a register dump:
//
//   - every deadline in a controller is an absolute T-state (a seek's completion,
//     the track under the head, the overrun timeout, the index pulse's phase), and
//     the container restores the machine's clock to the same value, so they are
//     stored as they stand rather than as deltas.
//   - a disk is embedded whole, because a disk is mutable: the user may have
//     written to it and the file on disk may since have been replaced. It is
//     written as the structure rather than re-encoded into TRD or DSK, because
//     those formats cannot carry what a copy-protection loader depends on -
//     per-sector status, deleted marks, and the weak-bit mask.

// errDiskShape is a disk whose geometry or sector layout cannot be true.
var errDiskShape = errors.New("disk: geometry is not plausible")

// Geometry bounds. A disk image is untrusted input like anything else in a
// state file, and these are wider than any real format: 256 tracks per side,
// 256 sectors per track, and 16K sectors.
const (
	maxDiskTracks   = 256
	maxDiskSectors  = 256
	maxSectorBytes  = 16384
	maxDiskGeometry = 1024
)

// SaveDisk writes a complete disk, geometry and every sector.
func SaveDisk(e *state.Encoder, d *Disk) {
	e.String(string(d.Type))
	e.I64(int64(d.Sides))
	e.I64(int64(d.TracksPerSide))
	e.I64(int64(d.SectorsPerTrack))
	e.I64(int64(d.SectorSize))
	e.Bool(d.ReadOnly)

	e.Count(len(d.Tracks))
	for ti := range d.Tracks {
		sectors := d.Tracks[ti].Sectors
		e.Count(len(sectors))
		for si := range sectors {
			s := &sectors[si]
			e.U8(s.Track)
			e.U8(s.Side)
			e.U8(s.SectorID)
			e.U8(s.N)
			e.Bytes(s.Data)
			e.Bool(s.Deleted)
			e.U8(s.ST1)
			e.U8(s.ST2)
			e.U16(s.IDCRC)
			e.Bytes(weakMask(s.Weak))
		}
	}
}

// LoadDisk reads a disk written by SaveDisk.
//
// Every length is bounded before it is used: the geometry against the maxima
// above, a sector's data against the sector size the geometry declares, and the
// weak mask against the data it describes.
func LoadDisk(d *state.Decoder) (*Disk, error) {
	diskType := DiskType(d.String())
	sides := int(d.I64())
	tracksPerSide := int(d.I64())
	sectorsPerTrack := int(d.I64())
	sectorSize := int(d.I64())
	readOnly := d.Bool()
	tracks := d.Count(4) // one track is at least its own sector count
	if err := d.Err(); err != nil {
		return nil, err
	}
	if tracks > maxDiskTracks || sides < 0 || sides > 4 ||
		tracksPerSide < 0 || tracksPerSide > maxDiskTracks ||
		sectorsPerTrack < 0 || sectorsPerTrack > maxDiskSectors ||
		sectorSize < 0 || sectorSize > maxSectorBytes {
		return nil, errDiskShape
	}

	out := &Disk{
		Type:            diskType,
		Tracks:          make([]DiskTrack, 0, tracks),
		Sides:           sides,
		TracksPerSide:   tracksPerSide,
		SectorsPerTrack: sectorsPerTrack,
		SectorSize:      sectorSize,
		ReadOnly:        readOnly,
	}
	for ti := 0; ti < tracks; ti++ {
		n := d.Count(4)
		if err := d.Err(); err != nil {
			return nil, err
		}
		if n > maxDiskSectors {
			return nil, errDiskShape
		}
		track := DiskTrack{Sectors: make([]DiskSector, 0, n)}
		for si := 0; si < n; si++ {
			var s DiskSector
			s.Track = d.U8()
			s.Side = d.U8()
			s.SectorID = d.U8()
			s.N = d.U8()
			data := d.Bytes()
			s.Deleted = d.Bool()
			s.ST1 = d.U8()
			s.ST2 = d.U8()
			s.IDCRC = d.U16()
			mask := d.Bytes()
			if err := d.Err(); err != nil {
				return nil, err
			}
			if len(data) > maxSectorBytes {
				return nil, fmt.Errorf("%w: sector of %d bytes", errDiskShape, len(data))
			}
			s.Data = data
			if len(mask) > 0 {
				weak, err := expandWeak(mask, len(data))
				if err != nil {
					return nil, err
				}
				s.Weak = weak
			}
			track.Sectors = append(track.Sectors, s)
		}
		out.Tracks = append(out.Tracks, track)
	}
	return out, nil
}

// weakMask packs a weak-bit mask into bytes, one bit per position. The mask is
// usually nil, in which case this is empty.
func weakMask(weak []bool) []byte {
	if len(weak) == 0 {
		return nil
	}
	out := make([]byte, (len(weak)+7)/8)
	for i, w := range weak {
		if w {
			out[i/8] |= 1 << uint(i%8)
		}
	}
	return out
}

// expandWeak unpacks a mask for a sector of dataLen bytes. A mask that does not
// cover the sector exactly is refused rather than padded: it means the file and
// its data disagree about how long the sector is.
func expandWeak(mask []byte, dataLen int) ([]bool, error) {
	if len(mask) != (dataLen+7)/8 {
		return nil, fmt.Errorf("%w: weak mask of %d bytes for %d bytes of data", errDiskShape, len(mask), dataLen)
	}
	out := make([]bool, dataLen)
	for i := range out {
		out[i] = mask[i/8]&(1<<uint(i%8)) != 0
	}
	return out, nil
}

// --- Beta Disk (WD1793) ---------------------------------------------------

// SaveState encodes the controller's registers, its drive selection and every
// deadline of a command still running.
//
// The disk in each drive is not here: it is mutable and goes in the media
// chunk, which the coordinator assembles from the slots the controllers expose.
func (c *BetaDiskController) SaveState(e *state.Encoder) error {
	e.U8(uint8(c.status))
	e.U8(c.track)
	e.U8(c.sector)
	e.U8(c.data)
	e.U8(uint8(c.command))
	e.Bool(c.interruptPending)
	e.Bool(c.dataRequest)
	e.Bool(c.statusTypeI)

	e.I64(c.tick)
	e.I64(c.cpuHz)
	e.U8(c.driveSelect)
	e.U8(c.sideSelect)
	e.Bool(c.systemReset)
	e.I64(int64(c.activeDrive))

	e.U8(c.state)
	e.I64(int64(c.byteCounter))
	e.Bytes(c.buffer)

	e.Bool(c.multiSector)
	e.Bool(c.writeTrackActive)
	e.I64(c.writeTrackStartTick)
	e.I64(c.transferStartTick)
	e.Bool(c.typeIPending)
	e.I64(c.typeICompleteTick)
	e.I64(c.dataReadyTick)
	e.Bool(c.noTiming)
	return nil
}

// LoadState applies a payload written by SaveState. The mounted disks are left
// alone: they belong to the media chunk.
func (c *BetaDiskController) LoadState(d *state.Decoder) error {
	var (
		status, track, sector, data, command   uint8
		interruptPending, dataRequest, statusI bool
		tick, cpuHz                            int64
		driveSelect, sideSelect                uint8
		systemReset                            bool
		activeDrive                            int
		phase                                  uint8
		byteCounter                            int
		buffer                                 []byte
		multiSector, writeTrackActive          bool
		writeTrackStartTick, transferStartTick int64
		typeIPending                           bool
		typeICompleteTick, dataReadyTick       int64
		noTiming                               bool
	)

	status = d.U8()
	track = d.U8()
	sector = d.U8()
	data = d.U8()
	command = d.U8()
	interruptPending = d.Bool()
	dataRequest = d.Bool()
	statusI = d.Bool()

	tick = d.I64()
	cpuHz = d.I64()
	driveSelect = d.U8()
	sideSelect = d.U8()
	systemReset = d.Bool()
	activeDrive = int(d.I64())

	phase = d.U8()
	byteCounter = int(d.I64())
	buffer = d.Bytes()

	multiSector = d.Bool()
	writeTrackActive = d.Bool()
	writeTrackStartTick = d.I64()
	transferStartTick = d.I64()
	typeIPending = d.Bool()
	typeICompleteTick = d.I64()
	dataReadyTick = d.I64()
	noTiming = d.Bool()

	if err := d.Err(); err != nil {
		return err
	}
	if byteCounter < 0 || byteCounter > len(buffer) {
		return fmt.Errorf("%w: byte counter %d for a %d-byte buffer", errDiskShape, byteCounter, len(buffer))
	}
	if activeDrive < 0 || activeDrive >= len(c.drives) {
		return fmt.Errorf("%w: drive %d", errDiskShape, activeDrive)
	}

	c.status = WD1793Status(status)
	c.track = track
	c.sector = sector
	c.data = data
	c.command = WD1793Command(command)
	c.interruptPending = interruptPending
	c.dataRequest = dataRequest
	c.statusTypeI = statusI

	c.tick = tick
	c.cpuHz = cpuHz
	c.driveSelect = driveSelect
	c.sideSelect = sideSelect
	c.systemReset = systemReset
	c.activeDrive = activeDrive

	c.state = phase
	c.byteCounter = byteCounter
	// The buffer is aliased by the decoder's payload, so it is copied: the
	// controller keeps it for as long as the transfer runs.
	c.buffer = append([]byte(nil), buffer...)

	c.multiSector = multiSector
	c.writeTrackActive = writeTrackActive
	c.writeTrackStartTick = writeTrackStartTick
	c.transferStartTick = transferStartTick
	c.typeIPending = typeIPending
	c.typeICompleteTick = typeICompleteTick
	c.dataReadyTick = dataReadyTick
	c.noTiming = noTiming
	return nil
}

// --- uPD765 ---------------------------------------------------------------

// SaveState encodes the +2A/+3 controller's command phase, result queue,
// transfer buffer and deadlines.
func (u *UPD765) SaveState(e *state.Encoder) error {
	e.U8(u.driveSel)
	e.U8(u.curTrack)
	e.U8(u.curSide)
	e.U8(u.curSector)
	e.U8(u.curPCN)
	e.U8(u.curHead)
	e.U8(u.curN)

	e.U8(u.cmd)
	e.Bytes(u.params)
	e.I64(int64(u.paramCount))

	e.Bytes(u.result)
	e.I64(int64(u.resultIdx))

	e.Bytes(u.dataBuf)
	e.I64(int64(u.dataIdx))
	e.Bool(u.writing)
	e.Bool(u.delData)

	e.Count(len(u.writePlan))
	for _, w := range u.writePlan {
		e.U8(w.sectorID)
		e.I64(int64(w.size))
	}

	e.Bool(u.writeProtect)
	e.Bool(u.formatActive)
	e.U8(u.formatFill)

	e.I64(int64(u.phase))
	e.U8(u.st0)
	e.U8(u.pcn)
	e.Bool(u.intrPending)
	e.U8(u.fddBusy)
	e.U8(u.lastMSR)

	e.I64(int64(u.speedlock))
	e.I64(int64(u.lastSectorKey))

	e.I64(u.curTick)
	e.I64(u.dataReadyTick)
	e.I64(u.dataDeadline)
	e.I64(u.cpuHz)
	e.Bool(u.seekPending)
	e.I64(u.seekCompleteTick)
	e.Bool(u.noTiming)
	return nil
}

// LoadState applies a payload written by SaveState.
func (u *UPD765) LoadState(d *state.Decoder) error {
	var (
		driveSel, curTrack, curSide, curSector uint8
		curPCN, curHead, curN                  uint8
		cmd                                    uint8
		params                                 []byte
		paramCount                             int
		result                                 []byte
		resultIdx                              int
		dataBuf                                []byte
		dataIdx                                int
		writing, delData                       bool
		writePlan                              []writeTarget
		writeProtect, formatActive             bool
		formatFill                             uint8
		phase                                  int
		st0, pcn, fddBusy, lastMSR             uint8
		intrPending                            bool
		speedlock                              int
		lastSectorKey                          uint
		curTick, dataReadyTick, dataDeadline   int64
		cpuHz                                  int64
		seekPending                            bool
		seekCompleteTick                       int64
		noTiming                               bool
	)

	driveSel = d.U8()
	curTrack = d.U8()
	curSide = d.U8()
	curSector = d.U8()
	curPCN = d.U8()
	curHead = d.U8()
	curN = d.U8()

	cmd = d.U8()
	params = d.Bytes()
	paramCount = int(d.I64())

	result = d.Bytes()
	resultIdx = int(d.I64())

	dataBuf = d.Bytes()
	dataIdx = int(d.I64())
	writing = d.Bool()
	delData = d.Bool()

	plan := d.Count(9) // one entry is a sector ID and a size
	if err := d.Err(); err != nil {
		return err
	}
	writePlan = make([]writeTarget, 0, plan)
	for i := 0; i < plan; i++ {
		w := writeTarget{
			sectorID: d.U8(),
			size:     int(d.I64()),
		}
		if err := d.Err(); err != nil {
			return err
		}
		writePlan = append(writePlan, w)
	}

	writeProtect = d.Bool()
	formatActive = d.Bool()
	formatFill = d.U8()

	phase = int(d.I64())
	st0 = d.U8()
	pcn = d.U8()
	intrPending = d.Bool()
	fddBusy = d.U8()
	lastMSR = d.U8()

	speedlock = int(d.I64())
	lastSectorKey = uint(d.I64())

	curTick = d.I64()
	dataReadyTick = d.I64()
	dataDeadline = d.I64()
	cpuHz = d.I64()
	seekPending = d.Bool()
	seekCompleteTick = d.I64()
	noTiming = d.Bool()

	if err := d.Err(); err != nil {
		return err
	}
	if paramCount < 0 || paramCount > len(params) {
		return fmt.Errorf("%w: param count %d for %d bytes", errDiskShape, paramCount, len(params))
	}
	if resultIdx < 0 || resultIdx > len(result) {
		return fmt.Errorf("%w: result index %d for %d bytes", errDiskShape, resultIdx, len(result))
	}
	if dataIdx < 0 || dataIdx > len(dataBuf) {
		return fmt.Errorf("%w: data index %d for %d bytes", errDiskShape, dataIdx, len(dataBuf))
	}

	u.driveSel = driveSel
	u.curTrack = curTrack
	u.curSide = curSide
	u.curSector = curSector
	u.curPCN = curPCN
	u.curHead = curHead
	u.curN = curN

	u.cmd = cmd
	u.params = append([]byte(nil), params...)
	u.paramCount = paramCount

	u.result = append([]byte(nil), result...)
	u.resultIdx = resultIdx

	u.dataBuf = append([]byte(nil), dataBuf...)
	u.dataIdx = dataIdx
	u.writing = writing
	u.delData = delData

	u.writePlan = writePlan

	u.writeProtect = writeProtect
	u.formatActive = formatActive
	u.formatFill = formatFill

	u.phase = phase
	u.st0 = st0
	u.pcn = pcn
	u.intrPending = intrPending
	u.fddBusy = fddBusy
	u.lastMSR = lastMSR

	u.speedlock = speedlock
	u.lastSectorKey = lastSectorKey

	u.curTick = curTick
	u.dataReadyTick = dataReadyTick
	u.dataDeadline = dataDeadline
	u.cpuHz = cpuHz
	u.seekPending = seekPending
	u.seekCompleteTick = seekCompleteTick
	u.noTiming = noTiming
	return nil
}

// --- Tape playback --------------------------------------------------------

// tapeState is a mounted tape as the container refers to it: where it came from,
// what it is, and where playback had reached.
type tapeState struct {
	name      string
	pulses    uint32
	hash      uint32
	pos       int
	tapeTick  int64
	nextTick  int64
	lastCPU   int64
	active    bool
	endNotify bool
}

// SavePlayback writes a tape reference: the path, a hash of the pulse stream and
// the playback position.
//
// The tape itself is not embedded - writing tapes is out of scope, so the pulse
// stream is a pure function of the file and a reference is enough. The hash is
// taken over the pulse stream rather than the file so it is available whatever
// the file has since done, and so a tape loaded from inside a .zip is identified
// by its contents rather than by the archive entry's name.
func SavePlayback(e *state.Encoder, pb *Playback) {
	e.String(pb.Tape.FileName)
	e.U32(pulseHash(pb.Tape))
	// The pulse count is a reference to the tape, not elements in this chunk:
	// the pulses themselves live in the file, so this is a plain number and not
	// a Count, which would promise the payload holds that many of something.
	e.U32(uint32(len(pb.Tape.Pulses)))
	e.I64(int64(pb.Pos))
	e.I64(pb.TapeTick)
	e.I64(pb.NextTick)
	e.I64(pb.LastCPUTick)
	e.Bool(pb.Active)
	e.Bool(pb.EndNotified)
}

// LoadPlayback reads a tape reference. The tape itself is loaded by the caller,
// which is the only part that can touch the filesystem.
func LoadPlayback(d *state.Decoder) (name string, hash uint32, pulses int, apply func(*Playback) error, err error) {
	st := tapeState{}
	st.name = d.String()
	st.hash = d.U32()
	st.pulses = d.U32()
	st.pos = int(d.I64())
	st.tapeTick = d.I64()
	st.nextTick = d.I64()
	st.lastCPU = d.I64()
	st.active = d.Bool()
	st.endNotify = d.Bool()
	if derr := d.Err(); derr != nil {
		return "", 0, 0, nil, derr
	}
	apply = func(pb *Playback) error {
		if pb.Tape == nil {
			return errors.New("tape: nothing mounted")
		}
		pb.Pos = st.pos
		pb.TapeTick = st.tapeTick
		pb.NextTick = st.nextTick
		pb.LastCPUTick = st.lastCPU
		pb.Active = st.active
		pb.EndNotified = st.endNotify
		return nil
	}
	return st.name, st.hash, int(st.pulses), apply, nil
}

// pulseHash identifies a tape's pulse stream. It is a CRC of every pulse's level
// and duration, which is exactly the data that decides how the tape plays - two
// files with the same hash load the same, whatever their names.
func pulseHash(t *Tape) uint32 {
	if t == nil {
		return 0
	}
	var buf [9]byte
	h := crc32.NewIEEE()
	for _, p := range t.Pulses {
		buf[0] = 0
		if p.Level {
			buf[0] = 1
		}
		d := uint64(p.Duration)
		for i := 0; i < 8; i++ {
			buf[1+i] = byte(d >> (8 * uint(i)))
		}
		h.Write(buf[:])
	}
	return h.Sum32()
}

// TapeIdentity exposes the pulse-stream hash and length, so the coordinator can
// check a tape it has just loaded against the one a state file refers to.
func TapeIdentity(t *Tape) (uint32, int) {
	if t == nil {
		return 0, 0
	}
	return pulseHash(t), len(t.Pulses)
}

var _ state.Component = (*BetaDiskController)(nil)
var _ state.Component = (*UPD765)(nil)
