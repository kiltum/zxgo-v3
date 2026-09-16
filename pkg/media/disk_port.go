package media

import (
	"log/slog"

	"github.com/kiltum/zxgo-v3/pkg/io_ports"
)

// diskLog is the logger for disk subsystem debug output.
// nil means the caller has not wired a logger (disk operations are silent).
var diskLog *slog.Logger

// diskCtrlLog is the logger for per-access port trace (very high volume).
// Typically this is the same object as diskLog, but a caller that wants only
// command-level logging can pass a different (or nil) logger.
var diskCtrlLog *slog.Logger

// SetDiskLogger stores the logger used by the disk subsystem and controller.
// Both may be the same *slog.Logger; pass nil for either to suppress that output.
func SetDiskLogger(dlog, ctllog *slog.Logger) {
	diskLog = dlog
	diskCtrlLog = ctllog
}

// SetTRDosActive records whether the TR-DOS ROM is currently paged in.

// Nil-safe logging helpers for disk operations (used when loggers may be nil during tests)
func diskLogWarn(msg string, args ...any) {
	if diskLog != nil {
		diskLog.Warn(msg, args...)
	}
}

func diskLogInfo(msg string, args ...any) {
	if diskLog != nil {
		diskLog.Info(msg, args...)
	}
}

func diskCtrlLogDebug(msg string, args ...any) {
	if diskCtrlLog != nil {
		diskCtrlLog.Debug(msg, args...)
	}
}

// regName returns a human-readable name for a Beta Disk port.
func regName(p byte, isWrite bool) string {
	if (p & 0x80) != 0 {
		return "SYSTEM"
	}
	switch p {
	case 0x1F:
		if isWrite {
			return "CMD"
		}
		return "STATUS"
	case 0x3F:
		return "TRACK"
	case 0x5F:
		return "SECTOR"
	case 0x7F:
		return "DATA"
	default:
		return "???"
	}
}

// HandlesPort implements io_ports.PortHandler for the Beta Disk controller.
func (c *BetaDiskController) HandlesPort(port uint16) bool {
	p := byte(port)
	if (p & 0x1F) != 0x1F {
		return false
	}
	if (p & 0x80) != 0 {
		return true // SYSTEM register: 0x9F, 0xBF, 0xDF, 0xFF
	}
	return p == 0x1F || p == 0x3F || p == 0x5F || p == 0x7F
}

// Read handles port reads for Beta Disk controller.
func (c *BetaDiskController) Read(port uint16) uint8 {
	data := c.ReadPort(port)
	if diskCtrlLog != nil {
		diskCtrlLog.Debug("beta disk read", "port", port, "reg", regName(byte(port), false), "val", data)
	}
	return data
}

// ReadAttached implements io_ports.AttachedHandler. The four WD1793 registers
// drive all eight bits (ReadPort leaves them high where the chip does not drive
// anyway, so a plain 0xFF mask is correct). The system register is different:
// only INTRQ and DRQ are driven, so its mask is 0xC0 and the other six bits take
// the floating bus - which is what a machine with no disk fitted sees on port
// 0xFF instead of the 0x3F ReadPort pads with (Fuse ref/beta.c does the same).
func (c *BetaDiskController) ReadAttached(port uint16) (uint8, uint8) {
	if (byte(port) & 0x80) != 0 {
		return c.ReadPort(port), 0xC0
	}
	return c.ReadPort(port), 0xFF
}

// Write handles port writes for Beta Disk controller.
func (c *BetaDiskController) Write(port uint16, value uint8) {
	if diskCtrlLog != nil {
		diskCtrlLog.Debug("beta disk write", "port", port, "reg", regName(byte(port), true), "val", value)
	}
	c.WritePort(port, value)
}

var _ io_ports.PortHandler = (*BetaDiskController)(nil)
