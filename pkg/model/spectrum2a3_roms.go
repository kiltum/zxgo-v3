package model

// Spectrum2A3_ROMLayout defines the ROM configuration for +2A/+3.
var Spectrum2A3_ROMLayout = &ROMLayout{
	ROMs: []ROMDescriptor{
		{
			Name:         "plus3-0",
			Description:  "+2A/+3 ROM 0 (128K editor)",
			BankIndex:    0,
			SizeKB:       16,
			FilePath:     "plus3-0.rom",
			EmbeddedName: "2a3/plus3-0.rom",
			Activation: ROMActivation{
				PortSelect: &PortSelect{
					Port:  0x7FFD,
					Mask:  0x10,
					Value: 0x00,
				},
			},
		},
		{
			Name:         "plus3-1",
			Description:  "+2A/+3 ROM 1 (syntax checker)",
			BankIndex:    1,
			SizeKB:       16,
			FilePath:     "plus3-1.rom",
			EmbeddedName: "2a3/plus3-1.rom",
			Activation: ROMActivation{
				PortSelect: &PortSelect{
					Port:  0x7FFD,
					Mask:  0x10,
					Value: 0x10,
				},
			},
		},
		{
			Name:         "plus3-2",
			Description:  "+3 ROM 2 (+3DOS)",
			BankIndex:    2,
			SizeKB:       16,
			FilePath:     "plus3-2.rom",
			EmbeddedName: "2a3/plus3-2.rom",
			Optional:     true, // +2A doesn't have this
			Activation: ROMActivation{
				PortSelect: &PortSelect{
					Port:  0x1FFD,
					Mask:  0x04,
					Value: 0x04,
				},
			},
		},
		{
			Name:         "plus3-3",
			Description:  "+3 ROM 3 (48K BASIC)",
			BankIndex:    3,
			SizeKB:       16,
			FilePath:     "plus3-3.rom",
			EmbeddedName: "2a3/plus3-3.rom",
			Optional:     true, // +2A doesn't have this
			Activation: ROMActivation{
				PortSelect: &PortSelect{
					Port:  0x1FFD,
					Mask:  0x04,
					Value: 0x00,
				},
			},
		},
	},
	DefaultROM: 0, // Start with ROM 0
}
