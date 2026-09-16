// Command zxgo-mcp is the MCP supervisor for zxgo-v3.
//
// It exposes the emulator and its build toolchain over the Model Context
// Protocol, so a Claude Code session on a master machine can drive a worker
// machine: edit code, sync it, rebuild it, run the emulator headlessly, and
// inspect CPU / memory / port / screen state, all through tool calls.
//
// The supervisor serves MCP over Streamable HTTP (see MCP_SERVER.md). The
// emulator worker (cmd/zxgo-worker) is a separate, rebuildable subprocess that
// this supervisor spawns and talks to over stdio JSON-RPC.
//
// Every tool call and its result is logged to stdout, because the supervisor
// runs on a dedicated worker machine where that noise is harmless and it is the
// only way to see what is happening.
//
// Default bind is loopback only. For the two-machine deployment pass
// -addr 0.0.0.0:8765 (or the worker's LAN address).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	name    = "zxgo-mcp"
	version = "0.1.0"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8765", "listen address for the MCP HTTP server")
	workdir := flag.String("workdir", "", "working directory for exec/build/test tools (default: process cwd)")
	worker := flag.String("worker", "./zxgo-worker", "path to the emulator worker binary (relative to workdir)")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	wd := *workdir
	if wd == "" {
		var err error
		if wd, err = os.Getwd(); err != nil {
			log.Error("cannot determine working directory", "err", err)
			os.Exit(1)
		}
	}
	if fi, err := os.Stat(wd); err != nil || !fi.IsDir() {
		log.Error("workdir is not a directory", "workdir", wd, "err", err)
		os.Exit(1)
	}

	s := server.NewMCPServer(
		name,
		version,
		server.WithToolCapabilities(true),
		server.WithHooks(newLoggingHooks(log)),
	)

	registerCodeSyncTools(s, wd, log)

	handle := newWorkerHandle(*worker, wd, log)
	if err := handle.spawn(); err != nil {
		log.Warn("worker failed to start; emulator tools disabled", "path", *worker, "err", err)
	} else {
		log.Info("emulator worker started", "path", *worker)
	}
	registerEmulatorTools(s, handle, log)
	registerReplayTool(s, handle, log)

	httpServer := server.NewStreamableHTTPServer(s)

	log.Info("zxgo-mcp listening", "addr", *addr, "workdir", wd, "endpoint", "/mcp")
	if err := httpServer.Start(*addr); err != nil {
		log.Error("server error", "err", err)
		os.Exit(1)
	}
}

// newLoggingHooks wires a hook pair that logs every tool call and its result to
// stdout via the supplied logger. This is the supervisor's activity log.
func newLoggingHooks(log *slog.Logger) *server.Hooks {
	h := &server.Hooks{}
	h.AddBeforeCallTool(func(ctx context.Context, id any, msg *mcp.CallToolRequest) {
		args, _ := json.Marshal(msg.Params.Arguments)
		log.Info("mcp_call", "tool", msg.Params.Name, "args", string(args))
	})
	h.AddAfterCallTool(func(ctx context.Context, id any, msg *mcp.CallToolRequest, result any) {
		log.Info("mcp_result", "tool", msg.Params.Name, "result", summarizeResult(result))
	})
	return h
}

// summarizeResult renders a tool result into one short, single-line string for
// the activity log.
func summarizeResult(result any) string {
	cr, ok := result.(*mcp.CallToolResult)
	if !ok || cr == nil {
		return truncate(fmt.Sprintf("%v", result), 400)
	}
	s := summarizeContent(cr.Content)
	if cr.IsError {
		s = "ERROR: " + s
	}
	return s
}

// summarizeContent extracts the text parts of a tool result's content list.
func summarizeContent(content []mcp.Content) string {
	var parts []string
	for _, c := range content {
		if tc, ok := c.(mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return truncate(strings.Join(parts, "\n"), 400)
}

// truncate limits s to n runes, appending an ellipsis when it was cut.
func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
