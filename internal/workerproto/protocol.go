// Package workerproto defines the JSON-RPC 2.0 wire types shared by the MCP
// supervisor and the emulator worker. The supervisor spawns the worker as a
// child process and exchanges newline-delimited JSON messages over stdio.
package workerproto

import "encoding/json"

// Version is the JSON-RPC version string carried on every message.
const Version = "2.0"

// Request is one JSON-RPC request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is one JSON-RPC response.
type Response struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      int64     `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
}

// RPCError is a JSON-RPC error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Registers is a full Z80 register dump.
type Registers struct {
	PC     uint16 `json:"pc"`
	SP     uint16 `json:"sp"`
	AF     uint16 `json:"af"`
	BC     uint16 `json:"bc"`
	DE     uint16 `json:"de"`
	HL     uint16 `json:"hl"`
	AF2    uint16 `json:"af2"`
	BC2    uint16 `json:"bc2"`
	DE2    uint16 `json:"de2"`
	HL2    uint16 `json:"hl2"`
	IX     uint16 `json:"ix"`
	IY     uint16 `json:"iy"`
	I      uint8  `json:"i"`
	R      uint8  `json:"r"`
	IM     uint8  `json:"im"`
	IFF1   bool   `json:"iff1"`
	IFF2   bool   `json:"iff2"`
	Memptr uint16 `json:"memptr"`
}

// MachineSummary describes one built machine.
type MachineSummary struct {
	Name   string `json:"name"`
	Model  string `json:"model"`
	Ticks  int64  `json:"ticks"`
	Frames int64  `json:"frames"`
	PC     uint16 `json:"pc"`
}

// StepResult is the outcome of step / run_instructions / run_tstates.
type StepResult struct {
	Ticks int64  `json:"ticks"`
	PC    uint16 `json:"pc"`
}

// RunFramesResult is the outcome of run_frames.
type RunFramesResult struct {
	Ticks  int64  `json:"ticks"`
	Frames int64  `json:"frames"`
	PC     uint16 `json:"pc"`
}

// RunUntilResult is the outcome of run_until_pc.
type RunUntilResult struct {
	Hit   bool   `json:"hit"`
	Ticks int64  `json:"ticks"`
	PC    uint16 `json:"pc"`
}

// MemoryDump is a hex + ASCII representation of a memory range.
type MemoryDump struct {
	Hex   string `json:"hex"`
	ASCII string `json:"ascii"`
}

// PortValue is a single I/O port read.
type PortValue struct {
	Value uint8 `json:"value"`
}
