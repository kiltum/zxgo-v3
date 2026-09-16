package mem

// ContentionKind selects the ULA contention pattern a model uses. It is derived
// from model.Config.ContentionModel: the 128K and the +2A/+3 are timing
// identical here (228 T-states/line, 70908 per frame), so nothing about the
// timing separates them -- only the delay pattern does.
type ContentionKind uint8

const (
	// ContentionNone is the CPU test harness (empty ContentionModel) and the
	// Pentagon ("none"): no delays anywhere.
	ContentionNone ContentionKind = iota
	Contention48k
	Contention128k
	Contention2A3
)

// ContentionKindFor maps a Config.ContentionModel string to its pattern. An
// unknown name becomes ContentionNone: a wrong pattern is worse than no
// pattern, and the only names in the tree are the four below.
func ContentionKindFor(contentionModel string) ContentionKind {
	switch contentionModel {
	case "48k":
		return Contention48k
	case "128k":
		return Contention128k
	case "2a3":
		return Contention2A3
	default:
		return ContentionNone
	}
}

// The two ULA delay patterns, named the way Fuse names them (the per-T-state
// delays read left to right). The +2A/+3's contention window opens three
// T-states earlier than the others', so its array is phase-shifted rather than
// just different in value (ref/fuse-1.9.0/spectrum.c:60-62, :220-226).
var (
	pattern65432100 = [8]int{6, 5, 4, 3, 2, 1, 0, 0}
	pattern76543210 = [8]int{1, 0, 7, 6, 5, 4, 3, 2}
)

// contentionParams returns the pattern, the line length and the pattern's
// origin T-state for a kind. The origins are libspectrum's top_left_pixel minus
// one (14335 for the 48K, 14361 for the 128K and the +2A/+3, which share it);
// they are the constants that reproduce Fuse's own contention checksums
// exactly, so do not "correct" them to the values in pkg/model.
func contentionParams(kind ContentionKind) (*[8]int, int, int) {
	switch kind {
	case Contention48k:
		return &pattern65432100, 224, 14335
	case Contention128k:
		return &pattern65432100, 228, 14361
	case Contention2A3:
		return &pattern76543210, 228, 14361
	}
	return nil, 0, 0
}

// contentionDelayPattern returns the ULA contention delay for a T-state
// position within the frame, assuming the address IS in a contended bank.
//
// Each pixel line is 128 T-states of pattern followed by a gap, and only the
// 192 pixel lines carry contention at all.
func contentionDelayPattern(kind ContentionKind, tstate int) int {
	pattern, clockPerLine, patternStart := contentionParams(kind)
	if pattern == nil {
		return 0
	}
	offset := tstate - patternStart
	if offset < 0 {
		return 0 // before the first pixel line
	}
	if offset/clockPerLine >= 192 {
		return 0 // past the last pixel line (top/bottom border)
	}
	posInLine := offset % clockPerLine
	if posInLine >= 128 {
		return 0 // the horizontal blank gap
	}
	return pattern[posInLine%8]
}

// ioContentionDelayPattern returns the I/O contention delay for a T-state. The
// 48K and 128K contend the same way for ports as for memory (Fuse contends a
// port read with A0 low through the same table); the +2A/+3 has no contended
// ports at all -- Fuse specplus3.c installs "none" for no_mreq.
func ioContentionDelayPattern(kind ContentionKind, tstate int) int {
	if kind == Contention2A3 {
		return 0
	}
	return contentionDelayPattern(kind, tstate)
}
