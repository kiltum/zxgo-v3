package model

// Spectrum48K_ROMLayout defines the ROM configuration for 48K with optional TR-DOS.
var Spectrum48K_ROMLayout = &ROMLayout{
	ROMs: []ROMDescriptor{
		{
			Name:         "48k",
			Description:  "48K BASIC ROM",
			BankIndex:    0,
			SizeKB:       16,
			FilePath:     "48.rom",
			EmbeddedName: "48k/48.rom",
			Activation:   ROMActivation{}, // Always active (no port/PC switching)
		},
		{
			Name:         "trdos",
			Description:  "TR-DOS ROM (Beta Disk operating system)",
			BankIndex:    1,
			SizeKB:       16,
			EmbeddedName: "48k/trdos.rom",
			Optional:     true, // TR-DOS only needed when disk is used
			Activation: ROMActivation{
				PCRange: &PCRange{
					Low:  0x3D00,
					High: 0x3DFF,
				},
				TriggerFrom: 0, // Activate from ROM 0 (48K BASIC)
				TriggerTo:   1,
				DeactivatePC: &PCRange{
					Low:  0x4000,
					High: 0xFFFF,
				},
			},
		},
	},
	DefaultROM: 0, // Start with 48K BASIC
}
