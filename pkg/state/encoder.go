package state

import (
	"bytes"
	"encoding/binary"
	"math"
)

// Encoder writes one component's payload.
//
// Every method is infallible. The only thing that can go wrong while encoding
// is a payload too large for the envelope's 32-bit length, and the container
// checks that once, when it writes the envelope, rather than making every
// component thread an error through a straight line of appends.
//
// The format is explicit and hand-written - no gob, no reflect - so a refactor
// that renames or reorders a struct field cannot silently change the bytes on
// disk, and a reader can see exactly what a payload holds.
//
// Numbers are little-endian and fixed width. Floats go through their IEEE bits
// so the encoding cannot depend on the host's byte order or on a text
// conversion that rounds.
type Encoder struct {
	buf bytes.Buffer
}

// NewEncoder creates an empty encoder.
func NewEncoder() *Encoder { return &Encoder{} }

// Payload returns the encoded bytes. The result is the encoder's own buffer and
// must not be modified by the caller.
func (e *Encoder) Payload() []byte { return e.buf.Bytes() }

// Len returns how many bytes have been encoded so far.
func (e *Encoder) Len() int { return e.buf.Len() }

// Reset empties the encoder so its buffer can be reused.
func (e *Encoder) Reset() { e.buf.Reset() }

// U8 writes one byte.
func (e *Encoder) U8(v uint8) { e.buf.WriteByte(v) }

// Bool writes a byte: 0 or 1. One byte rather than a bit, because a payload is
// read by a human as often as by a program and the saving is not worth the
// reader's confusion.
func (e *Encoder) Bool(v bool) {
	if v {
		e.buf.WriteByte(1)
		return
	}
	e.buf.WriteByte(0)
}

func (e *Encoder) U16(v uint16) {
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], v)
	e.buf.Write(b[:])
}

func (e *Encoder) U32(v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	e.buf.Write(b[:])
}

func (e *Encoder) U64(v uint64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], v)
	e.buf.Write(b[:])
}

// I16 writes a signed 16-bit value. Sample levels are int16 and go negative,
// so the sign has to survive.
func (e *Encoder) I16(v int16) { e.U16(uint16(v)) }

// I32 writes a signed 32-bit value. The mixer's DC-block accumulators are
// int32 and signed, so the sign has to survive the round trip.
func (e *Encoder) I32(v int32) { e.U32(uint32(v)) }

// I64 writes a signed 64-bit value. The T-state counters are the reason it
// exists: they are int64 in the emulator and they are signed on disk so a
// negative cannot be mistaken for a huge positive.
func (e *Encoder) I64(v int64) { e.U64(uint64(v)) }

// F32 writes a float32's IEEE bits.
func (e *Encoder) F32(v float32) { e.U32(math.Float32bits(v)) }

// F64 writes a float64's IEEE bits.
func (e *Encoder) F64(v float64) { e.U64(math.Float64bits(v)) }

// Bytes writes a length-prefixed blob.
func (e *Encoder) Bytes(b []byte) {
	e.U32(uint32(len(b)))
	e.buf.Write(b)
}

// String writes a length-prefixed string.
func (e *Encoder) String(s string) {
	e.U32(uint32(len(s)))
	e.buf.WriteString(s)
}

// Count writes an element count for a component's own count-and-loop encoding,
// the counterpart of Decoder.Count. The built-in slice methods use it, and a
// component encoding something without one - a slice of blobs, or a ragged
// struct - uses it directly.
func (e *Encoder) Count(n int) { e.U32(uint32(n)) }

// Slices are length-prefixed and element-wise. The four below plus Bytes cover
// the types the machine state actually holds; anything else is a count followed
// by a loop in the component, which keeps the file readable.

func (e *Encoder) U16s(v []uint16) {
	e.U32(uint32(len(v)))
	for _, x := range v {
		e.U16(x)
	}
}

func (e *Encoder) U32s(v []uint32) {
	e.U32(uint32(len(v)))
	for _, x := range v {
		e.U32(x)
	}
}

func (e *Encoder) U64s(v []uint64) {
	e.U32(uint32(len(v)))
	for _, x := range v {
		e.U64(x)
	}
}

func (e *Encoder) I64s(v []int64) {
	e.U32(uint32(len(v)))
	for _, x := range v {
		e.I64(x)
	}
}

func (e *Encoder) Strings(v []string) {
	e.U32(uint32(len(v)))
	for _, x := range v {
		e.String(x)
	}
}
