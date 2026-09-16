package banking

import (
	"testing"

	"github.com/kiltum/zxgo-v3/pkg/model"
)

func TestPortSelect_MatchesPort(t *testing.T) {
	tests := []struct {
		name     string
		ps       *model.PortSelect
		port     uint16
		value    uint8
		expected bool
	}{
		{"nil selector", nil, 0x7FFD, 0x10, false},
		{"128K ROM 0", &model.PortSelect{Port: 0x7FFD, Mask: 0x10, Value: 0x00}, 0x7FFD, 0x00, true},
		{"128K ROM 1", &model.PortSelect{Port: 0x7FFD, Mask: 0x10, Value: 0x10}, 0x7FFD, 0x10, true},
		{"wrong port", &model.PortSelect{Port: 0x7FFD, Mask: 0x10, Value: 0x00}, 0x1FFD, 0x00, false},
		{"wrong value", &model.PortSelect{Port: 0x7FFD, Mask: 0x10, Value: 0x00}, 0x7FFD, 0x10, false},
		{"mask works", &model.PortSelect{Port: 0x7FFD, Mask: 0x10, Value: 0x10}, 0x7FFD, 0x1F, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.ps.MatchesPort(tt.port, tt.value)
			if got != tt.expected {
				t.Errorf("MatchesPort() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestPCRange_ContainsPC(t *testing.T) {
	tests := []struct {
		name     string
		pr       *model.PCRange
		pc       uint16
		expected bool
	}{
		{"nil range", nil, 0x3D00, false},
		{"TR-DOS range start", &model.PCRange{Low: 0x3D00, High: 0x3DFF}, 0x3D00, true},
		{"TR-DOS range end", &model.PCRange{Low: 0x3D00, High: 0x3DFF}, 0x3DFF, true},
		{"TR-DOS range middle", &model.PCRange{Low: 0x3D00, High: 0x3DFF}, 0x3D80, true},
		{"before range", &model.PCRange{Low: 0x3D00, High: 0x3DFF}, 0x3CFF, false},
		{"after range", &model.PCRange{Low: 0x3D00, High: 0x3DFF}, 0x3E00, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.pr.ContainsPC(tt.pc)
			if got != tt.expected {
				t.Errorf("ContainsPC() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestROMActivation_ActivatesOnPort(t *testing.T) {
	rom0 := model.ROMActivation{
		PortSelect: &model.PortSelect{Port: 0x7FFD, Mask: 0x10, Value: 0x00},
	}
	rom1 := model.ROMActivation{
		PortSelect: &model.PortSelect{Port: 0x7FFD, Mask: 0x10, Value: 0x10},
	}
	noportROM := model.ROMActivation{
		PCRange: &model.PCRange{Low: 0x3D00, High: 0x3DFF},
	}

	tests := []struct {
		name     string
		act      *model.ROMActivation
		port     uint16
		value    uint8
		expected bool
	}{
		{"ROM 0 activated", &rom0, 0x7FFD, 0x00, true},
		{"ROM 1 activated", &rom1, 0x7FFD, 0x10, true},
		{"ROM 0 not activated", &rom0, 0x7FFD, 0x10, false},
		{"no port activation", &noportROM, 0x7FFD, 0x00, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.act.ActivatesOnPort(tt.port, tt.value)
			if got != tt.expected {
				t.Errorf("ActivatesOnPort() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestROMActivation_ActivatesOnPC(t *testing.T) {
	// TR-DOS: activates when PC in 0x3D00-0x3DFF and current ROM is 1
	trdos := model.ROMActivation{
		PCRange:     &model.PCRange{Low: 0x3D00, High: 0x3DFF},
		TriggerFrom: 1,
		TriggerTo:   4,
	}

	tests := []struct {
		name       string
		act        *model.ROMActivation
		pc         uint16
		currentROM int
		expected   bool
	}{
		{"TR-DOS activates from ROM1", &trdos, 0x3D00, 1, true},
		{"TR-DOS in range from ROM1", &trdos, 0x3D80, 1, true},
		{"TR-DOS wrong ROM", &trdos, 0x3D00, 0, false},
		{"TR-DOS out of range", &trdos, 0x3CFF, 1, false},
		{"no PC activation", &model.ROMActivation{PortSelect: &model.PortSelect{Port: 0x7FFD, Mask: 0x10, Value: 0x00}}, 0x3D00, 1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.act.ActivatesOnPC(tt.pc, tt.currentROM)
			if got != tt.expected {
				t.Errorf("ActivatesOnPC() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestROMActivation_DeactivatesOnPC(t *testing.T) {
	// TR-DOS deactivates when PC >= 0x4000
	trdos := model.ROMActivation{
		DeactivatePC: &model.PCRange{Low: 0x4000, High: 0xFFFF},
	}

	tests := []struct {
		name     string
		act      *model.ROMActivation
		pc       uint16
		expected bool
	}{
		{"TR-DOS stays active in ROM area", &trdos, 0x3FFF, false},
		{"TR-DOS deactivates at 0x4000", &trdos, 0x4000, true},
		{"TR-DOS deactivates above 0x4000", &trdos, 0x8000, true},
		{"no deactivation", &model.ROMActivation{PCRange: &model.PCRange{Low: 0x3D00, High: 0x3DFF}}, 0x4000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.act.DeactivatesOnPC(tt.pc)
			if got != tt.expected {
				t.Errorf("DeactivatesOnPC() = %v, want %v", got, tt.expected)
			}
		})
	}
}
