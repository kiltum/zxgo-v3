package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kiltum/zxgo-v3/pkg/disasm"
)

// disassemble decodes up to count instructions starting at addr, using the
// pkg/disasm disassembler. Returns one "ADDR  BYTES  MNEMONIC" line per
// instruction.
func (w *worker) disassemble(raw json.RawMessage) (any, error) {
	var p struct {
		Name  string `json:"name"`
		Addr  int    `json:"addr"`
		Count int    `json:"count"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e, err := w.getMachine(p.Name)
	if err != nil {
		return nil, err
	}
	if p.Count <= 0 || p.Count > 256 {
		p.Count = 16
	}

	m := e.Mapper()
	d := disasm.New()
	var sb strings.Builder
	addr := uint16(p.Addr)
	for i := 0; i < p.Count; i++ {
		data := make([]byte, 4)
		for j := 0; j < 4; j++ {
			data[j] = m.ReadByte(addr + uint16(j))
		}
		ins, err := d.Decode(data)
		if err != nil {
			break
		}
		sb.WriteString(fmt.Sprintf("%04X  %-12s %s\n", addr, hexBytes(data[:ins.Length]), ins.Mnemonic))
		addr += uint16(ins.Length)
	}
	return sb.String(), nil
}

// hexBytes renders bytes as space-separated uppercase hex.
func hexBytes(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		fmt.Fprintf(&sb, "%02X ", c)
	}
	return strings.TrimSpace(sb.String())
}
