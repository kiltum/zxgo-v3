package media

import (
	"bytes"
	"testing"
)

// TestNewTRDisk tests creation of a new TR-DOS format disk
func TestNewTRDisk(t *testing.T) {
	disk := NewTRDisk()

	if disk.Type != DiskTypeTRD {
		t.Errorf("Expected disk type TRD, got %s", disk.Type)
	}

	if disk.Sides != 2 {
		t.Errorf("Expected 2 sides, got %d", disk.Sides)
	}

	if disk.TracksPerSide != 80 {
		t.Errorf("Expected 80 tracks per side, got %d", disk.TracksPerSide)
	}

	// TR-DOS packs 16 sectors of 256 bytes into a track, not 9.
	if disk.SectorsPerTrack != 16 {
		t.Errorf("Expected 16 sectors per track, got %d", disk.SectorsPerTrack)
	}

	if disk.SectorSize != 256 {
		t.Errorf("Expected sector size 256, got %d", disk.SectorSize)
	}

	// 80 tracks * 2 sides * 16 sectors = 2560 sectors = 640 KB.
	totalSectors := len(disk.Tracks) * disk.SectorsPerTrack
	if totalSectors != 2560 {
		t.Errorf("Expected 2560 total sectors, got %d", totalSectors)
	}
}

// TestDiskSectorLayout tests that sectors are properly initialized
func TestDiskSectorLayout(t *testing.T) {
	disk := NewTRDisk()

	// Test first track, first sector
	track0 := disk.Tracks[0]
	if len(track0.Sectors) != 16 {
		t.Fatalf("Expected 16 sectors on track 0, got %d", len(track0.Sectors))
	}

	sector1 := track0.Sectors[0]
	if sector1.Track != 0 {
		t.Errorf("Expected sector track 0, got %d", sector1.Track)
	}
	if sector1.Side != 0 {
		t.Errorf("Expected sector side 0, got %d", sector1.Side)
	}
	if sector1.SectorID != 1 {
		t.Errorf("Expected sector ID 1, got %d", sector1.SectorID)
	}
	if len(sector1.Data) != 256 {
		t.Errorf("Expected sector data length 256, got %d", len(sector1.Data))
	}

	// Test last track, last sector
	track79 := disk.Tracks[79]
	sector16 := track79.Sectors[15]
	if sector16.Track != 79 {
		t.Errorf("Expected track 79, got %d", sector16.Track)
	}
	if sector16.SectorID != 16 {
		t.Errorf("Expected sector ID 16, got %d", sector16.SectorID)
	}
}

// TestRoundTRDisk tests round-trip loading and saving of TR-DOS format
func TestRoundTRDisk(t *testing.T) {
	// Create original disk
	original := NewTRDisk()

	// Write some test data to specific sectors
	testData := []byte("Hello, ZX Spectrum!")
	original.Tracks[0].Sectors[0].Data = make([]byte, 256)
	original.Tracks[0].Sectors[0].Data[10] = 'H'
	original.Tracks[0].Sectors[0].Data[11] = 'e'
	original.Tracks[0].Sectors[0].Data[12] = 'l'
	original.Tracks[0].Sectors[0].Data[13] = 'l'
	original.Tracks[0].Sectors[0].Data[14] = 'o'
	original.Tracks[10].Sectors[5].Data = make([]byte, 256)
	copy(original.Tracks[10].Sectors[5].Data, testData)

	// Save to buffer
	var buf bytes.Buffer
	err := SaveTRDisk(&buf, original)
	if err != nil {
		t.Fatalf("SaveTRDisk failed: %v", err)
	}

	// Load back
	loaded, err := LoadTRDisk(&buf)
	if err != nil {
		t.Fatalf("LoadTRDisk failed: %v", err)
	}

	// Compare structure
	if loaded.Type != original.Type {
		t.Errorf("Type mismatch: got %s, expected %s", loaded.Type, original.Type)
	}
	if loaded.TracksPerSide != original.TracksPerSide {
		t.Errorf("TracksPerSide mismatch: got %d, expected %d", loaded.TracksPerSide, original.TracksPerSide)
	}

	// Compare specific sectors
	if loaded.Tracks[0].Sectors[0].Data[10] != 'H' {
		t.Errorf("Track 0, Sector 0 Data mismatch: expected 'H', got %c", loaded.Tracks[0].Sectors[0].Data[10])
	}

	loadedData := loaded.Tracks[10].Sectors[5].Data
	expectData := original.Tracks[10].Sectors[5].Data
	if !bytes.Equal(loadedData[:len(testData)], expectData[:len(testData)]) {
		t.Errorf("Test data mismatch after round-trip")
	}
}

