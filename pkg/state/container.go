package state

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"math"
)

// Header flags.
const (
	// flagDeflate marks a body that went through flate. The header itself is
	// never compressed, so the magic, the version and the model key can be
	// checked, and refused, before a decompressor is started.
	flagDeflate uint16 = 1 << 0

	headerFlagsKnown = flagDeflate
)

// headerFixedSize is magic(8) + version(2) + flags(2) + cpuHz(4) + chunks(4)
// plus the two length prefixes of the strings.
const (
	headerFixedSize = 8 + 2 + 2 + 4 + 4
	chunkHeaderSize = 4 + 2 + 2 + 4 + 4 // id, version, flags, length, crc32
)

// Save writes a complete state file: the header, then one chunk per section in
// the order given.
//
// Save order does not matter - sections are independent - so the caller's order
// is used as it stands. A payload larger than the envelope can describe is a
// refusal rather than a truncated length.
func Save(w io.Writer, h Header, specs []Spec, opts Options) error {
	if err := checkSpecs(specs, true); err != nil {
		return err
	}

	var headerFlags uint16
	if opts.Deflate {
		headerFlags |= flagDeflate
	}

	// The header goes out first and uncompressed. It is small, and writing it
	// before the body means a caller that fails mid-body still leaves a file
	// whose magic and model are readable.
	var fixed [headerFixedSize]byte
	copy(fixed[0:8], Magic)
	binary.LittleEndian.PutUint16(fixed[8:], FormatVersion)
	binary.LittleEndian.PutUint16(fixed[10:], headerFlags)
	binary.LittleEndian.PutUint32(fixed[12:], uint32(h.CPUHz))
	binary.LittleEndian.PutUint32(fixed[16:], uint32(len(specs)))
	if _, err := w.Write(fixed[:]); err != nil {
		return fmt.Errorf("state: writing header: %w", err)
	}
	if err := writePString(w, h.Model); err != nil {
		return err
	}
	if err := writePString(w, h.Build); err != nil {
		return err
	}

	body := w
	var deflater *flate.Writer
	if opts.Deflate {
		var err error
		deflater, err = flate.NewWriter(w, flate.DefaultCompression)
		if err != nil {
			return fmt.Errorf("state: starting compressor: %w", err)
		}
		body = deflater
	}

	enc := NewEncoder()
	for _, s := range specs {
		enc.Reset()
		if err := s.Save(enc); err != nil {
			return fmt.Errorf("chunk %d (%s): %w", uint32(s.ID), s.ID, err)
		}
		if err := writeChunk(body, s, enc.Payload()); err != nil {
			return err
		}
	}

	if deflater != nil {
		if err := deflater.Close(); err != nil {
			return fmt.Errorf("state: finishing compressed body: %w", err)
		}
	}
	return nil
}

// writeChunk writes one chunk envelope and its payload.
func writeChunk(w io.Writer, s Spec, payload []byte) error {
	if len(payload) > math.MaxUint32 {
		return chunkError(s.ID, s.Version, ErrChunkTooLarge)
	}
	var hdr [chunkHeaderSize]byte
	binary.LittleEndian.PutUint32(hdr[0:], uint32(s.ID))
	binary.LittleEndian.PutUint16(hdr[4:], s.Version)
	binary.LittleEndian.PutUint16(hdr[6:], 0) // chunk flags, reserved
	binary.LittleEndian.PutUint32(hdr[8:], uint32(len(payload)))
	binary.LittleEndian.PutUint32(hdr[12:], crc32.ChecksumIEEE(payload))
	if _, err := w.Write(hdr[:]); err != nil {
		return fmt.Errorf("state: writing chunk %d (%s): %w", uint32(s.ID), s.ID, err)
	}
	if _, err := w.Write(payload); err != nil {
		return fmt.Errorf("state: writing chunk %d (%s): %w", uint32(s.ID), s.ID, err)
	}
	return nil
}

