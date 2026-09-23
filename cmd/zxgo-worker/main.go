// Command zxgo-worker is the headless emulator worker for the MCP supervisor.
//
// It reads newline-delimited JSON-RPC 2.0 requests from stdin, runs one or more
// emulator machines in-process, and writes one JSON response per request to
// stdout. It has no window and no audio (NullOutput); the supervisor spawns it
// as a child and drives it over stdio.
package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kiltum/zxgo-v3/internal/emulator"
	"github.com/kiltum/zxgo-v3/internal/workerproto"
	"github.com/kiltum/zxgo-v3/pkg/media"
	"github.com/kiltum/zxgo-v3/pkg/model"
	"github.com/kiltum/zxgo-v3/pkg/sound"
)

// maxScanLine bounds a single request line (write_memory can be large).
const maxScanLine = 8 * 1024 * 1024

type worker struct {
	machines map[string]*emulator.Emulator
	out      *json.Encoder
}

func main() {
	w := &worker{
		machines: map[string]*emulator.Emulator{},
		out:      json.NewEncoder(os.Stdout),
	}

	setupLogging()

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), maxScanLine)
	for sc.Scan() {
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var req workerproto.Request
		if err := json.Unmarshal(line, &req); err != nil {
			w.respond(req.ID, nil, fmt.Errorf("bad request: %v", err))
			continue
		}
		w.dispatch(&req)
	}
	// EOF: the supervisor closed the pipe; exit cleanly.
}

func (w *worker) respond(id int64, result any, err error) {
	resp := workerproto.Response{JSONRPC: workerproto.Version, ID: id}
	if err != nil {
		resp.Error = &workerproto.RPCError{Code: -32000, Message: err.Error()}
	} else {
		resp.Result = result
	}
	_ = w.out.Encode(resp)
}

func (w *worker) dispatch(req *workerproto.Request) {
	var result any
	var err error

	switch req.Method {
	case "ping":
		result = "pong"
	case "build_machine":
		result, err = w.buildMachine(req.Params)
	case "destroy_machine":
		result, err = w.destroyMachine(req.Params)
	case "list_machines":
		result, err = w.listMachines()
	case "reset":
		result, err = w.reset(req.Params)
	case "step":
		result, err = w.step(req.Params)
	case "run_instructions":
		result, err = w.runInstructions(req.Params)
	case "run_tstates":
		result, err = w.runTStates(req.Params)
	case "run_frames":
		result, err = w.runFrames(req.Params)
	case "run_until_pc":
		result, err = w.runUntilPC(req.Params)
	case "read_registers":
		result, err = w.readRegisters(req.Params)
	case "read_memory":
		result, err = w.readMemory(req.Params)
	case "write_memory":
		result, err = w.writeMemory(req.Params)
	case "read_port":
		result, err = w.readPort(req.Params)
	case "write_port":
		result, err = w.writePort(req.Params)
	case "press_key":
		result, err = w.pressKey(req.Params)
	case "release_key":
		result, err = w.releaseKey(req.Params)
	case "set_joystick":
		result, err = w.setJoystick(req.Params)
	case "read_screen":
		result, err = w.readScreen(req.Params)
	case "read_screen_text":
		result, err = w.readScreenText(req.Params)
	case "type_text":
		result, err = w.typeText(req.Params)
	case "type_raw":
		result, err = w.typeRaw(req.Params)
	case "write_register":
		result, err = w.writeRegister(req.Params)
	case "disassemble":
		result, err = w.disassemble(req.Params)
	case "read_port_state":
		result, err = w.readPortState(req.Params)
	case "set_trace":
		result, err = w.setTrace(req.Params)
	case "trace_get":
		result, err = w.traceGet(req.Params)
	case "trace_diff":
		result, err = w.traceDiff(req.Params)
	case "get_logs":
		result, err = w.getLogs(req.Params)
	case "clear_logs":
		result, err = w.clearLogs(req.Params)
	case "set_log_level":
		result, err = w.setLogLevel(req.Params)
	case "load_snapshot":
		result, err = w.loadSnapshot(req.Params)
	case "save_snapshot":
		result, err = w.saveSnapshot(req.Params)
	case "tape_control":
		result, err = w.tapeControl(req.Params)
	case "shutdown":
		w.respond(req.ID, "ok", nil)
		os.Exit(0)
	default:
		err = fmt.Errorf("unknown method: %s", req.Method)
	}

	w.respond(req.ID, result, err)
}