// TestReadSector tests reading sectors from disk
func TestReadSector(t *testing.T) {
	disk := NewTRDisk()
	disk.Tracks[5].Sectors[3].Data[10] = 0xAA
	disk.Tracks[5].Sectors[3].Data[11] = 0xBB

	data, err := disk.ReadSector(5, 0, 4) // Track 5, side 0, sector 4
	if err != nil {
		t.Fatalf("ReadSector failed: %v", err)
	}

	if data[10] != 0xAA {
		t.Errorf("Expected 0xAA at offset 10, got %02X", data[10])
	}
	if data[11] != 0xBB {
		t.Errorf("Expected 0xBB at offset 11, got %02X", data[11])
	}

	// Read return should be a copy, not the original
	data[0] = 0xFF
	if disk.Tracks[5].Sectors[3].Data[0] == 0xFF {
		t.Errorf("ReadSector returned original data instead of copy")
	}
}

// TestWriteSector tests writing sectors to disk
func TestWriteSector(t *testing.T) {
	disk := NewTRDisk()

	writeData := make([]byte, 256)
	for i := 0; i < 256; i++ {
		writeData[i] = byte(i % 256)
	}

	err := disk.WriteSector(7, 0, 2, writeData)
	if err != nil {
		t.Fatalf("WriteSector failed: %v", err)
	}

	// Verify data was written
	readData, err := disk.ReadSector(7, 0, 2)
	if err != nil {
		t.Fatalf("ReadSector after WriteSector failed: %v", err)
	}

	if !bytes.Equal(readData, writeData) {
		t.Error("Written data doesn't match read data")
	}
}

