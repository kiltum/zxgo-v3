package media

import "strings"

// TapeState is what a front end shows about the tape: what is loaded, how far it has
// run, and where the head is (UI_DESIGN.md section 6.3).
//
// It is plain data with no pointer into the playback, so a window can hold it across
// a frame without touching the tape's own state.
type TapeState struct {
	// FileName is the path the tape was loaded from, and Format is "TAP" or "TZX".
	FileName string
	Format   string

	Playing bool
	Ended   bool
	Done    bool

	// Pos is the pulse the head is at, out of Pulses.
	Pos    int
	Pulses int
	// Percent is how far along the tape is, as a percentage: 0 to 100, which is what
	// Playback.PercentComplete reports and what the field's name says. It is carried
	// rather than computed by the caller so that a window cannot divide by a total
	// that has changed underneath it - and so that the units are decided in one place
	// rather than at each display. (They were decided in two places once, and the tape
	// window read 10000%.)
	Percent float64

	// Blocks is the decoded block list, and Current is the index of the block the
	// head is in (-1 when there is none yet).
	Blocks  []TapeBlock
	Current int
}

// TapeBlock is one block of the tape's decoded list.
//
// A block says what it is in one character and, when it is a header, whose it is:
// that is enough to read a load by, and the alternative is a window full of the
// header's other fields, which the ROM reads and the user does not.
type TapeBlock struct {
	Index int
	// Symbol is one character: P, N, C or B for a header by the type byte in it (a
	// program, a number array, a character array, bytes), "d" for the data that
	// follows a header - a data block carries no type of its own - and "?" for a
	// header whose type byte is not one of the four the standard defines.
	Symbol string
	// Name is the name a header carries: the file name for a program or bytes block,
	// and the variable name for an array, which is the same ten-byte field either
	// way. Data blocks have none.
	Name string
	// Bytes is the block's length in bytes, and StartsAt the pulse it begins at.
	Bytes    int
	StartsAt int
	// Current marks the block the head is in.
	Current bool
}

// StateSnapshot reports the tape's state as plain data.
func (pb *Playback) StateSnapshot() TapeState {
	state := TapeState{Current: -1}
	if pb == nil || pb.Tape == nil {
		return state
	}

	state.FileName = pb.Tape.FileName
	state.Format = pb.Tape.Format
	state.Playing = pb.IsPlaying()
	state.Ended = pb.Ended()
	state.Done = pb.IsDone()
	state.Pos = pb.Pos
	state.Pulses = len(pb.Tape.Pulses)
	if state.Pulses > 0 {
		state.Percent = pb.PercentComplete()
	}

	for i, block := range pb.Tape.Blocks {
		symbol, name := blockIdentity(block)
		state.Blocks = append(state.Blocks, TapeBlock{
			Index:    i,
			Symbol:   symbol,
			Name:     name,
			Bytes:    len(block.Data),
			StartsAt: block.BlockPulse,
		})
	}

	// The block the head is in is the last one that starts at or before the pulse it
	// is on. Walking backwards finds it in one pass over the list, and blocks are in
	// stream order, so no comparison between blocks is needed.
	for i := len(state.Blocks) - 1; i >= 0; i-- {
		if state.Blocks[i].StartsAt <= pb.Pos {
			state.Blocks[i].Current = true
			state.Current = i
			break
		}
	}
	return state
}

// blockIdentity reads what a block is out of its own bytes: the type symbol and, for
// a header, the name it carries.
//
// A ZX header is flag(1) + type(1) + name(10) + length(2) + parameter1(2) +
// parameter2(2), and a nineteenth byte: the checksum for a program or a block of
// bytes, the first character of a variable name for an array. So the type is at
// offset 1 and the name at 2, which is what the ROM's loader reads and what Fuse
// reads (ref/fuse-1.9.0/tape.c, which requires exactly 19 bytes before it will call
// a block a header at all).
//
// Reading either from the wrong offset does not fail loudly - it produces a name
// with the type byte stuck on the front of it, or a type taken from a parameter's
// high byte - so this is written against the reference rather than by eye.
func blockIdentity(b Block) (symbol, name string) {
	if len(b.Data) == 0 {
		return "?", ""
	}
	if b.Data[0] != 0x00 {
		// A data block: the payload of the header before it.
		return "d", ""
	}

	symbol = "?"
	switch b.BlockType {
	case 0:
		symbol = "P" // program
	case 1:
		symbol = "N" // number array
	case 2:
		symbol = "C" // character array
	case 3:
		symbol = "B" // bytes
	}

	// Ten bytes of name, space-padded. NULs are trimmed too: some tape tools pad
	// with them, and a name with a NUL in it is invisible in a window rather than
	// visibly wrong.
	if len(b.Data) >= 12 {
		name = strings.TrimRight(string(b.Data[2:12]), " \x00")
	}
	return symbol, name
}
