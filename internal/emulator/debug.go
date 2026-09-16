package emulator

// Debug and headless execution API.
//
// These methods are additive: they loop the existing step() and add no new
// emulation state. They exist for the MCP worker and any other headless driver
// that needs to run the machine deterministically and touch I/O without a
// window or audio device.

// TotalTicks returns the cumulative T-state count since reset.
func (e *Emulator) TotalTicks() int64 { return e.totalTicks }

// FrameCount returns the number of completed frames since reset.
func (e *Emulator) FrameCount() int64 { return e.frameCount }

// RunInstructions executes n instructions and returns the T-states consumed.
func (e *Emulator) RunInstructions(n int) int {
	executed := 0
	for i := 0; i < n; i++ {
		ts, _ := e.step()
		executed += ts
	}
	e.pumpAudio()
	return executed
}

// RunTStates executes instructions until at least n T-states have elapsed and
// returns the T-states consumed (>= n).
func (e *Emulator) RunTStates(n int) int {
	executed := 0
	for executed < n {
		ts, _ := e.step()
		executed += ts
	}
	e.pumpAudio()
	return executed
}

// RunUntil executes instructions until cond returns true. cond is checked
// before each instruction, so a condition that is already true stops
// immediately. maxInstructions <= 0 means unbounded. It returns whether the
// condition was hit and the T-states consumed.
func (e *Emulator) RunUntil(cond func() bool, maxInstructions int) (bool, int) {
	executed := 0
	for i := 0; maxInstructions <= 0 || i < maxInstructions; i++ {
		if cond() {
			e.pumpAudio()
			return true, executed
		}
		ts, _ := e.step()
		executed += ts
	}
	e.pumpAudio()
	return false, executed
}

// ReadPort reads an I/O port through the real port bus (open-collector AND of
// every matching handler; 0xFF when no handler claims the port).
func (e *Emulator) ReadPort(port uint16) uint8 { return e.portBus.Read(port) }

// WritePort writes an I/O port, fanning out to every matching handler.
func (e *Emulator) WritePort(port uint16, value uint8) { e.portBus.Write(port, value) }