// Load reads a state file and applies it to the sections the caller listed.
//
// model is the running machine's model key; a file saved on another one is
// refused rather than half-applied. The specification, not the file, decides
// which sections exist, in which order they load, and which of them are
// required.
//
// The file is read in two passes. Pass one validates every envelope - magic,
// versions, ids, lengths, checksums, the required set, the model key - and
// everything it finds wrong is refused there, before any Load runs. Pass two
// applies the sections in the caller's order. A Result comes back even
// alongside an error, so a caller can see what was skipped or defaulted.
func Load(r io.Reader, model string, specs []Spec) (*Result, error) {
	res := &Result{}
	if err := checkSpecs(specs, false); err != nil {
		return res, err
	}

	hdr, body, err := readEnvelope(r, model)
	if err != nil {
		return res, err
	}
	res.Header = hdr.header

	chunks, err := parseChunks(body, hdr.declared, len(specs))
	if err != nil {
		return res, err
	}

	byID := make(map[ID]parsedChunk, len(chunks))

	// Pass one, the part that needs the specification: every refusal a reader
	// can see is decided here, so nothing below can leave the machine changed
	// only in part.
	known := make(map[ID]Spec, len(specs))
	for _, s := range specs {
		known[s.ID] = s
	}
	for _, c := range chunks {
		s, ok := known[c.id]
		if !ok {
			res.Skipped = append(res.Skipped, c.id)
			continue
		}
		if c.version > s.Version {
			return res, chunkError(c.id, c.version, ErrChunkVersion)
		}
		byID[c.id] = c
	}
	for _, s := range specs {
		if _, ok := byID[s.ID]; !ok {
			if s.Required {
				return res, chunkPlainError(s.ID, ErrMissingChunk)
			}
			res.Defaulted = append(res.Defaulted, s.ID)
		}
	}

	// Pass two: apply, in the caller's order. A component that refuses its own
	// payload stops the load; the ones already applied stay applied, and no
	// single component is left half-done (each assigns only after its whole
	// payload decoded).
	for _, s := range specs {
		c, ok := byID[s.ID]
		if !ok {
			continue
		}
		d := NewDecoder(c.payload, c.version)
		if err := s.Load(d); err != nil {
			return res, chunkError(s.ID, s.Version, fmt.Errorf("%w: %v", ErrLoad, err))
		}
		res.Applied = append(res.Applied, s.ID)
	}
	return res, nil
}

// parsedChunk is one chunk as read from the body, already checksummed.
type parsedChunk struct {
	id      ID
	version uint16
	payload []byte
}

// envelope is the header as read, plus the bytes that follow it.
type envelope struct {
	header   Header
	declared uint32
}

// readEnvelope reads and validates the header, then returns it with the body
// bytes.
//
// The body is read whole - decompressed, if the flag says so - because pass one
// has to walk every envelope and its checksum before pass two can apply
// anything, and a stream cannot be walked twice. It is bounded while reading, so
// a hostile deflate stream cannot expand without limit.
func readEnvelope(r io.Reader, model string) (envelope, []byte, error) {
	var e envelope

	var fixed [headerFixedSize]byte
	if _, err := io.ReadFull(r, fixed[:]); err != nil {
		// A file too short to hold the magic is not this format; one that holds
		// the magic but stops mid-header is truncated.
		if bytes.HasPrefix(fixed[:], []byte(Magic)) {
			return e, nil, fmt.Errorf("state: reading header: %w", ErrTruncated)
		}
		return e, nil, ErrNotState
	}
	if string(fixed[0:8]) != Magic {
		return e, nil, ErrNotState
	}
	if v := binary.LittleEndian.Uint16(fixed[8:]); v > FormatVersion {
		return e, nil, fmt.Errorf("state: %w (file %d, this build %d)", ErrFormatVersion, v, FormatVersion)
	}
	flags := binary.LittleEndian.Uint16(fixed[10:])
	if flags&^headerFlagsKnown != 0 {
		return e, nil, fmt.Errorf("state: header %w (%#04x)", ErrUnknownFlag, flags&^headerFlagsKnown)
	}
	e.header.CPUHz = int(int32(binary.LittleEndian.Uint32(fixed[12:])))
	e.declared = binary.LittleEndian.Uint32(fixed[16:])
	if e.declared > MaxChunks {
		return e, nil, fmt.Errorf("state: %w (%d)", ErrTooManyChunks, e.declared)
	}

	str, err := readPString(r)
	if err != nil {
		return e, nil, err
	}
	e.header.Model = str
	str, err = readPString(r)
	if err != nil {
		return e, nil, err
	}
	e.header.Build = str

	if e.header.Model != model {
		return e, nil, fmt.Errorf("state: %w (file %q, this machine %q)", ErrModel, e.header.Model, model)
	}

	var src io.Reader = r
	if flags&flagDeflate != 0 {
		fr := flate.NewReader(r)
		defer fr.Close()
		src = fr
	}
	body, err := io.ReadAll(io.LimitReader(src, MaxBodySize+1))
	if err != nil {
		return e, nil, fmt.Errorf("state: reading body: %w", err)
	}
	if len(body) > MaxBodySize {
		return e, nil, fmt.Errorf("state: %w (body expands past %d bytes)", ErrChunkTooLarge, MaxBodySize)
	}
	return e, body, nil
}

