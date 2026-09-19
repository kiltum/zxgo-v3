// Package state is the container for a full machine-state snapshot: a header, a
// stream of self-describing chunks, and the rules for reading a file written by
// another build.
//
// It is a leaf package like pkg/model - it imports nothing from the tree - so
// every component can depend on it for the Encoder and Decoder primitives
// without creating a cycle or pointing a dependency outward. The coordinator
// that decides *which* chunks a machine has lives in internal/emulator.
//
// # Shape
//
//	header: magic "ZXGOSTAT", container version, flags, cpuHz, model key,
//	        build id, chunk count
//	body:   chunk*, each id, version, flags, length, crc32, payload
//
// Little-endian throughout. The container version changes only when the header
// or the chunk envelope changes, never because a component's payload changed -
// that is what the per-chunk version is for.
//
// The header is envelope only: nothing a component owns lives in it, so magic,
// version and the model key are all checked before a single payload byte is
// trusted. Flags bit 0 deflates the body and never the header, so those checks
// happen on plaintext.
//
// # Versioning
//
// An unknown chunk id is skipped (the length makes it skippable) and reported.
// A chunk version newer than this build's is a refusal; an older one is the
// component's business - it migrates, or fills defaults, and can ask the
// Decoder which version it is reading. A missing optional chunk is a default
// and a report; a missing required chunk is a refusal. A model key that is not
// the running model's is a refusal, because the banks and the device set would
// not line up.
//
// # Reading is two passes
//
// Pass one walks the envelopes - magic, versions, ids, the required set, the
// model key, every length and every CRC - and refuses there. Only then does
// pass two call Load, in the order the caller listed the sections, so a failure
// the reader can see can never leave a machine half-applied.
package state

// Magic is the eight bytes every state file starts with. A file whose first
// eight bytes are not these is not this format, whatever extension it carries.
const Magic = "ZXGOSTAT"

// FormatVersion is the container format this build writes and the newest it
// reads. It is not a chunk version: bump it only for a change to the header or
// the chunk envelope.
const FormatVersion uint16 = 1

// Limits. A state file is untrusted input - it may be corrupt, truncated or
// hostile - so nothing in the format is sized from a field in the file without
// a bound first.
const (
	// MaxChunkSize is the largest single payload accepted. The biggest real
	// chunk today is a disk image (~800K) or a banked machine's RAM (1M); a GS
	// with its 2M target would still be far inside this.
	MaxChunkSize = 32 << 20

	// MaxChunks bounds the declared chunk count, which is read before any
	// chunk is. It is a sanity bound, not a preallocation: the reader grows its
	// list as it parses.
	MaxChunks = 4096

	// MaxBodySize bounds the body once decompressed, so a deflated file cannot
	// expand without limit.
	MaxBodySize = 64 << 20
)

// Header is the envelope the caller supplies on save and receives on load.
//
// Model is the model.AllModels key ("pentagon512"), not model.Config.Name (the
// display name), which is what makes it the right thing to compare when
// refusing a file saved on another machine - the two are separate strings and
// only the key identifies the machine. Build is informational: it goes into
// refusal messages and bug reports and never decides whether a file loads.
type Header struct {
	CPUHz int
	Model string
	Build string
}

// Options are the writer's choices. Loading needs none: everything it needs to
// know is in the header.
type Options struct {
	// Deflate compresses the body. On by default for a session file - RAM and a
	// mostly empty disk deflate well - and worth turning off for a file meant to
	// be looked at in a hex editor.
	Deflate bool
}

// Result is what a load did, whether or not it also returned an error: the
// refusals are all decided before anything is applied, so a Result accompanies
// a successful load and lists what was skipped or defaulted along the way.
// SessionInfo in the front end is built from this.
type Result struct {
	Header    Header
	Applied   []ID // sections that were present and loaded
	Skipped   []ID // unknown chunk ids, ignored
	Defaulted []ID // optional sections the file did not carry
}

// Spec is one chunk: who it is, which version this build speaks, whether its
// absence is fatal, and the two halves of its codec.
//
// The coordinator owns ids, versions and required-ness - the component only
// knows how to encode itself - and it lists the specs in load order, which is
// the order pass two calls Load in.
type Spec struct {
	ID       ID
	Version  uint16
	Required bool
	Save     func(*Encoder) error
	Load     func(*Decoder) error
}

// Component is the pair of methods a piece of machine state implements. A
// component's encoding runs inside its own package, so it reads and writes the
// fields it already has and needs no public accessors for them.
type Component interface {
	SaveState(*Encoder) error
	LoadState(*Decoder) error
}

// Identified is a component that knows which chunk it is written as. The
// emulator holds its AY chip through an interface, and the chunk id depends on
// which chip that is (a single AY, a TurboSound pair, a YM2203 pair), so the id
// travels with the chip rather than living in a type switch at the call site.
type Identified interface {
	Component
	ChunkID() ID
}

// SectionOf builds a Spec from a component that names its own chunk.
func SectionOf(version uint16, required bool, c Identified) Spec {
	return Section(c.ChunkID(), version, required, c)
}

// Section builds a Spec from a component. Version is the chunk version this
// build writes; Required says whether a file without it must be refused.
func Section(id ID, version uint16, required bool, c Component) Spec {
	return Spec{
		ID:       id,
		Version:  version,
		Required: required,
		Save:     c.SaveState,
		Load:     c.LoadState,
	}
}
