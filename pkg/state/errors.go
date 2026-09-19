package state

import (
	"errors"
	"fmt"
)

// Refusals. Every one of these is a sentinel a caller can test with errors.Is,
// so a session restore can decide to start clean and an explicit load can tell
// the user why, without either of them parsing a message. The wrapping always
// adds the detail (which chunk, which version) around one of these.
var (
	// ErrNotState is a file whose magic is not this format's.
	ErrNotState = errors.New("not a state file")

	// ErrFormatVersion is a container version this build does not speak. Only
	// one direction is guaranteed: new builds read old files.
	ErrFormatVersion = errors.New("container version is newer than this build")

	// ErrUnknownFlag is a header or chunk flag bit this build does not know.
	// Flags change what a payload means, so an unknown one cannot be ignored
	// the way an unknown chunk id can.
	ErrUnknownFlag = errors.New("unknown flag bit")

	// ErrModel is a file saved on another model. The banks and the device set
	// would not line up, so nothing is applied.
	ErrModel = errors.New("saved on a different model")

	// ErrTruncated is a file that ends before its own declared contents do,
	// including a deflate stream that stops mid-way.
	ErrTruncated = errors.New("file is truncated")

	// ErrCRC is a chunk whose bytes do not match the checksum the envelope
	// carries. The container is well formed; the payload inside it is not.
	ErrCRC = errors.New("chunk failed its checksum")

	// ErrChunkVersion is a chunk written by a newer build. The component cannot
	// know what a field it has never seen means, so the whole file is refused
	// rather than the chunk being skipped.
	ErrChunkVersion = errors.New("chunk version is newer than this build")

	// ErrChunkTooLarge is a chunk longer than MaxChunkSize.
	ErrChunkTooLarge = errors.New("chunk is larger than the format allows")

	// ErrTooManyChunks is a chunk count beyond MaxChunks.
	ErrTooManyChunks = errors.New("chunk count is implausible")

	// ErrChunkCount is a body that does not hold the number of chunks the
	// header declared.
	ErrChunkCount = errors.New("chunk count does not match the header")

	// ErrDuplicateChunk is the same id twice in one file.
	ErrDuplicateChunk = errors.New("chunk id appears more than once")

	// ErrMissingChunk is a required section the file does not carry.
	ErrMissingChunk = errors.New("required chunk is missing")

	// ErrBadSpec is a section table that cannot be used: a nil codec, or the
	// same id listed twice. It is a caller bug, so it is reported rather than
	// refused silently.
	ErrBadSpec = errors.New("invalid section table")

	// ErrLoad is a component's own LoadState failing. The component decides
	// what is wrong with its payload; this wraps it with which chunk it was.
	ErrLoad = errors.New("component refused its payload")
)

// DecodeError is what the Decoder latches: what went wrong, in words a bug
// report or a log line can carry.
type DecodeError struct {
	What   string
	Detail string
}

func (e *DecodeError) Error() string {
	if e.Detail == "" {
		return "state: " + e.What
	}
	return "state: " + e.What + ": " + e.Detail
}

// chunkError wraps a sentinel with the chunk it came from, so a refusal reads
// as "chunk 6 (ula) v2: chunk version is newer than this build".
func chunkError(id ID, version uint16, err error) error {
	return fmt.Errorf("chunk %d (%s) v%d: %w", uint32(id), id, version, err)
}

// chunkErrorNoVersion is chunkError for the failures that happen before a
// version is known.
func chunkPlainError(id ID, err error) error {
	return fmt.Errorf("chunk %d (%s): %w", uint32(id), id, err)
}
