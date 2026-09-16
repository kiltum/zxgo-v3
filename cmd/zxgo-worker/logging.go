package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/kiltum/zxgo-v3/internal/emulator"
	"github.com/kiltum/zxgo-v3/pkg/cpu"
	"github.com/kiltum/zxgo-v3/pkg/logger"
	"github.com/kiltum/zxgo-v3/pkg/media"
	"github.com/kiltum/zxgo-v3/pkg/ula"
)

const logBufMax = 1 << 20 // 1 MB

var (
	logBuf   = newRingBuffer(logBufMax)
	logLevel = new(slog.LevelVar)
)

// setupLogging wires the emulator's subsystem loggers to the shared ring buffer.
// It is called once at worker startup.
func setupLogging() {
	logLevel.Set(slog.LevelDebug)
	l := logger.New(logBuf, logLevel)
	media.SetDiskLogger(l.WithGroup("disk"), l.WithGroup("disk-ctrl"))
	ula.SetULALogger(l.WithGroup("ula"))
	emulator.SetInterruptLogger(l.WithGroup("int"))
	cpu.SetInterruptLogger(l.WithGroup("int"))
	media.SetUPD765Logger(l.WithGroup("upd765"))
}

func (w *worker) getLogs(raw json.RawMessage) (any, error) {
	return logBuf.String(), nil
}

func (w *worker) clearLogs(raw json.RawMessage) (any, error) {
	logBuf.Clear()
	return "ok", nil
}

func (w *worker) setLogLevel(raw json.RawMessage) (any, error) {
	var p struct {
		Level string `json:"level"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	var lv slog.Level
	switch p.Level {
	case "debug":
		lv = slog.LevelDebug
	case "info":
		lv = slog.LevelInfo
	case "warn":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		return nil, fmt.Errorf("unknown level %q", p.Level)
	}
	logLevel.Set(lv)
	return "ok", nil
}

// ringBuffer is a thread-safe byte buffer with a size cap; oldest lines are
// dropped once the cap is exceeded.
type ringBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func newRingBuffer(max int) *ringBuffer {
	return &ringBuffer{max: max}
}

func (r *ringBuffer) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = append(r.buf, p...)
	if len(r.buf) > r.max {
		excess := len(r.buf) - r.max
		if idx := bytes.IndexByte(r.buf[excess:], '\n'); idx >= 0 {
			r.buf = r.buf[excess+idx+1:]
		} else {
			r.buf = r.buf[excess:]
		}
	}
	return len(p), nil
}

func (r *ringBuffer) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return string(r.buf)
}

func (r *ringBuffer) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = r.buf[:0]
}
