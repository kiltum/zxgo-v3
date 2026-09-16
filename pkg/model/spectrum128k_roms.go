package model

// Spectrum128K_ROMLayout defines the ROM configuration for 128K with TR-DOS support.
var Spectrum128K_ROMLayout = &ROMLayout{
	ROMs: []ROMDescriptor{
		{
			Name:         "128k-editor",
			Description:  "128K Editor ROM (Sinclair BASIC with 128K extensions)",
			BankIndex:    0,
			SizeKB:       16,
			FilePath:     "128-0.rom",
			EmbeddedName: "128k/128-0.rom",
			Activation: ROMActivation{
				PortSelect: &PortSelect{
					Port:  0x7FFD,
					Mask:  0x10,
					Value: 0x00,
				},
			},
		},
		{
			Name:         "128k-menu",
			Description:  "128K Menu/SOS ROM (tape loader, 48K BASIC)",
			BankIndex:    1,
			SizeKB:       16,
			FilePath:     "128-1.rom",
			EmbeddedName: "128k/128-1.rom",
			Activation: ROMActivation{
				PortSelect: &PortSelect{
					Port:  0x7FFD,
					Mask:  0x10,
					Value: 0x10,
				},
			},
		},
		{
			Name:         "trdos",
			Description:  "TR-DOS ROM (Beta Disk operating system)",
			BankIndex:    4,
			SizeKB:       16,
			EmbeddedName: "128k/trdos.rom",
			Optional:     true, // TR-DOS only needed when disk is used
			Activation: ROMActivation{
				PCRange: &PCRange{
					Low:  0x3D00,
					High: 0x3DFF,
				},
				TriggerFrom: 1, // Activate from ROM 1 (menu/SOS)
				TriggerTo:   4,
				DeactivatePC: &PCRange{
					Low:  0x4000,
					High: 0xFFFF,
				},
			},
		},
	},
	DefaultROM: 0, // Start with 128K editor
}

// Update Spectrum128K config to include ROM layout
func init() {
	// This will be integrated into the main Config when we wire it all together
}
