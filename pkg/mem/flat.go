package mem

import (
	"fmt"

	"github.com/kiltum/zxgo-v3/pkg/model"
)

// flatMapper is a flat 64KB RAM mapper for bare-metal test programs (ZEXALL,
// the Fuse instruction tests). It has no ROM/RAM banking, no paging and no
// contention -- just a contiguous 64KB of memory, which is what a CP/M-style
// test program expects. Direct array access (no scheme dispatch, no ROM
// activation loop, no string compare) keeps the CPU test fast.
type flatMapper struct {
	cfg      model.Config
	mem      [65536]byte
	romWrite bool
}

// newFlat48K builds a flat 64KB mapper.
func newFlat48K(cfg model.Config) *flatMapper {
	return &flatMapper{cfg: cfg, romWrite: true}
}

func (f *flatMapper) ReadByte(addr uint16) uint8     { return f.mem[addr] }
func (f *flatMapper) FetchOpcode(addr uint16) uint8  { return f.mem[addr] }
func (f *flatMapper) ULAReadByte(addr uint16) uint8  { return f.mem[addr] }
func (f *flatMapper) WriteByte(addr uint16, v uint8) { f.mem[addr] = v }

func (f *flatMapper) WritePort(uint16, uint8) {}
func (f *flatMapper) Model() model.Config     { return f.cfg }
func (f *flatMapper) Reset()                  { f.mem = [65536]byte{} }
func (f *flatMapper) LoadROM(int, []byte) error {
	return nil
}
func (f *flatMapper) SetROMWritable(w bool) { f.romWrite = w }
func (f *flatMapper) IsContended(uint16) bool {
	return false
}
func (f *flatMapper) GetBank(int) []byte  { return nil }
func (f *flatMapper) SetBank(int, []byte) {}

func (f *flatMapper) Snapshot() [][]byte {
	banks := make([][]byte, 4)
	for i := range banks {
		banks[i] = make([]byte, 16384)
		copy(banks[i], f.mem[i*16384:(i+1)*16384])
	}
	return banks
}

func (f *flatMapper) RestoreSnapshot(banks [][]byte) {
	for i := range banks {
		if i < 4 && banks[i] != nil {
			copy(f.mem[i*16384:(i+1)*16384], banks[i])
		}
	}
}

// NumRAMBanks returns 4: the flat 64K is exposed as four 16K banks so the
// snapshot path has something to iterate.
func (f *flatMapper) NumRAMBanks() int { return 4 }

// PagingState is empty: this mapper has no banking and no paging port at all.
// It exists for the bare-metal CPU tests, which never see a state file.
func (f *flatMapper) PagingState() PagingState { return PagingState{} }

// RestorePagingState accepts only the empty state, so a real machine's paging
// state cannot be applied to a mapper that has nowhere to put it.
func (f *flatMapper) RestorePagingState(s PagingState) error {
	if s.Scheme != "" || len(s.Blob) != 0 || s.Write7FFD != 0 {
		return fmt.Errorf("flat mapper cannot restore paging scheme %q", s.Scheme)
	}
	return nil
}

var _ MemoryMapper = (*flatMapper)(nil)
