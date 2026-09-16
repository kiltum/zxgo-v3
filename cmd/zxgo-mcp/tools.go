package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	defaultExecTimeout = 120 * time.Second
	maxExecTimeout     = 600 * time.Second
)

// registerCodeSyncTools wires the build/sync/file tools. These do not touch
// the emulator; they exist so the build path and MCP transport can be
// validated before any emulator work is added.
func registerCodeSyncTools(s *server.MCPServer, workdir string, log *slog.Logger) {
	s.AddTool(mcp.NewTool("ping",
		mcp.WithDescription("Sanity check: returns hostname, working directory, and go version."),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		host, _ := os.Hostname()
		goVer := "unknown"
		if out, err := exec.Command("go", "version").Output(); err == nil {
			goVer = strings.TrimSpace(string(out))
		}
		return textResult("pong\nhost=%s\nworkdir=%s\ngo=%s\ngoos=%s goarch=%s",
			host, workdir, goVer, runtime.GOOS, runtime.GOARCH), nil
	})

	s.AddTool(mcp.NewTool("exec",
		mcp.WithDescription("Run a shell command in the working directory and return stdout, stderr, and exit code."),
		mcp.WithString("command", mcp.Description("The shell command to run."), mcp.Required()),
		mcp.WithString("cwd", mcp.Description("Working directory (defaults to the server workdir).")),
		mcp.WithNumber("timeout_ms", mcp.Description("Timeout in milliseconds (default 120000, max 600000).")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		command, err := req.RequireString("command")
		if err != nil {
			return mcp.NewToolResultError("missing required argument 'command'"), nil
		}

		cwd := req.GetString("cwd", workdir)
		timeout := defaultExecTimeout
		if v, ok := req.GetArguments()["timeout_ms"].(float64); ok && v > 0 {
			timeout = time.Duration(v) * time.Millisecond
			if timeout > maxExecTimeout {
				timeout = maxExecTimeout
			}
		}

		out, exitCode, err := runCommand(ctx, cwd, timeout, command)
		if err != nil {
			// Context deadline or failure to start the shell.
			return textResult("%sexit code: %d", out, exitCode), nil
		}
		return textResult("%sexit code: %d", out, exitCode), nil
	})

	s.AddTool(mcp.NewTool("read_file",
		mcp.WithDescription("Read a file and return its contents. Relative paths resolve against the server workdir."),
		mcp.WithString("path", mcp.Description("Path to the file."), mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("path")
		if err != nil {
			return mcp.NewToolResultError("missing required argument 'path'"), nil
		}
		full := resolvePath(workdir, path)
		data, err := os.ReadFile(full)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("read_file %s: %v", full, err)), nil
		}
		return textResult("%s", string(data)), nil
	})

	s.AddTool(mcp.NewTool("write_file",
		mcp.WithDescription("Write a file. Relative paths resolve against the server workdir. Creates parent directories as needed."),
		mcp.WithString("path", mcp.Description("Path to the file."), mcp.Required()),
		mcp.WithString("contents", mcp.Description("File contents."), mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("path")
		if err != nil {
			return mcp.NewToolResultError("missing required argument 'path'"), nil
		}
		contents, err := req.RequireString("contents")
		if err != nil {
			return mcp.NewToolResultError("missing required argument 'contents'"), nil
		}
		full := resolvePath(workdir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("write_file %s: %v", full, err)), nil
		}
		if err := os.WriteFile(full, []byte(contents), 0o644); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("write_file %s: %v", full, err)), nil
		}
		return textResult("wrote %s (%d bytes)", full, len(contents)), nil
	})

	s.AddTool(mcp.NewTool("edit_file",
		mcp.WithDescription("Replace a string in a file (exact match). Use for targeted edits instead of sed."),
		mcp.WithString("path", mcp.Description("Path to the file."), mcp.Required()),
		mcp.WithString("old_string", mcp.Description("Exact text to replace."), mcp.Required()),
		mcp.WithString("new_string", mcp.Description("Replacement text."), mcp.Required()),
		mcp.WithBoolean("replace_all", mcp.Description("Replace every occurrence (default: replace the single unique occurrence).")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("path")
		if err != nil {
			return mcp.NewToolResultError("missing required argument 'path'"), nil
		}
		oldStr, err := req.RequireString("old_string")
		if err != nil {
			return mcp.NewToolResultError("missing required argument 'old_string'"), nil
		}
		newStr, err := req.RequireString("new_string")
		if err != nil {
			return mcp.NewToolResultError("missing required argument 'new_string'"), nil
		}
		replaceAll := false
		if v, ok := req.GetArguments()["replace_all"].(bool); ok {
			replaceAll = v
		}

		full := resolvePath(workdir, path)
		data, err := os.ReadFile(full)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("edit_file %s: %v", full, err)), nil
		}
		content := string(data)
		count := strings.Count(content, oldStr)
		if count == 0 {
			return mcp.NewToolResultError(fmt.Sprintf("edit_file %s: old_string not found", full)), nil
		}
		if !replaceAll && count > 1 {
			return mcp.NewToolResultError(fmt.Sprintf("edit_file %s: old_string occurs %d times; make it unique or set replace_all", full, count)), nil
		}
		if replaceAll {
			content = strings.ReplaceAll(content, oldStr, newStr)
		} else {
			content = strings.Replace(content, oldStr, newStr, 1)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("edit_file %s: %v", full, err)), nil
		}
		replaced := 1
		if replaceAll {
			replaced = count
		}
		return textResult("edited %s (%d replacement(s))", full, replaced), nil
	})

	s.AddTool(mcp.NewTool("rebuild",
		mcp.WithDescription("Build the project with 'go build'. Reports output and exit code."),
		mcp.WithString("target", mcp.Description("Build target (default './...').")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		target := req.GetString("target", "./...")
		cmd := fmt.Sprintf("go build %s", target)
		out, exitCode, _ := runCommand(ctx, workdir, defaultExecTimeout, cmd)
		return textResult("%sexit code: %d", out, exitCode), nil
	})

	s.AddTool(mcp.NewTool("test",
		mcp.WithDescription("Run 'go test' and report output and exit code."),
		mcp.WithString("packages", mcp.Description("Packages to test (default './...').")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		pkgs := req.GetString("packages", "./...")
		cmd := fmt.Sprintf("go test %s", pkgs)
		out, exitCode, _ := runCommand(ctx, workdir, defaultExecTimeout, cmd)
		return textResult("%sexit code: %d", out, exitCode), nil
	})

	log.Info("registered code-sync tools",
		"tools", "ping exec read_file write_file edit_file rebuild test",
		"workdir", workdir)
}

// runCommand runs a shell command and returns a combined stdout/stderr report
// plus the process exit code. A timeout or start failure is reflected in the
// returned report, not an error.
func runCommand(ctx context.Context, cwd string, timeout time.Duration, command string) (string, int, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "/bin/sh", "-c", command)
	cmd.Dir = cwd

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			var b strings.Builder
			b.WriteString(fmt.Sprintf("$ %s\n", command))
			b.WriteString("[timeout after " + timeout.String() + "]\n")
			return b.String(), -1, nil
		}
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			exitCode = -1
		}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("$ %s\n", command))
	if stdout.Len() > 0 {
		b.WriteString("--- stdout ---\n")
		b.WriteString(stdout.String())
	}
	if stderr.Len() > 0 {
		b.WriteString("--- stderr ---\n")
		b.WriteString(stderr.String())
	}
	return b.String(), exitCode, nil
}

// resolvePath returns an absolute path for p, rooted at workdir when p is
// relative.
func resolvePath(workdir, p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(workdir, p)
}

// textResult builds a plain-text tool result.
func textResult(format string, args ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: fmt.Sprintf(format, args...),
			},
		},
	}
}
