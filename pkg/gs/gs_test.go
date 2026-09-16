package gs

import (
	"testing"

	"github.com/kiltum/zxgo-v3/pkg/rom"
)

// TestGSMemoryMapping verifies the 4-segment memory map and page selection.
func TestGSMemoryMapping(t *testing.T) {
	g := New(3500000)

	rom := make([]byte, romSize)
	rom[0] = 0xAA                 // ROM[0] byte 0
	rom[blockSize] = 0xBB         // ROM[1] byte 0
	if err := g.LoadROM(rom); err != nil {
		t.Fatal(err)
	}

	// Segment 0 = ROM[0], read-only.
	if got := g.ReadMemory(0x0000); got != 0xAA {
		t.Errorf("0x0000 = %02X, want AA", got)
	}
	g.WriteMemory(0x0000, 0x11)
	if got := g.ReadMemory(0x0000); got != 0x00AA {
		t.Errorf("ROM write leaked: 0x0000 = %02X, want AA", got)
	}

	// Segment 1 = RAM[15], writable.
	g.WriteMemory(0x4000, 0xCC)
	if got := g.ReadMemory(0x4000); got != 0xCC {
		t.Errorf("0x4000 = %02X, want CC", got)
	}

	// mapping=0: upper window is ROM (0x8000=ROM[0], 0xC000=ROM[1]).
	if got := g.ReadMemory(0x8000); got != 0xAA {
		t.Errorf("mapping=0: 0x8000 = %02X, want AA", got)
	}
	if got := g.ReadMemory(0xC000); got != 0xBB {
		t.Errorf("mapping=0: 0xC000 = %02X, want BB", got)
	}

	// mapping=1: upper window is RAM blocks 0 and 1.
	g.gsWritePort(0, 1)
	g.WriteMemory(0x8000, 0xDD)
	if got := g.ReadMemory(0x8000); got != 0xDD {
		t.Errorf("mapping=1: 0x8000 = %02X, want DD (RAM[0])", got)
	}
}

// TestGSDACCapture verifies a read of 0x6000-0x7FFF feeds the DAC channel
// selected by A8-A9, with the channel volume applied.
func TestGSDACCapture(t *testing.T) {
	g := New(3500000)

	g.gsWritePort(6, 0x3F) // channel 0 volume = full
	g.gsWritePort(7, 0x3F) // channel 1 volume = full
	g.gsWritePort(8, 0x3F) // channel 2 volume = full

	g.WriteMemory(0x6000, 0x80) // centre sample -> signed 0
	g.ReadMemory(0x6000)        // capture into channel 0
	if g.dac[0] != 128 {
		t.Errorf("dac[0] = %d, want 128", g.dac[0])
	}

	g.WriteMemory(0x6100, 0xFF) // channel 1, signed +127
	g.ReadMemory(0x6100)
	if g.dac[1] != 255 {
		t.Errorf("dac[1] = %d, want 255", g.dac[1])
	}

	g.WriteMemory(0x6200, 0x00) // channel 2, signed -128
	g.ReadMemory(0x6200)
	if g.dac[2] != 0 {
		t.Errorf("dac[2] = %d, want 0", g.dac[2])
	}
}

// TestGSHostHandshake verifies the 0xBB/0xB3 command/data/status/output protocol.
func TestGSHostHandshake(t *testing.T) {
	g := New(3500000)
	g.Reset()

	g.Write(0xBB, 0x42) // command
	if g.command != 0x42 || g.state&1 == 0 {
		t.Errorf("command write: command=%02X state=%02X", g.command, g.state)
	}

	g.Write(0xB3, 0x24) // data
	if g.data != 0x24 || g.state&0x80 == 0 {
		t.Errorf("data write: data=%02X state=%02X", g.data, g.state)
	}

	if got := g.Read(0xBB); got != g.state {
		t.Errorf("status read = %02X, want %02X", got, g.state)
	}

	g.gsWritePort(3, 0x99) // GS writes output
	if got := g.Read(0xB3); got != 0x99 {
		t.Errorf("output read = %02X, want 99", got)
	}
	if g.state&0x80 != 0 {
		t.Errorf("state bit 7 should clear after output read, got %02X", g.state)
	}
}

// TestGSRuns loads the embedded GS ROM and verifies the GS Z80 advances.
func TestGSRuns(t *testing.T) {
	g := New(3500000)
	romData := rom.Get("gs/gs105a.rom")
	if romData == nil {
		t.Skip("GS ROM not embedded")
	}
	if err := g.LoadROM(romData); err != nil {
		t.Fatal(err)
	}

	g.Tick(69888) // one Spectrum frame
	if g.gsTicks <= 0 {
		t.Fatal("GS Z80 did not advance")
	}
	t.Logf("GS advanced %d T-states, PC=%04X", g.gsTicks, g.cpu.PC)
}
