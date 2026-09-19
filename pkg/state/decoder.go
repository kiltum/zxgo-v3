package state

import (
	"encoding/binary"
	"math"
)

// Decoder reads one component's payload.
//
// It reports failure by latching the first error and returning zero values from
// then on, so a component can write a straight line of reads and check once at
// the end:
//
//	func (c *Chip) LoadState(d *state.Decoder) error {
//	    du := d.U32()
//	    x := d.U16()
//	    c.someField = d.Bytes()
//	    if err := d.Err(); err != nil {
//	        return err // c is untouched: nothing was assigned
//	    }
//	    c.dup = du
//	    c.x = x
//	    return nil
//	}
//
// Assigning only after Err is checked is the parse-fully-then-assign rule, and
// it is what bounds a failed load: a component whose payload will not decode
// stays exactly as it was.
//
// # Bounds
//
// Every read is checked against what is left, and a length-prefixed value is
// refused rather than allocated when the length claims more bytes than the
// payload can hold. A file cannot make this decoder allocate.
//
// Lengths are also *not* trusted to be present: More reports whether anything
// remains, which is how a component reads a payload written by a build with
// fewer fields - check More, then read, instead of assuming a fixed layout.
type Decoder struct {
	buf     []byte
	pos     int
	err     error
	version uint16
}

// NewDecoder reads payload, which a component wrote when its chunk was at
// version. version is what a component branches on to migrate an older payload.
func NewDecoder(payload []byte, version uint16) *Decoder {
	return &Decoder{buf: payload, version: version}
}

// Version returns the chunk version the payload was written at. A component
// that added a field appends it and bumps its version, so a reader that sees an
// older number knows the field is simply not there.
func (d *Decoder) Version() uint16 { return d.version }

// Remaining returns how many bytes are left unread.
func (d *Decoder) Remaining() int { return len(d.buf) - d.pos }

// More reports whether any byte is left. It is the version-tolerant read: a
// field this build has and the writer did not is absent, not an error.
func (d *Decoder) More() bool { return d.Remaining() > 0 }

// Err returns the first decoding failure, or nil.
func (d *Decoder) Err() error { return d.err }

// Failed reports whether a failure has been latched.
func (d *Decoder) Failed() bool { return d.err != nil }

// fail latches the first error. Later failures are dropped: the first one is
// the one that explains the payload.
func (d *Decoder) fail(what string, detail string) {
	if d.err == nil {
		d.err = &DecodeError{What: what, Detail: detail}
	}
}

// take advances over n bytes, latching an error if they are not there.
func (d *Decoder) take(n int) []byte {
	if d.err != nil {
		return nil
	}
	if n < 0 || n > d.Remaining() {
		d.fail("payload ended early", "wanted bytes that the chunk does not hold")
		return nil
	}
	b := d.buf[d.pos : d.pos+n]
	d.pos += n
	return b
}

// U8 reads one byte.
func (d *Decoder) U8() uint8 {
	b := d.take(1)
	if b == nil {
		return 0
	}
	return b[0]
}

// Bool reads a byte as a boolean. Anything that is not zero is true, so a file
// written by a build that stored a flag differently still reads as set rather
// than silently clearing it.
func (d *Decoder) Bool() bool { return d.U8() != 0 }

func (d *Decoder) U16() uint16 {
	b := d.take(2)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint16(b)
}

func (d *Decoder) U32() uint32 {
	b := d.take(4)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint32(b)
}

func (d *Decoder) U64() uint64 {
	b := d.take(8)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint64(b)
}

// I16 reads a signed 16-bit value.
func (d *Decoder) I16() int16 { return int16(d.U16()) }

// I32 reads a signed 32-bit value.
func (d *Decoder) I32() int32 { return int32(d.U32()) }

// I64 reads a signed 64-bit value.
func (d *Decoder) I64() int64 { return int64(d.U64()) }

// F32 reads a float32's IEEE bits.
func (d *Decoder) F32() float32 { return math.Float32frombits(d.U32()) }

// F64 reads a float64's IEEE bits.
func (d *Decoder) F64() float64 { return math.Float64frombits(d.U64()) }

// Bytes reads a length-prefixed blob. The allocation is bounded by what the
// payload can actually hold, so a corrupt length cannot ask for gigabytes.
//
// The returned slice aliases the payload rather than copying it. A component
// that keeps it must copy, or it will be holding the reader's buffer.
func (d *Decoder) Bytes() []byte {
	n := d.U32()
	if d.err != nil {
		return nil
	}
	// Compare in 64 bits: on a 32-bit host int(n) would go negative for a
	// length above 2^31 and the check would pass.
	if uint64(n) > uint64(d.Remaining()) {
		d.fail("length overruns the chunk", "a length-prefixed value claims more bytes than remain")
		return nil
	}
	b := d.take(int(n))
	if b == nil {
		return nil
	}
	return b
}

// String reads a length-prefixed string.
func (d *Decoder) String() string { return string(d.Bytes()) }

// Count reads an element count for a component's own count-and-loop encoding
// and checks that the payload could hold that many elements of elemSize bytes.
//
// It is exported because "a count followed by a loop" is the shape most
// components use for anything without a built-in method, and the bound has to
// be applied there too: a slice header is allocated before its elements are
// read, so a count believed straight from the file is an allocation the file
// chooses.
func (d *Decoder) Count(elemSize int) int {
	n := d.U32()
	if d.err != nil {
		return 0
	}
	if elemSize <= 0 || uint64(n)*uint64(elemSize) > uint64(d.Remaining()) {
		d.fail("slice length overruns the chunk", "an element count claims more bytes than remain")
		return 0
	}
	return int(n)
}

// count is Count for the slice methods, which know their element size.
func (d *Decoder) count(elemSize int) int { return d.Count(elemSize) }

func (d *Decoder) U16s() []uint16 {
	n := d.count(2)
	if d.err != nil {
		return nil
	}
	v := make([]uint16, n)
	for i := range v {
		v[i] = d.U16()
	}
	return v
}

func (d *Decoder) U32s() []uint32 {
	n := d.count(4)
	if d.err != nil {
		return nil
	}
	v := make([]uint32, n)
	for i := range v {
		v[i] = d.U32()
	}
	return v
}

func (d *Decoder) U64s() []uint64 {
	n := d.count(8)
	if d.err != nil {
		return nil
	}
	v := make([]uint64, n)
	for i := range v {
		v[i] = d.U64()
	}
	return v
}

func (d *Decoder) I64s() []int64 {
	n := d.count(8)
	if d.err != nil {
		return nil
	}
	v := make([]int64, n)
	for i := range v {
		v[i] = d.I64()
	}
	return v
}

// Strings reads a length-prefixed slice of strings. Each element is itself
// length-prefixed, and each is bounded against what remains, so the count check
// here is only the outer layer.
func (d *Decoder) Strings() []string {
	n := d.count(4)
	if d.err != nil {
		return nil
	}
	v := make([]string, n)
	for i := range v {
		v[i] = d.String()
	}
	return v
}
