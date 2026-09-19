package state

import "fmt"

// ID identifies a chunk in the file. It is a uint32 on disk so the space is
// effectively inexhaustible, and it is declared once, here, in one block.
//
// Ids are never reused and never reordered. An old file with id 7 keeps meaning
// what id 7 meant; a retired component leaves a gap, which is finer than
// handing its number to something else. Adding a component appends its id.
//
// Which ids a file actually carries depends on the machine it was saved from:
// a model with a single AY writes IDAY and never IDTurboSound, and a machine
// with no GS writes neither IDGeneralSound nor IDCPUGS. The coordinator in
// internal/emulator builds exactly the sections the live machine has, so a
// missing chunk is the normal case rather than an error, and only the sections
// it marks required can refuse a load.
type ID uint32

const (
	// IDEmulator is the emulator's own clock: totalTicks, frameCount, the
	// interrupt tick and the fast-tape flag. Everything else that stores an
	// absolute tick is only meaningful against it, which is why it loads first.
	IDEmulator ID = 1

	// IDCPU is the Spectrum's Z80: all registers, the EI-delay latch, MEMPTR,
	// and the two behaviour switches (CPU type, MEMPTR on the repeat path).
	IDCPU ID = 2

	// IDCPUGS is the General Sound's own Z80. Same routines as IDCPU, a
	// different machine.
	IDCPUGS ID = 3

	// IDRAM is every RAM bank the mapper holds, in bank order. ROM is not part
	// of it: the model defines the ROMs and they are reloaded, never restored.
	IDRAM ID = 4

	// IDPaging is the banking state - 0x7FFD, 0x1FFD and each family's own
	// registers - encoded and keyed by scheme name, plus the mapper's record of
	// the last byte written to 0x7FFD, which the lock bit makes unrecoverable
	// from the effective state alone.
	IDPaging ID = 5

	// IDULA is video state: border (including a deferred change), the raster
	// position, the flash counter, the snow ring and the interrupt deadline.
	IDULA ID = 6

	// IDBeeper is the 1-bit speaker: EAR/MIC levels and the event cursor.
	IDBeeper ID = 7

	// IDMixer is the audio grid position and the DC-block filter history.
	IDMixer ID = 8

	// IDAY is a single AY-3-8912, including its generator counters and its
	// queue of register writes that have not reached their tick yet.
	IDAY ID = 9

	// IDTurboSound is a TurboSound pair: two AY-3-8912s and which one the
	// select port last chose. Mutually exclusive with IDAY.
	IDTurboSound ID = 10

	// IDYM2203 is one YM2203 (OPN): the FM operator state, the SSG subset, the
	// timers and the prescaler. Mutually exclusive with IDAY.
	IDYM2203 ID = 11

	// IDCovox is a Covox 8-bit DAC and its level.
	IDCovox ID = 12

	// IDSounDrive is a SounDrive's four DACs and their levels. Nothing
	// registers one today - it cannot share a machine with a Beta Disk - so no
	// file carries this id yet.
	IDSounDrive ID = 13

	// IDGeneralSound is the GS card: its Z80's memory and paging, the host
	// handshake registers, the DAC volumes and the unconsumed DAC events.
	IDGeneralSound ID = 14

	// IDBetaDisk is the WD1793 controller: registers, drive selection, the
	// in-flight buffer and the deadlines of a command still running.
	IDBetaDisk ID = 15

	// IDUPD765 is the +2A/+3 floppy controller: phase, parameters, the result
	// queue, the transfer buffer and the deadlines of a command still running.
	IDUPD765 ID = 16

	// IDDiskImages is the mounted disk images themselves, embedded as bytes
	// because a disk is mutable and the file on disk may have moved on.
	IDDiskImages ID = 17

	// IDTape is the mounted tape by path and content hash plus the pulse
	// position - a reference, not the image, because tape writing is out of
	// scope and the pulse stream is a pure function of the file.
	IDTape ID = 18

	// IDTurboSoundFM is a TurboSound FM board: two YM2203s and which one the
	// select port last chose. It is a separate id from IDYM2203 because the
	// payload is a different shape - a pair wrapping two chips rather than one
	// chip - and a file has to say which it holds.
	IDTurboSoundFM ID = 19
)

// idNames is the id-to-name table, used in messages. An id absent from it is
// one this build does not know, which is exactly what a skip reports.
var idNames = map[ID]string{
	IDEmulator:     "emulator",
	IDCPU:          "cpu",
	IDCPUGS:        "cpu-gs",
	IDRAM:          "ram",
	IDPaging:       "paging",
	IDULA:          "ula",
	IDBeeper:       "beeper",
	IDMixer:        "mixer",
	IDAY:           "ay",
	IDTurboSound:   "turbosound",
	IDYM2203:       "ym2203",
	IDTurboSoundFM: "turbosound-fm",
	IDCovox:        "covox",
	IDSounDrive:    "soundrive",
	IDGeneralSound: "general-sound",
	IDBetaDisk:     "beta-disk",
	IDUPD765:       "upd765",
	IDDiskImages:   "disk-images",
	IDTape:         "tape",
}

// String names the chunk for a message: the component's name when this build
// knows the id, and the raw number when it does not.
func (id ID) String() string {
	if n, ok := idNames[id]; ok {
		return n
	}
	return fmt.Sprintf("chunk-%d", uint32(id))
}

// Known reports whether this build recognises the id.
func Known(id ID) bool {
	_, ok := idNames[id]
	return ok
}
