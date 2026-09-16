// Package rom provides embedded ROM storage organized by model.
// Each model directory contains the ROMs needed for that model.
package rom

import _ "embed"

// Embedded 48K ROMs
//
//go:embed 48k/48.rom
var rom48k []byte

//go:embed 48k/trdos.rom
var romTRDOS48k []byte

// Embedded 128K ROMs
//
//go:embed 128k/128-0.rom
var rom128k0 []byte

//go:embed 128k/128-1.rom
var rom128k1 []byte

//go:embed 128k/trdos.rom
var romTRDOS128k []byte

// Embedded Pentagon ROMs
//
//go:embed pentagon/128.rom
var romPentagon128 []byte

//go:embed pentagon/trdos.rom
var romPentagonTRDOS []byte

//go:embed 2a3/plus3-0.rom
var rom2a3_0 []byte

//go:embed 2a3/plus3-1.rom
var rom2a3_1 []byte

//go:embed 2a3/plus3-2.rom
var rom2a3_2 []byte

//go:embed 2a3/plus3-3.rom
var rom2a3_3 []byte

// Embedded General Sound ROM
//
//go:embed gs/gs105a.rom
var romGS []byte

// Registry maps embedded ROM names to their data.
var Registry = map[string][]byte{
	"48k/48.rom":         rom48k,
	"48k/trdos.rom":      romTRDOS48k,
	"128k/128-0.rom":     rom128k0,
	"128k/128-1.rom":     rom128k1,
	"128k/trdos.rom":     romTRDOS128k,
	"pentagon/128.rom":   romPentagon128,
	"pentagon/trdos.rom": romPentagonTRDOS,
	"2a3/plus3-0.rom":    rom2a3_0,
	"2a3/plus3-1.rom":    rom2a3_1,
	"2a3/plus3-2.rom":    rom2a3_2,
	"2a3/plus3-3.rom":    rom2a3_3,
	"gs/gs105a.rom":      romGS,
}

// Get returns the embedded ROM data for the given name.
// Returns nil if the ROM is not found.
func Get(name string) []byte {
	return Registry[name]
}
