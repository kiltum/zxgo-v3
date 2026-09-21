package frontend

// RunState is what the machine is doing between ticks. UI_DESIGN.md section 5.2.
type RunState int

const (
	// StatePaused polls input and presents, and does not advance the CPU.
	StatePaused RunState = iota
	// StateRun takes one RunSlice per tick.
	StateRun
	// StateStep executes a single instruction and falls back to paused.
	StateStep
)

func (s RunState) String() string {
	switch s {
	case StatePaused:
		return "paused"
	case StateRun:
		return "running"
	case StateStep:
		return "step"
	}
	return "unknown"
}
