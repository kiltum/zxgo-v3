package emulator

import (
	"fmt"

	"github.com/kiltum/zxgo-v3/pkg/snap"
)

// LoadSNA applies a parsed SNA snapshot to the emulator.
func (e *Emulator) LoadSNA(s *snap.Snapshot) error {
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
	for i := 0; i < 49152; i++ {
		mapper.WriteByte(uint16(0x4000+i), s.RAM[i])
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

	return &snap.Snapshot{Header: header, RAM: ram, Is128K: false}, nil
}
