package main

import (
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/kiltum/zxgo-v3/internal/emuagent"
)

// workerHandle is a swappable reference to the emulator worker process. The
// restart_worker tool closes the current worker, rebuilds its binary, and
// respawns it, so a code change can be picked up without restarting the
// supervisor by hand.
type workerHandle struct {
	mu      sync.Mutex
	agent   *emuagent.Agent
	worker  string
	workdir string
	log     *slog.Logger
}

func newWorkerHandle(worker, workdir string, log *slog.Logger) *workerHandle {
	return &workerHandle{worker: worker, workdir: workdir, log: log}
}

func (h *workerHandle) get() *emuagent.Agent {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.agent
}

// spawn starts the worker binary and stores the agent. Caller must hold h.mu.
func (h *workerHandle) spawn() error {
	p := h.worker
	if !filepath.IsAbs(p) {
		p = filepath.Join(h.workdir, p)
	}
	if _, err := exec.Command("test", "-x", p).Output(); err != nil {
		return fmt.Errorf("worker binary not found at %s", p)
	}
	agent, err := emuagent.Start(p, h.workdir)
	if err != nil {
		return err
	}
	h.agent = agent
	return nil
}

// restart rebuilds the worker binary, closes the old worker, and respawns it.
func (h *workerHandle) restart() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.agent != nil {
		_ = h.agent.Close()
		h.agent = nil
	}

	build := exec.Command("go", "build", "-o", "zxgo-worker", "./cmd/zxgo-worker")
	build.Dir = h.workdir
	if out, err := build.CombinedOutput(); err != nil {
		h.log.Error("worker rebuild failed", "err", err, "out", string(out))
		return fmt.Errorf("rebuild failed: %v", err)
	}

	if err := h.spawn(); err != nil {
		h.log.Error("worker restart failed", "err", err)
		return err
	}
	h.log.Info("worker restarted", "worker", h.worker)
	return nil
}
