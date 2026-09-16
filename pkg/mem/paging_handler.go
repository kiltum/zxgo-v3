package mem

import "github.com/kiltum/zxgo-v3/pkg/io_ports"

// PagingPortHandler wraps a MemoryMapper to handle paging port writes.
// This allows the mapper to be registered with the PortBus without modifying
// the MemoryMapper interface.
type PagingPortHandler struct {
	mapper MemoryMapper
}

// NewPagingPortHandler creates a PortHandler that routes paging writes to the mapper.
func NewPagingPortHandler(mapper MemoryMapper) io_ports.PortHandler {
	return &PagingPortHandler{mapper: mapper}
}

func (h *PagingPortHandler) HandlesPort(port uint16) bool {
	// 128K: 0x7FFD (bit 14 set, bit 0 set)
	// +2A/+3: also 0x1FFD
	masked := port & 0xC002
	return masked == 0x4000 && (port&1) == 1 // 0x7FFD, 0x1FFD etc
}

func (h *PagingPortHandler) Read(port uint16) uint8 {
	return 0xFF
}

func (h *PagingPortHandler) Write(port uint16, value uint8) {
	h.mapper.WritePort(port, value)
}
