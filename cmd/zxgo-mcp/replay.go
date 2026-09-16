package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerReplayTool wires the replay engine: a JSON repro script (ordered
// worker steps) executed against a fresh worker, returning each step's result.
func registerReplayTool(s *server.MCPServer, h *workerHandle, log *slog.Logger) {
	s.AddTool(mcp.NewTool("replay",
		mcp.WithDescription("Execute a JSON repro script: {\"steps\":[{\"method\":\"...\",\"params\":{...}}, ...]} against the worker and return each step's result."),
		mcp.WithString("script", mcp.Description("JSON script (see replay schema)."), mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		script, _ := req.RequireString("script")
		var s struct {
			Steps []struct {
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			} `json:"steps"`
		}
		if err := json.Unmarshal([]byte(script), &s); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("bad script: %v", err)), nil
		}
		agent := h.get()
		if agent == nil {
			return mcp.NewToolResultError("worker not available; build it first: go build -o zxgo-worker ./cmd/zxgo-worker"), nil
		}

		var out []string
		for i, st := range s.Steps {
			raw, err := agent.Call(st.Method, st.Params)
			if err != nil {
				out = append(out, fmt.Sprintf("[%d] %s: ERROR %v", i, st.Method, err))
				continue
			}
			out = append(out, fmt.Sprintf("[%d] %s: %s", i, st.Method, compact(raw)))
		}
		return textResult("%s", strings.Join(out, "\n")), nil
	})
	log.Info("registered replay tool")
}

// compact renders a raw JSON result as one line (newlines collapsed) for the
// replay transcript.
func compact(raw []byte) string {
	s := strings.ReplaceAll(string(raw), "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}
