package model

// Pentagon128_ROMLayout defines the ROM configuration for the Pentagon 128.
// The two system ROMs live in a single 32 KB image (pentagon/128.rom): bank 0
// is the editor/system ROM and bank 1 is the 48K BASIC ROM. Bank 4 is the Beta
// Disk (TR-DOS) ROM, activated by an instruction fetch from 0x3D00-0x3DFF.
var Pentagon128_ROMLayout = &ROMLayout{
	ROMs: []ROMDescriptor{
		{
			Name:         "pentagon-editor",
			Description:  "Pentagon editor/system ROM",
			BankIndex:    0,
			SizeKB:       16,
			FileOffset:   0,
			EmbeddedName: "pentagon/128.rom",
			Activation: ROMActivation{
				PortSelect: &PortSelect{
					Port:  0x7FFD,
					Mask:  0x10,
					Value: 0x00,
				},
			},
		},
		{
			Name:         "pentagon-basic",
			Description:  "Pentagon 48K BASIC ROM",
			BankIndex:    1,
			SizeKB:       16,
			FileOffset:   16384,
			EmbeddedName: "pentagon/128.rom",
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
			EmbeddedName: "pentagon/trdos.rom",
			Optional:     true, // Beta Disk only needed when disk is used
			Activation: ROMActivation{
				PCRange: &PCRange{
					Low:  0x3D00,
					High: 0x3DFF,
				},
				TriggerFrom: 1, // Activate from ROM 1 (BASIC)
				TriggerTo:   4,
				DeactivatePC: &PCRange{
					Low:  0x4000,
					High: 0xFFFF,
				},
			},
		},
	},
	DefaultROM: 0,
}