// TestWriteProtected tests write protection
func TestWriteProtected(t *testing.T) {
	disk := NewTRDisk()
	disk.ReadOnly = true

	writeData := make([]byte, 256)
	err := disk.WriteSector(0, 0, 1, writeData)
	if err == nil {
		t.Error("Expected error when writing to protected disk")
	}

	if err.Error() != "disk is write protected" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// TestInvalidSector tests error handling for invalid sectors
func TestInvalidSector(t *testing.T) {
	disk := NewTRDisk()

	// Invalid track number
	_, err := disk.ReadSector(100, 0, 1)
	if err == nil {
		t.Error("Expected error for invalid track number")
	}

	// Invalid sector number
	_, err = disk.ReadSector(0, 0, 20)
	if err == nil {
		t.Error("Expected error for invalid sector number")
	}
}

// TestFormatDisk tests disk formatting
func TestFormatDisk(t *testing.T) {
	disk := NewTRDisk()

	// Corrupt some sectors
	disk.Tracks[5].Sectors[3].Data[10] = 0xAA
	disk.Tracks[10].Sectors[7].Data[20] = 0xBB

	// Format disk
	err := disk.FormatDisk()
	if err != nil {
		t.Fatalf("FormatDisk failed: %v", err)
	}

	// Verify all sectors are cleared
	for track := 0; track < len(disk.Tracks); track++ {
		for sector := 0; sector < disk.SectorsPerTrack; sector++ {
			sec := disk.Tracks[track].Sectors[sector]
			for i := 0; i < len(sec.Data); i++ {
				if sec.Data[i] != 0 {
					t.Errorf("Track %d, Sector %d not cleared: found %02X at offset %d", track, sector, sec.Data[i], i)
					return
				}
			}
			if sec.Deleted {
				t.Errorf("Track %d, Sector %d marked as deleted after format", track, sector)
			}
		}
	}
}

// TestBetaDiskController tests the Beta Disk controller
func TestBetaDiskController(t *testing.T) {
	ctrl := NewBetaDiskController()

	// Initial state: NOT_READY (no disk) plus TRACK00, which reflects head
	// position and is set whether or not a disk is present.
	if ctrl.GetStatus() != 0x84 {
		t.Errorf("Expected status 0x84, got 0x%02X", ctrl.GetStatus())
	}
	if ctrl.GetTrack() != 0 {
		t.Errorf("Expected track 0, got %d", ctrl.GetTrack())
	}
	if ctrl.GetSector() != 1 {
		t.Errorf("Expected sector 1, got %d", ctrl.GetSector())
	}
}

// TestBetaDiskMountUnmount tests disk mounting and unmounting
func TestBetaDiskMountUnmount(t *testing.T) {
	ctrl := NewBetaDiskController()
	disk := NewTRDisk()

	// Mount disk
	ctrl.MountDisk(disk)
	if ctrl.ActiveDisk() == nil {
		t.Error("Expected disk mounted flag to be true ActiveDisk() != nil")
	}

	// Unmount disk
	ctrl.UnmountDisk()
	if ctrl.ActiveDisk() != nil {
		t.Error("Expected disk mounted flag to be false ActiveDisk() == nil")
	}
}

// TestBetaDiskPortIO tests port I/O operations
func TestBetaDiskPortIO(t *testing.T) {
	ctrl := NewBetaDiskController()
	disk := NewTRDisk()
	ctrl.MountDisk(disk)

	// Test writing to track register
	ctrl.WritePort(0x3F, 0x2B) // Address 0x3F selects track register
	if ctrl.GetTrack() != 0x2B {
		t.Errorf("Expected track 0x2B, got %d", ctrl.GetTrack())
	}

	// Test writing to sector register
	ctrl.WritePort(0x5F, 0x0A) // Address 0x5F selects sector register
	if ctrl.GetSector() != 0x0A {
		t.Errorf("Expected sector 0x0A, got %d", ctrl.GetSector())
	}

	// Test reading from status register
	// The STATUS register is composed on read (index pulse and TRACK00 come
	// from drive state, not from the stored byte), so compare against the
	// composed value.
	status := ctrl.ReadPort(0x1F)
	if status != ctrl.statusByte() {
		t.Errorf("Port read mismatch: expected 0x%02X, got 0x%02X", ctrl.statusByte(), status)
	}
}

// TestBetaDiskRestoreCommand tests the Restore command
func TestBetaDiskRestoreCommand(t *testing.T) {
	ctrl := NewBetaDiskController()
	disk := NewTRDisk()
	ctrl.MountDisk(disk)

	// Set to some track first
	ctrl.WritePort(0x3F, 50)
	if ctrl.GetTrack() != 50 {
		t.Fatalf("Failed to set initial track")
	}

	// Send Restore command
	ctrl.WritePort(0x1F, uint8(byte(0x00)))

	// RESTORE steps at the r1r0 rate and settles (h bit); advance the clock past
	// it so the command completes.
	ctrl.SetTick(ctrl.typeICompleteTick)

	// Check that track returned to 0
	if ctrl.GetTrack() != 0 {
		t.Errorf("Expected track 0 after Restore, got %d", ctrl.GetTrack())
	}

	// Check that busy flag is cleared and interrupt is pending
	if ctrl.GetStatus()&uint8(BetaDiskBusy) != 0 {
		t.Error("Expected busy flag cleared after Restore")
	}
	if !ctrl.GetInterruptRequest() {
		t.Error("Expected interrupt request after Restore")
	}
}

// TestBetaDiskSeekCommand tests the Seek command
func TestBetaDiskSeekCommand(t *testing.T) {
	ctrl := NewBetaDiskController()
	disk := NewTRDisk()
	ctrl.MountDisk(disk)

	// Write target track to data register
	ctrl.WritePort(0x7F, 42) // Data register at 0x7F

	// Send Seek command
	ctrl.WritePort(0x1F, uint8(byte(0x10)))

	if ctrl.GetTrack() != 42 {
		t.Errorf("Expected track 42 after Seek, got %d", ctrl.GetTrack())
	}
}

// TestBetaDiskStepCommands tests Step, StepIn, StepOut commands
func TestBetaDiskStepCommands(t *testing.T) {
	ctrl := NewBetaDiskController()
	disk := NewTRDisk()
	ctrl.MountDisk(disk)

	// Set initial track to 10 using Seek
	ctrl.WritePort(0x7F, 10) // Data register to 10
	ctrl.WritePort(0x1F, byte(0x10)) // Seek: bit4=1, bit6=0, to track in data reg

	if ctrl.GetTrack() != 10 {
		t.Fatalf("Setup: Expected track 10 after Seek, got %d", ctrl.GetTrack())
	}

	// Step IN (bit5=0) from track 10 -> 11
	ctrl.WritePort(0x1F, byte(0x50)) // Step: bit6=1, bit4=1, bit5=0 -> IN
	if ctrl.GetTrack() != 11 {
		t.Errorf("Step-IN: expected track 11, got %d", ctrl.GetTrack())
	}

	// Step OUT (bit5=1) from track 11 -> 10
	ctrl.WritePort(0x1F, byte(0x70)) // Step: bit6=1, bit4=1, bit5=1 -> OUT
	if ctrl.GetTrack() != 10 {
		t.Errorf("Step-OUT: expected track 10, got %d", ctrl.GetTrack())
	}

	// Another Step IN from track 10 -> 11
	ctrl.WritePort(0x1F, byte(0x50))
	if ctrl.GetTrack() != 11 {
		t.Errorf("Step-IN: expected track 11, got %d", ctrl.GetTrack())
	}

	// Restore to 0, then Step OUT from 0 should stay at 0
	ctrl.WritePort(0x1F, byte(0x00)) // Restore
	ctrl.WritePort(0x1F, byte(0x70)) // Step OUT cannot go below 0
	if ctrl.GetTrack() != 0 {
		t.Errorf("Step-OUT-from-0: expected track 0, got %d", ctrl.GetTrack())
	}

	// SEEK to track 79, then Step IN stays at 79 (capped at 85)
	ctrl.WritePort(0x7F, 79)          // data reg = 79
	ctrl.WritePort(0x1F, byte(0x10))  // Seek: reads data reg -> track=79
	ctrl.WritePort(0x1F, byte(0x50))  // Step IN: 79->80
	if ctrl.GetTrack() != 80 {
		t.Errorf("Step-IN from 79: expected track 80, got %d", ctrl.GetTrack())
	}
}

// TestBetaDiskReset tests controller reset
func TestBetaDiskReset(t *testing.T) {
	ctrl := NewBetaDiskController()
	disk := NewTRDisk()
	ctrl.MountDisk(disk)

	// Set some values
	ctrl.WritePort(0x3F, 50)
	ctrl.WritePort(0x5F, 15)
	ctrl.WritePort(0x1F, uint8(byte(0x10)))

	// Reset controller
	ctrl.Reset()

	// A disk is mounted in this test, so NOT_READY clears on reset; TRACK00
	// and the index pulse are composed from drive state.
	if got := ctrl.GetStatus(); got&0x80 != 0 {
		t.Errorf("Expected NOT_READY clear after reset with disk mounted, got 0x%02X", got)
	}
	if ctrl.GetTrack() != 0 {
		t.Errorf("Expected track 0 after reset, got %d", ctrl.GetTrack())
	}
	if ctrl.GetSector() != 1 {
		t.Errorf("Expected sector 1 after reset, got %d", ctrl.GetSector())
	}
}

// TestDetectDiskFormat tests format detection
func TestDetectDiskFormat(t *testing.T) {
	// Test TR-DOS format
	trdData := make([]byte, 80*9*256)
	trdData[228] = 0x03 // Typical TR-DOS disk type
	if DetectDiskFormat(trdData) != DiskTypeTRD {
		t.Error("Expected DiskTypeTRD for TR-DOS format")
	}

	// Test unknown format
	unknownData := make([]byte, 1000)
	if DetectDiskFormat(unknownData) != DiskTypeRAW {
		t.Error("Expected DiskTypeRAW for unknown format")
	}

	// Test too short data
	shortData := make([]byte, 100)
	if DetectDiskFormat(shortData) != DiskTypeRAW {
		t.Error("Expected DiskTypeRAW for short data")
	}
}

// TestGetDiskInfo tests disk information retrieval
func TestGetDiskInfo(t *testing.T) {
	disk := NewTRDisk()
	info := disk.GetDiskInfo()

	expectedInfo := "Disk Type: TRD, Tracks: 80, Sides: 2, Sectors/Track: 16, Size: 655360 bytes"
	if info != expectedInfo {
		t.Errorf("Expected '%s', got '%s'", expectedInfo, info)
	}
}