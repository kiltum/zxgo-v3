// Package emuagent manages the emulator worker child process. The MCP
// supervisor uses it to spawn cmd/zxgo-worker and issue JSON-RPC commands over
// the worker's stdio.
package emuagent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/kiltum/zxgo-v3/internal/workerproto"
)

// Agent is a running worker process with a request/response client.
type Agent struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	mu     sync.Mutex
	nextID int64
}

// Start spawns the worker binary at workerPath, rooted at workdir. The worker's
// stderr is left on our stderr for diagnostics; its stdout is reserved for the
// JSON-RPC stream.
func Start(workerPath, workdir string) (*Agent, error) {
	cmd := exec.Command(workerPath)
	cmd.Dir = workdir
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &Agent{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
	}, nil
}

// Call issues one JSON-RPC request and returns the raw JSON of the matching
// response's result. It is safe for concurrent use.
func (a *Agent) Call(method string, params any) (json.RawMessage, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.nextID++
	id := a.nextID

	req := workerproto.Request{JSONRPC: workerproto.Version, ID: id, Method: method}
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		req.Params = b
	}

	line, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if _, err := a.stdin.Write(append(line, '\n')); err != nil {
		return nil, fmt.Errorf("writing to worker: %w", err)
	}

	for {
		respLine, err := a.stdout.ReadBytes('\n')
		if err != nil {
			return nil, fmt.Errorf("worker closed stdout: %w", err)
		}
		var resp workerproto.Response
		if err := json.Unmarshal(respLine, &resp); err != nil {
			return nil, fmt.Errorf("bad worker response: %w", err)
		}
		if resp.ID != id {
			continue // out-of-order line; keep reading
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("%s", resp.Error.Message)
		}
		return json.Marshal(resp.Result)
	}
}

// Close shuts the worker down and waits for it to exit.
func (a *Agent) Close() error {
	if a.stdin != nil {
		_ = a.stdin.Close()
	}
	if a.cmd.Process != nil {
		_ = a.cmd.Process.Kill()
	}
	return a.cmd.Wait()
}
