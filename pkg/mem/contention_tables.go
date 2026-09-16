package mem

// Precomputed contention tables, one per model pattern. These eliminate the
// need to recalculate the contention pattern on every memory access (AUDIT T3),
// which was doing division, modulo and array work on the hottest path.

// ContentionTable48K is precomputed for the 48K (224 T-states/line).
var ContentionTable48K [69888]uint8

// ContentionTable128K is precomputed for the 128K (228 T-states/line).
var ContentionTable128K [70908]uint8

// ContentionTable2A3 is precomputed for the +2A/+3: the 128K's line length and
// origin, but the 76543210 pattern rather than 65432100.
var ContentionTable2A3 [70908]uint8

// IOContentionTable48K and IOContentionTable128K are precomputed I/O
// contention. The +2A/+3 has no contended ports, so it needs no I/O table.
var (
	IOContentionTable48K  [69888]uint8
	IOContentionTable128K [70908]uint8
)

func init() {
	precomputeContentionTables()
}

// precomputeContentionTables fills the tables at package initialization. This
// is much faster than the previous per-access computation, which involved
// division and modulo operations.
func precomputeContentionTables() {
	for t := 0; t < len(ContentionTable48K); t++ {
		ContentionTable48K[t] = uint8(contentionDelayPattern(Contention48k, t))
		IOContentionTable48K[t] = uint8(ioContentionDelayPattern(Contention48k, t))
	}
	for t := 0; t < len(ContentionTable128K); t++ {
		ContentionTable128K[t] = uint8(contentionDelayPattern(Contention128k, t))
		IOContentionTable128K[t] = uint8(ioContentionDelayPattern(Contention128k, t))
	}
	for t := 0; t < len(ContentionTable2A3); t++ {
		ContentionTable2A3[t] = uint8(contentionDelayPattern(Contention2A3, t))
	}
}

// GetContentionDelay returns the precomputed memory contention delay for a
// pattern and a frame T-state. The unsigned cast makes a negative or
// past-the-end T-state a miss rather than a panic, for free.
func GetContentionDelay(kind ContentionKind, tstate int) uint8 {
	switch kind {
	case Contention48k:
		if uint(tstate) < uint(len(ContentionTable48K)) {
			return ContentionTable48K[tstate]
		}
	case Contention128k:
		if uint(tstate) < uint(len(ContentionTable128K)) {
			return ContentionTable128K[tstate]
		}
	case Contention2A3:
		if uint(tstate) < uint(len(ContentionTable2A3)) {
			return ContentionTable2A3[tstate]
		}
	}
	return 0
}

// GetIOContentionDelay returns the precomputed I/O contention delay for a
// pattern and a frame T-state. Callers still decide whether the port is
// contended at all (A0 low); the +2A/+3 has no contended ports, so every
// T-state returns 0 for it.
func GetIOContentionDelay(kind ContentionKind, tstate int) uint8 {
	switch kind {
	case Contention48k:
		if uint(tstate) < uint(len(IOContentionTable48K)) {
			return IOContentionTable48K[tstate]
		}
	case Contention128k:
		if uint(tstate) < uint(len(IOContentionTable128K)) {
			return IOContentionTable128K[tstate]
		}
	}
	return 0
}
