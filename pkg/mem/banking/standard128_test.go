package banking

import (
	"testing"

	"github.com/kiltum/zxgo-v3/pkg/model"
)

func TestStandard128_MapAddress(t *testing.T) {
	s := NewStandard128(nil)
	s.activeROM = 1
	s.ramBankSlot3 = 3

	tests := []struct {
		name           string
		addr           uint16
		expectedType   BankType
		expectedBank   int
		expectedOffset uint16
	}{
		{"ROM start", 0x0000, BankROM, 1, 0x0000},
		{"ROM end", 0x3FFF, BankROM, 1, 0x3FFF},
		{"RAM bank 5 start", 0x4000, BankRAM, 5, 0x0000},
		{"RAM bank 5 end", 0x7FFF, BankRAM, 5, 0x3FFF},
		{"RAM bank 2 start", 0x8000, BankRAM, 2, 0x0000},
		{"RAM bank 2 end", 0xBFFF, BankRAM, 2, 0x3FFF},
		{"RAM bank 3 start", 0xC000, BankRAM, 3, 0x0000},
		{"RAM bank 3 end", 0xFFFF, BankRAM, 3, 0x3FFF},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typ, bank, offset := s.MapAddress(tt.addr)
			if typ != tt.expectedType || bank != tt.expectedBank || offset != tt.expectedOffset {
				t.Errorf("MapAddress(0x%04X) = (%d, %d, 0x%04X), want (%d, %d, 0x%04X)",
					tt.addr, typ, bank, offset, tt.expectedType, tt.expectedBank, tt.expectedOffset)
			}
		})
	}
}

func TestStandard128_WritePort(t *testing.T) {
	s := NewStandard128(nil)

	// Port 0x7FFD: RAM bank 3, shadow screen on, ROM 1
	handled := s.WritePort(0x7FFD, 0x1B) // 0001 1011 = ROM1, shadow, bank 3
	if !handled {
		t.Error("WritePort(0x7FFD) should be handled")
	}
	if s.ramBankSlot3 != 3 {
		t.Errorf("ramBankSlot3 = %d, want 3", s.ramBankSlot3)
	}
	if !s.shadowScreen {
		t.Error("shadowScreen should be true")
	}
	if s.activeROM != 1 {
		t.Errorf("activeROM = %d, want 1", s.activeROM)
	}

	// Paging lock
	s.WritePort(0x7FFD, 0x20) // D5 set
	if !s.pagingLocked {
		t.Error("pagingLocked should be true")
	}

	// Further writes should be ignored (returns true but doesn't change state)
	oldBank := s.ramBankSlot3
	s.WritePort(0x7FFD, 0x00)
	if s.ramBankSlot3 != oldBank {
		t.Error("ramBankSlot3 should not change when locked")
	}
}

func TestStandard128_FetchOpcode_TRDOSActivation(t *testing.T) {
	// Set up ROM layout with TR-DOS activation
	layout := &model.ROMLayout{
		ROMs: []model.ROMDescriptor{
			{
				Name:      "128k-editor",
				BankIndex: 0,
				Activation: model.ROMActivation{
					PortSelect: &model.PortSelect{Port: 0x7FFD, Mask: 0x10, Value: 0x00},
				},
			},
			{
				Name:      "128k-sos",
				BankIndex: 1,
				Activation: model.ROMActivation{
					PortSelect: &model.PortSelect{Port: 0x7FFD, Mask: 0x10, Value: 0x10},
				},
			},
			{
				Name:      "trdos",
				BankIndex: 4,
				Activation: model.ROMActivation{
					PCRange:      &model.PCRange{Low: 0x3D00, High: 0x3DFF},
					TriggerFrom:  1,
					TriggerTo:    4,
					DeactivatePC: &model.PCRange{Low: 0x4000, High: 0xFFFF},
				},
			},
		},
		DefaultROM: 0,
	}

	s := NewStandard128(layout)
	s.activeROM = 1 // ROM_SOS

	// Fetch from 0x3D00 should activate TR-DOS
	newROM := s.FetchOpcode(0x3D00)
	if newROM != 4 {
		t.Errorf("FetchOpcode(0x3D00) = %d, want 4 (TR-DOS)", newROM)
	}
	if s.activeROM != 4 {
		t.Errorf("activeROM = %d, want 4", s.activeROM)
	}

	// Fetch from 0x4000 should deactivate TR-DOS
	newROM = s.FetchOpcode(0x4000)
	if newROM != 1 {
		t.Errorf("FetchOpcode(0x4000) = %d, want 1 (back to SOS)", newROM)
	}
	if s.activeROM != 1 {
		t.Errorf("activeROM = %d, want 1", s.activeROM)
	}
}

func TestStandard128_Reset(t *testing.T) {
	s := NewStandard128(nil)
	s.activeROM = 4
	s.ramBankSlot3 = 7
	s.shadowScreen = true
	s.pagingLocked = true

	s.Reset()

	if s.activeROM != 0 {
		t.Errorf("activeROM after reset = %d, want 0", s.activeROM)
	}
	if s.ramBankSlot3 != 0 {
		t.Errorf("ramBankSlot3 after reset = %d, want 0", s.ramBankSlot3)
	}
	if s.shadowScreen {
		t.Error("shadowScreen after reset should be false")
	}
	if s.pagingLocked {
		t.Error("pagingLocked after reset should be false")
	}
}

func TestStandard128_Encode(t *testing.T) {
	s := NewStandard128(nil)
	s.activeROM = 1
	s.ramBankSlot3 = 3
	s.shadowScreen = true
	s.pagingLocked = true
	s.previousROM = 4

	blob := s.Encode()

	s2 := NewStandard128(nil)
	if err := s2.Decode(blob); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if s2.activeROM != 1 || s2.ramBankSlot3 != 3 || !s2.shadowScreen || !s2.pagingLocked || s2.previousROM != 4 {
		t.Errorf("Decode() gave activeROM=%d slot3=%d shadow=%v locked=%v previous=%d",
			s2.activeROM, s2.ramBankSlot3, s2.shadowScreen, s2.pagingLocked, s2.previousROM)
	}

	// A truncated blob must leave the scheme as it was.
	s3 := NewStandard128(nil)
	s3.ramBankSlot3 = 6
	if err := s3.Decode(blob[:3]); err == nil {
		t.Error("Decode() accepted a truncated blob")
	}
	if s3.ramBankSlot3 != 6 {
		t.Errorf("a failed Decode changed state: slot3 = %d, want 6", s3.ramBankSlot3)
	}

	// Pentagon512 shares the encoding but is a different scheme, so the name in
	// the enclosing paging state is what keeps the two apart.
	if NewPentagon512(nil).Name() == s.Name() {
		t.Error("Pentagon512 and Standard128 report the same scheme name")
	}
}
