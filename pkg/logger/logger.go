// Package logger provides log/slog-based structured logging with shared
// emulator tick context. The emulator main loop calls SetTick before each
// instruction; every log line the handler emitted after that carries tick=
// and frame= automatically.
//
// Usage from packages that want to log:
//
//	var log *slog.Logger
//	func SetLogger(l *slog.Logger) { log = l }
//
//	func doThing() {
//	    if log != nil { log.Debug("thing", "val", 42) }
//	}
//
// Usage from cmd/zxgo:
//
//	l := logger.New(os.Stderr, slog.LevelInfo)
//	media.SetLogger(l.With("subsys", "disk"))
//	io_ports.SetLogger(l.With("subsys", "port"))
package logger

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
)

var currentTick, currentFrame int64

// SetTick updates the shared clock. Called by the emulator main loop before
// each instruction. Safe to call from a single goroutine.
func SetTick(tick, frame int64) {
	atomic.StoreInt64(&currentTick, tick)
	atomic.StoreInt64(&currentFrame, frame)
}

// New creates a root *slog.Logger that writes text to w with the given
// default level. The handler adds tick= and frame= to every record.
func New(w io.Writer, level slog.Leveler) *slog.Logger {
	h := &tickHandler{inner: slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})}
	return slog.New(h)
}

// tickHandler adds tick= and frame= from the shared clock to every record
// before delegating to the inner handler.
type tickHandler struct {
	inner slog.Handler
}

func (h *tickHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *tickHandler) Handle(ctx context.Context, r slog.Record) error {
	r.AddAttrs(
		slog.Int64("tick", atomic.LoadInt64(&currentTick)),
		slog.Int64("frame", atomic.LoadInt64(&currentFrame)),
	)
	return h.inner.Handle(ctx, r)
}

func (h *tickHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &tickHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *tickHandler) WithGroup(name string) slog.Handler {
	return &tickHandler{inner: h.inner.WithGroup(name)}
}