func (w *worker) getMachine(name string) (*emulator.Emulator, error) {
	e, ok := w.machines[name]
	if !ok {
		return nil, fmt.Errorf("no such machine: %q", name)
	}
	return e, nil
}

func decodeParams(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return nil // empty params -> zero struct
	}
	return json.Unmarshal(raw, v)
}

func (w *worker) summary(name string, e *emulator.Emulator) workerproto.MachineSummary {
	return workerproto.MachineSummary{
		Name:   name,
		Model:  e.ModelConfig().Name,
		Ticks:  e.TotalTicks(),
		Frames: e.FrameCount(),
		PC:     e.CPU().PC,
	}
}

func (w *worker) buildMachine(raw json.RawMessage) (any, error) {
	var p struct {
		Name     string `json:"name"`
		Model    string `json:"model"`
		RomsDir  string `json:"roms_dir"`
		Disk     string `json:"disk"`
		Tape     string `json:"tape"`
		Snapshot string `json:"snapshot"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	if p.Name == "" {
		return nil, fmt.Errorf("build_machine: name is required")
	}
	if p.Model == "" {
		p.Model = "48k"
	}
	cfg, ok := model.AllModels[p.Model]
	if !ok {
		return nil, fmt.Errorf("unknown model: %q", p.Model)
	}
	romsDir := p.RomsDir
	if romsDir == "" {
		romsDir = "roms"
	}

	e, err := emulator.NewFromModel(cfg, romsDir, &sound.NullOutput{})
	if err != nil {
		return nil, fmt.Errorf("building %s: %w", p.Name, err)
	}
	e.Reset()

	if p.Disk != "" {
		disk, err := loadDisk(p.Disk)
		if err != nil {
			return nil, fmt.Errorf("loading disk: %w", err)
		}
		if err := e.LoadDisk(disk); err != nil {
			return nil, fmt.Errorf("mounting disk: %w", err)
		}
	}

	if p.Snapshot != "" {
		if err := loadSnapshotFile(e, p.Snapshot); err != nil {
			return nil, fmt.Errorf("loading snapshot: %w", err)
		}
	}

	if p.Tape != "" {
		if err := loadTapeFile(e, p.Tape); err != nil {
			return nil, fmt.Errorf("loading tape: %w", err)
		}
	}

	w.machines[p.Name] = e
	return w.summary(p.Name, e), nil
}

func (w *worker) destroyMachine(raw json.RawMessage) (any, error) {
	var p struct {
		Name string `json:"name"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	if _, ok := w.machines[p.Name]; !ok {
		return nil, fmt.Errorf("no such machine: %q", p.Name)
	}
	delete(w.machines, p.Name)
	return "ok", nil
}

func (w *worker) listMachines() (any, error) {
	out := make([]workerproto.MachineSummary, 0, len(w.machines))
	for name, e := range w.machines {
		out = append(out, w.summary(name, e))
	}
	return out, nil
}

func (w *worker) reset(raw json.RawMessage) (any, error) {
	var p struct {
		Name string `json:"name"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e, err := w.getMachine(p.Name)
	if err != nil {
		return nil, err
	}
	e.Reset()
	return "ok", nil
}

func (w *worker) step(raw json.RawMessage) (any, error) {
	var p struct {
		Name string `json:"name"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e, err := w.getMachine(p.Name)
	if err != nil {
		return nil, err
	}
	ticks := e.RunInstructions(1)
	return workerproto.StepResult{Ticks: int64(ticks), PC: e.CPU().PC}, nil
}

func (w *worker) runInstructions(raw json.RawMessage) (any, error) {
	var p struct {
		Name string `json:"name"`
		N    int    `json:"n"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e, err := w.getMachine(p.Name)
	if err != nil {
		return nil, err
	}
	ticks := e.RunInstructions(p.N)
	return workerproto.StepResult{Ticks: int64(ticks), PC: e.CPU().PC}, nil
}

func (w *worker) runTStates(raw json.RawMessage) (any, error) {
	var p struct {
		Name string `json:"name"`
		N    int    `json:"n"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e, err := w.getMachine(p.Name)
	if err != nil {
		return nil, err
	}
	ticks := e.RunTStates(p.N)
	return workerproto.StepResult{Ticks: int64(ticks), PC: e.CPU().PC}, nil
}

func (w *worker) runFrames(raw json.RawMessage) (any, error) {
	var p struct {
		Name string `json:"name"`
		N    int    `json:"n"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e, err := w.getMachine(p.Name)
	if err != nil {
		return nil, err
	}
	for i := 0; i < p.N; i++ {
		e.RunFrame()
	}
	return workerproto.RunFramesResult{Ticks: e.TotalTicks(), Frames: e.FrameCount(), PC: e.CPU().PC}, nil
}

// runUntilPCDefaultCap bounds a run_until_pc whose max_instructions is omitted or
// <= 0. A missed PC (wrong addr, or a target the code never reaches) must not
// hang the worker's single-threaded stdin RPC loop; 0 therefore means "default
// cap", never unbounded, at this boundary.
const runUntilPCDefaultCap = 50_000_000

func (w *worker) runUntilPC(raw json.RawMessage) (any, error) {
	var p struct {
		Name            string `json:"name"`
		Addr            int    `json:"addr"`
		MaxInstructions int    `json:"max_instructions"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e, err := w.getMachine(p.Name)
	if err != nil {
		return nil, err
	}
	target := uint16(p.Addr)
	max := p.MaxInstructions
	if max <= 0 {
		max = runUntilPCDefaultCap
	}
	hit, ticks := e.RunUntil(func() bool { return e.CPU().PC == target }, max)
	return workerproto.RunUntilResult{Hit: hit, Ticks: int64(ticks), PC: e.CPU().PC}, nil
}

func (w *worker) readRegisters(raw json.RawMessage) (any, error) {
	var p struct {
		Name string `json:"name"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e, err := w.getMachine(p.Name)
	if err != nil {
		return nil, err
	}
	c := e.CPU()
	return workerproto.Registers{
		PC:     c.PC,
		SP:     c.SP,
		AF:     uint16(c.A)<<8 | uint16(c.F),
		BC:     uint16(c.B)<<8 | uint16(c.C),
		DE:     uint16(c.D)<<8 | uint16(c.E),
		HL:     uint16(c.H)<<8 | uint16(c.L),
		AF2:    uint16(c.A_)<<8 | uint16(c.F_),
		BC2:    uint16(c.B_)<<8 | uint16(c.C_),
		DE2:    uint16(c.D_)<<8 | uint16(c.E_),
		HL2:    uint16(c.H_)<<8 | uint16(c.L_),
		IX:     c.IX,
		IY:     c.IY,
		I:      c.I,
		R:      c.R,
		IM:     c.IM,
		IFF1:   c.IFF1,
		IFF2:   c.IFF2,
		Memptr: c.MEMPTR,
	}, nil
}

func (w *worker) readMemory(raw json.RawMessage) (any, error) {
	var p struct {
		Name string `json:"name"`
		Addr int    `json:"addr"`
		Len  int    `json:"len"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e, err := w.getMachine(p.Name)
	if err != nil {
		return nil, err
	}
	if p.Len <= 0 || p.Len > 0x10000 {
		return nil, fmt.Errorf("read_memory: len must be in 1..65536")
	}
	m := e.Mapper()
	data := make([]byte, p.Len)
	for i := 0; i < p.Len; i++ {
		data[i] = m.ReadByte(uint16(p.Addr) + uint16(i))
	}
	return workerproto.MemoryDump{Hex: hex.EncodeToString(data), ASCII: asciiDump(data)}, nil
}

func (w *worker) writeMemory(raw json.RawMessage) (any, error) {
	var p struct {
		Name string `json:"name"`
		Addr int    `json:"addr"`
		Hex  string `json:"hex"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e, err := w.getMachine(p.Name)
	if err != nil {
		return nil, err
	}
	data, err := hexDecode(p.Hex)
	if err != nil {
		return nil, fmt.Errorf("write_memory: %v", err)
	}
	m := e.Mapper()
	for i, b := range data {
		m.WriteByte(uint16(p.Addr)+uint16(i), b)
	}
	return "ok", nil
}

func (w *worker) readPort(raw json.RawMessage) (any, error) {
	var p struct {
		Name string `json:"name"`
		Port int    `json:"port"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e, err := w.getMachine(p.Name)
	if err != nil {
		return nil, err
	}
	return workerproto.PortValue{Value: e.ReadPort(uint16(p.Port))}, nil
}

func (w *worker) writePort(raw json.RawMessage) (any, error) {
	var p struct {
		Name  string `json:"name"`
		Port  int    `json:"port"`
		Value int    `json:"value"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e, err := w.getMachine(p.Name)
	if err != nil {
		return nil, err
	}
	e.WritePort(uint16(p.Port), uint8(p.Value))
	return "ok", nil
}

func (w *worker) pressKey(raw json.RawMessage) (any, error) {
	var p struct {
		Name string `json:"name"`
		Row  int    `json:"row"`
		Col  int    `json:"col"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e, err := w.getMachine(p.Name)
	if err != nil {
		return nil, err
	}
	e.PressKey(p.Row, p.Col)
	return "ok", nil
}

func (w *worker) releaseKey(raw json.RawMessage) (any, error) {
	var p struct {
		Name string `json:"name"`
		Row  int    `json:"row"`
		Col  int    `json:"col"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e, err := w.getMachine(p.Name)
	if err != nil {
		return nil, err
	}
	e.ReleaseKey(p.Row, p.Col)
	return "ok", nil
}

// setJoystick writes the Kempston joystick. It is the whole state at once, as the
// port is, and it goes through the emulator's own input method so that a session
// driven from here records and replays like one driven by a gamepad.
func (w *worker) setJoystick(raw json.RawMessage) (any, error) {
	var p struct {
		Name  string `json:"name"`
		Right bool   `json:"right"`
		Left  bool   `json:"left"`
		Down  bool   `json:"down"`
		Up    bool   `json:"up"`
		Fire  bool   `json:"fire"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e, err := w.getMachine(p.Name)
	if err != nil {
		return nil, err
	}
	e.SetJoystick(p.Right, p.Left, p.Down, p.Up, p.Fire)
	// The readable state back, so a caller can see what the port now holds without
	// a second call: the five booleans it just sent, as the byte the machine reads.
	return map[string]any{
		"kempston": e.Kempston().Read(0x1F),
		"right":    p.Right,
		"left":     p.Left,
		"down":     p.Down,
		"up":       p.Up,
		"fire":     p.Fire,
	}, nil
}

func (w *worker) readScreen(raw json.RawMessage) (any, error) {
	var p struct {
		Name string `json:"name"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e, err := w.getMachine(p.Name)
	if err != nil {
		return nil, err
	}
	return e.ScreenASCII(), nil
}

func (w *worker) readScreenText(raw json.RawMessage) (any, error) {
	var p struct {
		Name string `json:"name"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e, err := w.getMachine(p.Name)
	if err != nil {
		return nil, err
	}
	return e.ScreenText(), nil
}

func (w *worker) typeText(raw json.RawMessage) (any, error) {
	var p struct {
		Name   string `json:"name"`
		Text   string `json:"text"`
		DownMS int    `json:"down_ms"`
		UpMS   int    `json:"up_ms"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e, err := w.getMachine(p.Name)
	if err != nil {
		return nil, err
	}
	typeText(e, p.Text, msToFrames(p.DownMS, 100), msToFrames(p.UpMS, 200))
	return "ok", nil
}

func (w *worker) typeRaw(raw json.RawMessage) (any, error) {
	var p struct {
		Name     string `json:"name"`
		Sequence []struct {
			Row    int  `json:"row"`
			Col    int  `json:"col"`
			Shift  bool `json:"shift"`
			Caps   bool `json:"caps"`
			DownMS int  `json:"down_ms"`
			UpMS   int  `json:"up_ms"`
		} `json:"sequence"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e, err := w.getMachine(p.Name)
	if err != nil {
		return nil, err
	}
	for _, k := range p.Sequence {
		typeKey(e, keyPress{row: k.Row, col: k.Col, shift: k.Shift, caps: k.Caps}, msToFrames(k.DownMS, 100), msToFrames(k.UpMS, 200))
	}
	return "ok", nil
}

func (w *worker) writeRegister(raw json.RawMessage) (any, error) {
	var p struct {
		Name     string `json:"name"`
		Register string `json:"register"`
		Value    int    `json:"value"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e, err := w.getMachine(p.Name)
	if err != nil {
		return nil, err
	}
	c := e.CPU()
	switch strings.ToUpper(p.Register) {
	case "PC":
		c.PC = uint16(p.Value)
	case "SP":
		c.SP = uint16(p.Value)
	case "A":
		c.A = uint8(p.Value)
	case "F":
		c.F = uint8(p.Value)
	case "B":
		c.B = uint8(p.Value)
	case "C":
		c.C = uint8(p.Value)
	case "D":
		c.D = uint8(p.Value)
	case "E":
		c.E = uint8(p.Value)
	case "H":
		c.H = uint8(p.Value)
	case "L":
		c.L = uint8(p.Value)
	case "IX":
		c.IX = uint16(p.Value)
	case "IY":
		c.IY = uint16(p.Value)
	case "I":
		c.I = uint8(p.Value)
	case "R":
		c.R = uint8(p.Value)
	case "IM":
		c.IM = uint8(p.Value)
	case "IFF1":
		c.IFF1 = p.Value != 0
	case "IFF2":
		c.IFF2 = p.Value != 0
	default:
		return nil, fmt.Errorf("unknown register: %q", p.Register)
	}
	return "ok", nil
}

// loadDisk opens and decodes a disk image by extension (.trd/.scl/.dsk). The
// path may be a .zip holding the image; media.LoadDiskFile unpacks it and picks
// the format from the name inside.
func loadDisk(path string) (*media.Disk, error) {
	return media.LoadDiskFile(path)
}

// asciiDump renders bytes as printable ASCII with '.' for non-printables.
func asciiDump(data []byte) string {
	var b strings.Builder
	for _, c := range data {
		if c >= 0x20 && c <= 0x7E {
			b.WriteByte(c)
		} else {
			b.WriteByte('.')
		}
	}
	return b.String()
}

// hexDecode parses a hex string, tolerating whitespace and ','/'-' separators.
func hexDecode(s string) ([]byte, error) {
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r', ',', '-':
			return -1
		}
		return r
	}, s)
	if cleaned == "" {
		return nil, nil
	}
	if len(cleaned)%2 != 0 {
		return nil, fmt.Errorf("hex string has odd length")
	}
	return hex.DecodeString(cleaned)
}
