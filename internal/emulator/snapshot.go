package emulator

import (
	"fmt"
	"github.com/kiltum/zxgo-v3/pkg/mem"

	"github.com/kiltum/zxgo-v3/pkg/snap"
)

// LoadSNA applies a parsed SNA snapshot to the emulator.
func (e *Emulator) LoadSNA(s *snap.Snapshot) error {
	if err := e.checkSnapshotClass(s.Is128K, "SNA snapshot"); err != nil {
		return err
	}
	if !s.ValidFor48K() && !s.ValidFor128K() {
		return fmt.Errorf("invalid snapshot: incompatible RAM size")
	}
	cpu := e.CPU()

	cpu.A = s.Header.A
	cpu.F = s.Header.F
	cpu.B, cpu.C = byte(s.Header.BC>>8), byte(s.Header.BC&0xFF)
	cpu.D, cpu.E = byte(s.Header.DE>>8), byte(s.Header.DE&0xFF)
	cpu.H, cpu.L = byte(s.Header.HL>>8), byte(s.Header.HL&0xFF)

	cpu.A_, cpu.F_ = byte(s.Header.AF_>>8), byte(s.Header.AF_&0xFF)
	cpu.B_, cpu.C_ = byte(s.Header.BC_>>8), byte(s.Header.BC_&0xFF)
	cpu.D_, cpu.E_ = byte(s.Header.DE_>>8), byte(s.Header.DE_&0xFF)
	cpu.H_, cpu.L_ = byte(s.Header.HL_>>8), byte(s.Header.HL_&0xFF)

	cpu.IX = s.Header.IX
	cpu.IY = s.Header.IY
	cpu.I = s.Header.I
	cpu.R = s.Header.R
	cpu.IM = s.Header.IM
	cpu.SP = s.Header.SP

	cpu.IFF1 = (s.Header.IFF2 & 0x04) != 0
	cpu.IFF2 = cpu.IFF1

	mapper := e.Mapper()
	if len(s.RAM) != 49152 {
		return fmt.Errorf("invalid snapshot RAM size: expected 49152, got %d", len(s.RAM))
	}
	// Paging first: the 48K window below belongs to whichever banks were mapped
	// when the snapshot was taken, so the machine has to be in that state before
	// the bytes land.
	if s.Is128K {
		if w, ok := mapper.(interface{ WritePort(uint16, uint8) }); ok {
			w.WritePort(0x7FFD, s.P7FFD)
		}
	}
	for i := 0; i < 49152; i++ {
		mapper.WriteByte(uint16(0x4000+i), s.RAM[i])
	}
	// Then the banks the snapshot carried separately, in the same ascending order
	// CreateSNA used, skipping the three the 48K window already covered.
	if s.Is128K && len(s.RAMBanks) == 5 && e.hasBankedRAM() {
		paged := int(s.P7FFD & 0x07)
		i := 0
		for b := 0; b < bankCount && i < len(s.RAMBanks); b++ {
			if b == 5 || b == 2 || b == paged {
				continue
			}
			mapper.SetBank(b, s.RAMBanks[i])
			i++
		}
	}

	e.ULA().SetBorderColor(int(s.Header.Border))

	// The SNA format stores the PC on the stack: SP points at it. Pop it.
	pc, err := s.GetRegisterValue("PC")
	if err == nil {
		cpu.PC = pc
		cpu.SP += 2
	} else {
		cpu.PC = 0x8000
	}
	return nil
}

// LoadZ80 applies a parsed Z80 snapshot to the emulator.
func (e *Emulator) LoadZ80(s *snap.Z80Snapshot) error {
	if err := e.checkSnapshotClass(s.Is128K, "Z80 snapshot"); err != nil {
		return err
	}
	cpu := e.CPU()

	cpu.A = byte(s.Header.AF >> 8)
	cpu.F = byte(s.Header.AF & 0xFF)
	cpu.B, cpu.C = byte(s.Header.BC>>8), byte(s.Header.BC&0xFF)
	cpu.D, cpu.E = byte(s.Header.DE>>8), byte(s.Header.DE&0xFF)
	cpu.H, cpu.L = byte(s.Header.HL>>8), byte(s.Header.HL&0xFF)

	cpu.A_, cpu.F_ = byte(s.Header.AF_>>8), byte(s.Header.AF_&0xFF)
	cpu.H_, cpu.L_ = byte(s.Header.HL_>>8), byte(s.Header.HL_&0xFF)
	cpu.D_, cpu.E_ = byte(s.Header.DE_>>8), byte(s.Header.DE_&0xFF)
	cpu.B_, cpu.C_ = byte(s.Header.BC_>>8), byte(s.Header.BC_&0xFF)

	cpu.IX = s.Header.IX
	cpu.IY = s.Header.IY
	cpu.I = s.Header.I
	cpu.R = s.Header.R
	cpu.SP = s.Header.SP
	cpu.PC = s.Header.PC

	cpu.IFF1 = (s.Header.IFF & 0x04) != 0
	cpu.IFF2 = (s.Header.Flags & 0x04) != 0
	cpu.IM = s.Header.Flags & 0x03

	mapper := e.Mapper()
	if s.Is128K {
		expectedSize := 8 * 16384 // 8 pages * 16KB = 128KB
		if len(s.RAM) != expectedSize {
			return fmt.Errorf("incomplete 128K Z80 snapshot: got %d bytes RAM, want %d", len(s.RAM), expectedSize)
		}
		for bank := 0; bank < 8; bank++ {
			start := bank * 16384
			mapper.SetBank(bank, s.RAM[start:start+16384])
		}
	} else {
		if len(s.RAM) == 49152 {
			for i := 0; i < len(s.RAM); i++ {
				mapper.WriteByte(uint16(0x4000+i), s.RAM[i])
			}
		} else if len(s.RAM) > 0 {
			return fmt.Errorf("incomplete or compressed Z80 snapshot: got %d bytes RAM, want 49152", len(s.RAM))
		}
	}
	return nil
}

