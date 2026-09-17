package main

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerEmulatorTools wires the machine lifecycle, execution, and inspection
// tools. Each one forwards to the emulator worker over stdio JSON-RPC. The
// agent may be nil when the worker binary is not built yet; forward() then
// returns a clear error instead of failing.
func registerEmulatorTools(s *server.MCPServer, h *workerHandle, log *slog.Logger) {
	s.AddTool(mcp.NewTool("build_machine",
		mcp.WithDescription("Build a headless emulator machine. Returns a machine summary."),
		mcp.WithString("name", mcp.Description("Machine name."), mcp.Required()),
		mcp.WithString("model", mcp.Description("Model: 48k, 128k, 2a3, pentagon (default 48k).")),
		mcp.WithString("roms_dir", mcp.Description("Directory with ROM files (default 'roms').")),
		mcp.WithString("disk", mcp.Description("Path to a disk image (.trd/.scl/.dsk), or a .zip holding one.")),
		mcp.WithString("tape", mcp.Description("Path to a tape image (.tap/.tzx), or a .zip holding one; mounted, not started.")),
		mcp.WithString("snapshot", mcp.Description("Path to a snapshot image (.sna/.z80), or a .zip holding one.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		return forward(h, "build_machine", map[string]any{
			"name":     name,
			"model":    req.GetString("model", "48k"),
			"roms_dir": req.GetString("roms_dir", "roms"),
			"disk":     req.GetString("disk", ""),
			"tape":     req.GetString("tape", ""),
			"snapshot": req.GetString("snapshot", ""),
		}), nil
	})

	s.AddTool(mcp.NewTool("destroy_machine",
		mcp.WithDescription("Destroy a machine and free its resources."),
		mcp.WithString("name", mcp.Description("Machine name."), mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		return forward(h, "destroy_machine", map[string]any{"name": name}), nil
	})

	s.AddTool(mcp.NewTool("list_machines",
		mcp.WithDescription("List all machines with model, ticks, frames, and PC."),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return forward(h, "list_machines", nil), nil
	})

	s.AddTool(mcp.NewTool("reset",
		mcp.WithDescription("Reset a machine to its power-on state."),
		mcp.WithString("name", mcp.Description("Machine name."), mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		return forward(h, "reset", map[string]any{"name": name}), nil
	})

	s.AddTool(mcp.NewTool("step",
		mcp.WithDescription("Execute one instruction. Returns ticks and new PC."),
		mcp.WithString("name", mcp.Description("Machine name."), mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		return forward(h, "step", map[string]any{"name": name}), nil
	})

	s.AddTool(mcp.NewTool("run_instructions",
		mcp.WithDescription("Execute n instructions. Returns ticks and final PC."),
		mcp.WithString("name", mcp.Description("Machine name."), mcp.Required()),
		mcp.WithNumber("n", mcp.Description("Number of instructions."), mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		n := argInt(req, "n", 0)
		return forward(h, "run_instructions", map[string]any{"name": name, "n": n}), nil
	})

	s.AddTool(mcp.NewTool("run_tstates",
		mcp.WithDescription("Execute until n T-states have elapsed. Returns ticks and final PC."),
		mcp.WithString("name", mcp.Description("Machine name."), mcp.Required()),
		mcp.WithNumber("n", mcp.Description("Number of T-states."), mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		n := argInt(req, "n", 0)
		return forward(h, "run_tstates", map[string]any{"name": name, "n": n}), nil
	})

	s.AddTool(mcp.NewTool("run_frames",
		mcp.WithDescription("Execute n frames. Returns ticks, frames, and final PC."),
		mcp.WithString("name", mcp.Description("Machine name."), mcp.Required()),
		mcp.WithNumber("n", mcp.Description("Number of frames."), mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		n := argInt(req, "n", 0)
		return forward(h, "run_frames", map[string]any{"name": name, "n": n}), nil
	})

	s.AddTool(mcp.NewTool("run_until_pc",
		mcp.WithDescription("Execute until PC reaches addr, or max_instructions. Returns hit, ticks, PC."),
		mcp.WithString("name", mcp.Description("Machine name."), mcp.Required()),
		mcp.WithNumber("addr", mcp.Description("Target PC address (decimal, 0-65535)."), mcp.Required()),
		mcp.WithNumber("max_instructions", mcp.Description("Instruction cap (default 50000000; 0 means the same default, never unbounded).")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		addr := argInt(req, "addr", 0)
		max := argInt(req, "max_instructions", 0)
		return forward(h, "run_until_pc", map[string]any{"name": name, "addr": addr, "max_instructions": max}), nil
	})

	s.AddTool(mcp.NewTool("read_registers",
		mcp.WithDescription("Dump all Z80 registers."),
		mcp.WithString("name", mcp.Description("Machine name."), mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		return forward(h, "read_registers", map[string]any{"name": name}), nil
	})

	s.AddTool(mcp.NewTool("read_memory",
		mcp.WithDescription("Read len bytes from addr. Returns hex + ASCII."),
		mcp.WithString("name", mcp.Description("Machine name."), mcp.Required()),
		mcp.WithNumber("addr", mcp.Description("Start address (decimal)."), mcp.Required()),
		mcp.WithNumber("len", mcp.Description("Number of bytes (1-65536)."), mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		addr := argInt(req, "addr", 0)
		l := argInt(req, "len", 0)
		return forward(h, "read_memory", map[string]any{"name": name, "addr": addr, "len": l}), nil
	})

	s.AddTool(mcp.NewTool("write_memory",
		mcp.WithDescription("Write bytes (hex string) to addr."),
		mcp.WithString("name", mcp.Description("Machine name."), mcp.Required()),
		mcp.WithNumber("addr", mcp.Description("Start address (decimal)."), mcp.Required()),
		mcp.WithString("hex", mcp.Description("Hex bytes, e.g. '3e00'."), mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		addr := argInt(req, "addr", 0)
		hexStr, _ := req.RequireString("hex")
		return forward(h, "write_memory", map[string]any{"name": name, "addr": addr, "hex": hexStr}), nil
	})

	s.AddTool(mcp.NewTool("read_port",
		mcp.WithDescription("Read an I/O port (some ports have read side-effects)."),
		mcp.WithString("name", mcp.Description("Machine name."), mcp.Required()),
		mcp.WithNumber("port", mcp.Description("Port address (decimal)."), mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		port := argInt(req, "port", 0)
		return forward(h, "read_port", map[string]any{"name": name, "port": port}), nil
	})

	s.AddTool(mcp.NewTool("write_port",
		mcp.WithDescription("Write a value to an I/O port."),
		mcp.WithString("name", mcp.Description("Machine name."), mcp.Required()),
		mcp.WithNumber("port", mcp.Description("Port address (decimal)."), mcp.Required()),
		mcp.WithNumber("value", mcp.Description("Value (0-255)."), mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		port := argInt(req, "port", 0)
		value := argInt(req, "value", 0)
		return forward(h, "write_port", map[string]any{"name": name, "port": port, "value": value}), nil
	})

	s.AddTool(mcp.NewTool("press_key",
		mcp.WithDescription("Press a key in the ZX keyboard matrix (row 0-7, col 0-4)."),
		mcp.WithString("name", mcp.Description("Machine name."), mcp.Required()),
		mcp.WithNumber("row", mcp.Description("Row 0-7."), mcp.Required()),
		mcp.WithNumber("col", mcp.Description("Column 0-4."), mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		row := argInt(req, "row", 0)
		col := argInt(req, "col", 0)
		return forward(h, "press_key", map[string]any{"name": name, "row": row, "col": col}), nil
	})

	s.AddTool(mcp.NewTool("release_key",
		mcp.WithDescription("Release a key in the ZX keyboard matrix (row 0-7, col 0-4)."),
		mcp.WithString("name", mcp.Description("Machine name."), mcp.Required()),
		mcp.WithNumber("row", mcp.Description("Row 0-7."), mcp.Required()),
		mcp.WithNumber("col", mcp.Description("Column 0-4."), mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		row := argInt(req, "row", 0)
		col := argInt(req, "col", 0)
		return forward(h, "release_key", map[string]any{"name": name, "row": row, "col": col}), nil
	})

	s.AddTool(mcp.NewTool("read_screen",
		mcp.WithDescription("Read the screen as an ASCII brightness map."),
		mcp.WithString("name", mcp.Description("Machine name."), mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		return forwardText(h, "read_screen", map[string]any{"name": name}), nil
	})

	s.AddTool(mcp.NewTool("read_screen_text",
		mcp.WithDescription("Read the screen as a 32x24 text grid (display file + ROM font)."),
		mcp.WithString("name", mcp.Description("Machine name."), mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		return forwardText(h, "read_screen_text", map[string]any{"name": name}), nil
	})

	s.AddTool(mcp.NewTool("type_text",
		mcp.WithDescription("Type text via the ZX keyboard. Letters/digits unshifted; symbols use SYMBOL SHIFT. Human-like press/hold/release timing."),
		mcp.WithString("name", mcp.Description("Machine name."), mcp.Required()),
		mcp.WithString("text", mcp.Description("Text to type."), mcp.Required()),
		mcp.WithNumber("down_ms", mcp.Description("Key hold time in ms, rounded up to 20 ms frames (default 100).")),
		mcp.WithNumber("up_ms", mcp.Description("Gap after release in ms, rounded up to 20 ms frames (default 200).")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		text, _ := req.RequireString("text")
		params := map[string]any{"name": name, "text": text}
		if v, ok := req.GetArguments()["down_ms"].(float64); ok {
			params["down_ms"] = int(v)
		}
		if v, ok := req.GetArguments()["up_ms"].(float64); ok {
			params["up_ms"] = int(v)
		}
		return forward(h, "type_text", params), nil
	})

	s.AddTool(mcp.NewTool("type_raw",
		mcp.WithDescription("Type an explicit key sequence. sequence is a JSON array of {row,col,shift,down_ms,up_ms}. Escape hatch when type_text maps wrong."),
		mcp.WithString("name", mcp.Description("Machine name."), mcp.Required()),
		mcp.WithString("sequence", mcp.Description("JSON array of {row,col,shift,down_ms,up_ms} objects."), mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		seqStr, _ := req.RequireString("sequence")
		var seq []any
		if err := json.Unmarshal([]byte(seqStr), &seq); err != nil {
			return mcp.NewToolResultError("sequence must be a JSON array of {row,col,shift,down_ms,up_ms}"), nil
		}
		return forward(h, "type_raw", map[string]any{"name": name, "sequence": seq}), nil
	})

	s.AddTool(mcp.NewTool("write_register",
		mcp.WithDescription("Write a CPU register by name (pc, sp, a, f, b, c, d, e, h, l, ix, iy, i, r, im)."),
		mcp.WithString("name", mcp.Description("Machine name."), mcp.Required()),
		mcp.WithString("register", mcp.Description("Register name (case-insensitive)."), mcp.Required()),
		mcp.WithNumber("value", mcp.Description("Value (0-65535)."), mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		reg, _ := req.RequireString("register")
		value := argInt(req, "value", 0)
		return forward(h, "write_register", map[string]any{"name": name, "register": reg, "value": value}), nil
	})

	s.AddTool(mcp.NewTool("disassemble",
		mcp.WithDescription("Disassemble count instructions at addr. Returns ADDR / BYTES / MNEMONIC lines."),
		mcp.WithString("name", mcp.Description("Machine name."), mcp.Required()),
		mcp.WithNumber("addr", mcp.Description("Start address (decimal)."), mcp.Required()),
		mcp.WithNumber("count", mcp.Description("Number of instructions (default 16, max 256).")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		addr := argInt(req, "addr", 0)
		count := argInt(req, "count", 16)
		return forwardText(h, "disassemble", map[string]any{"name": name, "addr": addr, "count": count}), nil
	})

	s.AddTool(mcp.NewTool("read_port_state",
		mcp.WithDescription("Consolidated peripheral state dump (border, FDC status/track/sector, joystick)."),
		mcp.WithString("name", mcp.Description("Machine name."), mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		return forward(h, "read_port_state", map[string]any{"name": name}), nil
	})

	s.AddTool(mcp.NewTool("set_trace",
		mcp.WithDescription("Turn the instruction delta trace on or off for a machine."),
		mcp.WithString("name", mcp.Description("Machine name."), mcp.Required()),
		mcp.WithBoolean("enabled", mcp.Description("Trace on (true) or off (false)."), mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		enabled := false
		if v, ok := req.GetArguments()["enabled"].(bool); ok {
			enabled = v
		}
		return forward(h, "set_trace", map[string]any{"name": name, "enabled": enabled}), nil
	})

	s.AddTool(mcp.NewTool("trace_get",
		mcp.WithDescription("Return up to n most-recent instruction trace records (PC + register/memory/port deltas)."),
		mcp.WithString("name", mcp.Description("Machine name."), mcp.Required()),
		mcp.WithNumber("n", mcp.Description("Number of records (default 100).")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		n := argInt(req, "n", 100)
		return forward(h, "trace_get", map[string]any{"name": name, "n": n}), nil
	})

	s.AddTool(mcp.NewTool("trace_diff",
		mcp.WithDescription("Compare the last n trace records of two machines and return the first divergence."),
		mcp.WithString("name_a", mcp.Description("First machine name."), mcp.Required()),
		mcp.WithString("name_b", mcp.Description("Second machine name."), mcp.Required()),
		mcp.WithNumber("n", mcp.Description("Number of records to compare (default 100).")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nameA, _ := req.RequireString("name_a")
		nameB, _ := req.RequireString("name_b")
		n := argInt(req, "n", 100)
		return forward(h, "trace_diff", map[string]any{"name_a": nameA, "name_b": nameB, "n": n}), nil
	})

	s.AddTool(mcp.NewTool("get_logs",
		mcp.WithDescription("Return the emulator's captured diagnostic logs (disk, ULA, interrupts)."),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return forwardText(h, "get_logs", nil), nil
	})

	s.AddTool(mcp.NewTool("clear_logs",
		mcp.WithDescription("Clear the captured diagnostic log buffer."),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return forward(h, "clear_logs", nil), nil
	})

	s.AddTool(mcp.NewTool("set_log_level",
		mcp.WithDescription("Set the diagnostic log level (debug, info, warn, error)."),
		mcp.WithString("level", mcp.Description("Log level."), mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		level, _ := req.RequireString("level")
		return forward(h, "set_log_level", map[string]any{"level": level}), nil
	})

	s.AddTool(mcp.NewTool("load_snapshot",
		mcp.WithDescription("Load a .sna or .z80 snapshot file into a machine. The path may be a .zip holding the snapshot."),
		mcp.WithString("name", mcp.Description("Machine name."), mcp.Required()),
		mcp.WithString("path", mcp.Description("Path to the snapshot file, or a .zip holding it."), mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		path, _ := req.RequireString("path")
		return forward(h, "load_snapshot", map[string]any{"name": name, "path": path}), nil
	})

	s.AddTool(mcp.NewTool("save_snapshot",
		mcp.WithDescription("Save a machine's state as an SNA snapshot file."),
		mcp.WithString("name", mcp.Description("Machine name."), mcp.Required()),
		mcp.WithString("path", mcp.Description("Path to write the .sna file."), mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		path, _ := req.RequireString("path")
		return forward(h, "save_snapshot", map[string]any{"name": name, "path": path}), nil
	})

	s.AddTool(mcp.NewTool("tape_control",
		mcp.WithDescription("Play, pause, or rewind a machine's tape (action: play/pause/reset)."),
		mcp.WithString("name", mcp.Description("Machine name."), mcp.Required()),
		mcp.WithString("action", mcp.Description("Action: play, pause, or reset."), mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		action, _ := req.RequireString("action")
		return forward(h, "tape_control", map[string]any{"name": name, "action": action}), nil
	})

	s.AddTool(mcp.NewTool("restart_worker",
		mcp.WithDescription("Rebuild the worker binary and restart the worker process, so code changes are picked up without restarting the supervisor."),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := h.restart(); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return textResult("worker restarted"), nil
	})

	log.Info("registered emulator tools", "count", 33)
}

// forward sends method+params to the worker and returns the result pretty-
// printed as JSON text.
func forward(h *workerHandle, method string, params map[string]any) *mcp.CallToolResult {
	agent := h.get()
	if agent == nil {
		return mcp.NewToolResultError("worker not available; build it first: go build -o zxgo-worker ./cmd/zxgo-worker")
	}
	raw, err := agent.Call(method, params)
	if err != nil {
		return mcp.NewToolResultError(err.Error())
	}
	var v any
	if err := json.Unmarshal(raw, &v); err == nil {
		if b, err := json.MarshalIndent(v, "", "  "); err == nil {
			return textResult("%s", b)
		}
	}
	return textResult("%s", raw)
}

// forwardText sends method+params to the worker and returns the string result
// as plain text, preserving newlines (for screen dumps and other text grids).
func forwardText(h *workerHandle, method string, params map[string]any) *mcp.CallToolResult {
	agent := h.get()
	if agent == nil {
		return mcp.NewToolResultError("worker not available; build it first: go build -o zxgo-worker ./cmd/zxgo-worker")
	}
	raw, err := agent.Call(method, params)
	if err != nil {
		return mcp.NewToolResultError(err.Error())
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return textResult("%s", s)
	}
	return textResult("%s", raw)
}

// argInt reads an integer argument (JSON numbers decode as float64).
func argInt(req mcp.CallToolRequest, key string, def int) int {
	v, ok := req.GetArguments()[key].(float64)
	if !ok {
		return def
	}
	return int(v)
}
