// Package media implements ZX Spectrum media formats: tape (TAP/TZX),
// disk (TR-DOS, +3 DOS), and printer.
//
// Tape subsystem generates a bitstream synchronized with CPU T-states that
// drives the ULA's EAR bit (port 0xFE, bit 6). This enables proper tape
// loader emulation supporting all standard and custom loaders.
//
// Disk subsystem implements floppy disk controller emulation:
//   - Beta Disk: WD1793 controller with .TRD/.SCL formats
//   - +3 DOS: uPD765 controller with .DSK format
//
// Architecture:
//
//	Tape interface -> TAP/TZX parsers -> bitstream -> Playback -> ULA.SetAudioState()
//	Disk interface -> TRD/SCL/DSK parsers -> Disk -> BetaDiskController/WD1793
package media