// parseChunks walks the body's envelopes: every length is bounded before it is
// used, every checksum is verified, and the count has to be the one the header
// declared.
func parseChunks(body []byte, declared uint32, hint int) ([]parsedChunk, error) {
	chunks := make([]parsedChunk, 0, hint)
	seen := make(map[ID]bool, hint)

	for pos := 0; pos < len(body); {
		if pos+chunkHeaderSize > len(body) {
			return nil, fmt.Errorf("state: %w (chunk envelope at byte %d)", ErrTruncated, pos)
		}
		id := ID(binary.LittleEndian.Uint32(body[pos:]))
		version := binary.LittleEndian.Uint16(body[pos+4:])
		flags := binary.LittleEndian.Uint16(body[pos+6:])
		length := binary.LittleEndian.Uint32(body[pos+8:])
		sum := binary.LittleEndian.Uint32(body[pos+12:])
		pos += chunkHeaderSize

		if flags != 0 {
			return nil, chunkError(id, version, fmt.Errorf("%w (%#04x)", ErrUnknownFlag, flags))
		}
		if length > MaxChunkSize {
			return nil, chunkError(id, version, fmt.Errorf("%w (%d bytes)", ErrChunkTooLarge, length))
		}
		if uint64(length) > uint64(len(body)-pos) {
			return nil, chunkError(id, version, ErrTruncated)
		}
		payload := body[pos : pos+int(length)]
		pos += int(length)

		if got := crc32.ChecksumIEEE(payload); got != sum {
			return nil, chunkError(id, version, fmt.Errorf("%w (%08x, envelope says %08x)", ErrCRC, got, sum))
		}
		if seen[id] {
			return nil, chunkPlainError(id, ErrDuplicateChunk)
		}
		seen[id] = true

		chunks = append(chunks, parsedChunk{id: id, version: version, payload: payload})
	}
	// The count in the header is what a reader promises about the file, so a
	// body that holds a different number of chunks is malformed even when every
	// chunk it does hold is valid - which is what a file cut short at a chunk
	// boundary looks like once the truncation has not left a partial envelope.
	if uint32(len(chunks)) != declared {
		return nil, fmt.Errorf("state: %w (header says %d, body holds %d)", ErrChunkCount, declared, len(chunks))
	}
	return chunks, nil
}

// checkSpecs rejects a section table that cannot be used. save says which half
// of each Spec has to be present.
func checkSpecs(specs []Spec, save bool) error {
	seen := make(map[ID]bool, len(specs))
	for _, s := range specs {
		if seen[s.ID] {
			return fmt.Errorf("state: %w: id %s listed twice", ErrBadSpec, s.ID)
		}
		seen[s.ID] = true
		if save && s.Save == nil {
			return fmt.Errorf("state: %w: id %s has no SaveState", ErrBadSpec, s.ID)
		}
		if !save && s.Load == nil {
			return fmt.Errorf("state: %w: id %s has no LoadState", ErrBadSpec, s.ID)
		}
	}
	return nil
}

// writePString writes a length-prefixed string.
func writePString(w io.Writer, s string) error {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(len(s)))
	if _, err := w.Write(b[:]); err != nil {
		return fmt.Errorf("state: writing header string: %w", err)
	}
	if _, err := io.WriteString(w, s); err != nil {
		return fmt.Errorf("state: writing header string: %w", err)
	}
	return nil
}

// readPString reads a length-prefixed string, bounded so a corrupt length
// cannot ask for an unbounded read.
func readPString(r io.Reader) (string, error) {
	var b [4]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return "", fmt.Errorf("state: reading header string: %w", ErrTruncated)
	}
	n := binary.LittleEndian.Uint32(b[:])
	if n > MaxChunkSize {
		return "", fmt.Errorf("state: header string %w (%d bytes)", ErrChunkTooLarge, n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", fmt.Errorf("state: reading header string: %w", ErrTruncated)
	}
	return string(buf), nil
}