// CreateSNA builds a 48K SNA snapshot from the current emulator state. The SNA
// format stores the PC on the stack: SP is decremented by 2 and the PC is
// written at that address, so LoadSNA can pop it back.
func (e *Emulator) CreateSNA() (*snap.Snapshot, error) {
	cpu := e.CPU()
	mapper := e.Mapper()

	ram := make([]byte, 49152)
	for i := 0; i < 49152; i++ {
		ram[i] = mapper.ReadByte(uint16(0x4000 + i))
	}

	// Push PC onto the stack (SP-2) and store it in RAM.
	sp := cpu.SP - 2
	if sp >= 0x4000 && sp <= 0xFFFD {
		off := int(sp - 0x4000)
		ram[off] = byte(cpu.PC & 0xFF)
		ram[off+1] = byte(cpu.PC >> 8)
	}

	header := snap.SNAHeader{
		A:   cpu.A,
		F:   cpu.F,
		BC:  uint16(cpu.B)<<8 | uint16(cpu.C),
		DE:  uint16(cpu.D)<<8 | uint16(cpu.E),
		HL:  uint16(cpu.H)<<8 | uint16(cpu.L),
		AF_: uint16(cpu.A_)<<8 | uint16(cpu.F_),
		BC_: uint16(cpu.B_)<<8 | uint16(cpu.C_),
		DE_: uint16(cpu.D_)<<8 | uint16(cpu.E_),
		HL_: uint16(cpu.H_)<<8 | uint16(cpu.L_),
		IY:  cpu.IY,
		IX:  cpu.IX,
		I:   cpu.I,
		R:   cpu.R,
		SP:  sp,
		IM:  cpu.IM,
	}
	if cpu.IFF1 {
		header.IFF2 = 0x04
	}
	header.Border = uint8(e.ULA().BorderColor())

	snapOut := &snap.Snapshot{Header: header, RAM: ram}

	// A machine with RAM banks has more to save than the 48K window holds. The
	// 48K part above already covers banks 5 (0x4000) and 2 (0x8000), which are
	// always mapped, plus whatever sits at 0xC000; the SNA format stores the
	// other five after it. They are written in ascending bank order, which is
	// this emulator's convention - the file records no bank numbers, so the
	// loader has to derive the same list from the same paging value.
	if e.hasBankedRAM() {
		paged := int(pagingValue(mapper) & 0x07)
		for b := 0; b < bankCount; b++ {
			if b == 5 || b == 2 || b == paged {
				continue
			}
			snapOut.RAMBanks = append(snapOut.RAMBanks, mapper.GetBank(b))
		}
		// The window shows three *distinct* banks only when the paged bank is
		// neither 2 nor 5. Page bank 5 to 0xC000 and the window holds it twice,
		// leaving six banks for five slots - the format cannot express that, so
		// the highest-numbered one is dropped. Every writer of this format has
		// the same corner, and a save that is lossy in one paging state beats
		// one that refuses.
		if len(snapOut.RAMBanks) > 5 {
			snapOut.RAMBanks = snapOut.RAMBanks[:5]
		}
		if len(snapOut.RAMBanks) == 5 {
			snapOut.Is128K = true
			snapOut.P7FFD = pagingValue(mapper)
		}
	}

	return snapOut, nil
}

// bankCount is how many RAM banks a banked machine has. Eight is the 128K and
// the +2A/+3; the Pentagon has more, but its extra banks are not part of the
// 128K SNA layout and are dropped rather than written in a shape no loader
// would read back.
const bankCount = 8

// checkSnapshotClass refuses a snapshot that describes a banked machine when the
// emulator is not one. The banks and the paging register have nowhere to go, so
// applying only the 48K window would produce a machine that boots to the wrong
// thing and looks like it worked - the one failure mode this check exists to
// avoid. The other direction is deliberately allowed: a 48K program runs on a
// 128K, and refusing it would break the common case.
//
// The remaining mismatches - a +3 snapshot on a 128K, a Pentagon one on a +2A -
// are one class and load; they want a warning rather than a refusal, and a
// warning needs a logger in this package, which is not wired yet.
func (e *Emulator) checkSnapshotClass(is128K bool, what string) error {
	if is128K && !e.hasBankedRAM() {
		return fmt.Errorf("%s is for a banked machine, and this is %s", what, e.cfg.Name)
	}
	return nil
}

// hasBankedRAM reports whether this machine's memory is banked, which is what
// makes the 128K snapshot form meaningful. It asks the *model*, not the mapper:
// the generic mapper allocates all eight banks for every machine, so bank
// presence cannot tell a 48K from a 128K. PagingModel is the same test the port
// bus uses to decide whether to register the paging handler.
func (e *Emulator) hasBankedRAM() bool { return e.cfg.PagingModel != "none" }

// pagingValue reads the last byte written to 0x7FFD, if the mapper keeps one.
// A flat-memory model does not, and 0 is the right answer for it.
func pagingValue(mapper mem.MemoryMapper) uint8 {
	if p, ok := mapper.(interface{ PagingValue() uint8 }); ok {
		return p.PagingValue()
	}
	return 0
}
