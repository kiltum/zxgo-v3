package media

import (
	"bytes"
	"testing"
)

// TestDiskInfoExtraction tests the GetDetailedInfo function
func TestDiskInfoExtraction(t *testing.T) {
	// Create a test disk
	disk := NewTRDisk()

	// TR-DOS keeps its disk metadata in the disk-information sector -- track 0,
	// side 0, sector ID 9 -- not in the first catalog sector.
	if info := disk.sectorRef(0, 0, trdInfoSector); info != nil {
		copy(info.Data[trdInfoLabel:trdInfoLabel+8], []byte("TESTDISK"))
		info.Data[trdInfoFreeSectors] = 50 // low byte of the free-sector word
		info.Data[trdInfoFreeSectors+1] = 0
		info.Data[trdInfoFileCount] = 12
	}

	// Get detailed disk information
	info := disk.GetDetailedInfo()

	// Verify basic information
	if info.Type != DiskTypeTRD {
		t.Errorf("Expected disk type TRD, got %s", info.Type)
	}

	if info.Tracks != 160 { // 80 cylinders, both sides
		t.Errorf("Expected 160 tracks, got %d", info.Tracks)
	}

	if info.Sectors != 2560 { // 80 cylinders * 2 sides * 16 sectors
		t.Errorf("Expected 2560 sectors, got %d", info.Sectors)
	}

	// Verify TR-DOS specific information
	if info.Name != "TESTDISK" {
		t.Errorf("Expected disk name 'TESTDISK', got '%s'", info.Name)
	}

	if info.FreeSectors != 50 {
		t.Errorf("Expected 50 free sectors, got %d", info.FreeSectors)
	}

	if info.FileCount != 12 {
		t.Errorf("Expected 12 files, got %d", info.FileCount)
	}

	t.Logf("Disk Info extraction works correctly:")
	t.Logf("   Name: %s, Type: %s, Tracks: %d, Sectors: %d",
		info.Name, info.Type, info.Tracks, info.Sectors)
	t.Logf("   Size: %d KB, Free: %d sectors, Files: %d",
		info.SizeKB, info.FreeSectors, info.FileCount)
}

// TestDiskRoundTripWithMetadata tests a complete disk save/load cycle with metadata
func TestDiskRoundTripWithMetadata(t *testing.T) {
	// Create and customize a disk
	disk1 := NewTRDisk()

	// Add identifiable data to test sectors (avoid sector 0 which contains TR-DOS metadata)
	testData1 := make([]byte, 256)
	copy(testData1, []byte("DISK TEST DATA v1.0 - IMPORTANT TRACK 5"))
	testData2 := make([]byte, 256)
	copy(testData2, []byte("ANOTHER TEST SECTOR 10 - VERIFY INTEGRITY"))

	err := disk1.WriteSector(5, 0, 1, testData1) // Track 5, sector 1
	if err != nil {
		t.Fatalf("Failed to write test data 1: %v", err)
	}

	err = disk1.WriteSector(10, 0, 1, testData2) // Track 10, sector 1
	if err != nil {
		t.Fatalf("Failed to write test data 2: %v", err)
	}

	// Add custom disk name to metadata
	if len(disk1.Tracks) > 0 && len(disk1.Tracks[0].Sectors) > 0 {
		sectorData := disk1.Tracks[0].Sectors[0].Data
		diskName := []byte("ROUNDTRIP")
		for i := 0; i < len(diskName) && i < 8; i++ {
			sectorData[9+i] = diskName[i]
		}
		sectorData[25] = 100 // 100 free sectors
	}

	// Save to buffer
	var buf bytes.Buffer
	err = SaveTRDisk(&buf, disk1)
	if err != nil {
		t.Fatalf("Failed to save disk: %v", err)
	}

	// Load from buffer
	disk2, err := LoadTRDisk(&buf)
	if err != nil {
		t.Fatalf("Failed to load disk: %v", err)
	}

	// Verify disk info matches (basic structure should be preserved)
	info1 := disk1.GetDetailedInfo()
	info2 := disk2.GetDetailedInfo()

	if info1.Tracks != info2.Tracks {
		t.Errorf("Track counts don't match: %d != %d",
			info1.Tracks, info2.Tracks)
	}

	if info1.SizeKB != info2.SizeKB {
		t.Errorf("Sizes don't match: %d KB != %d KB",
			info1.SizeKB, info2.SizeKB)
	}

	if info1.Sectors != info2.Sectors {
		t.Errorf("Sector counts don't match: %d != %d",
			info1.Sectors, info2.Sectors)
	}

	// Verify test data matches (avoid sector 0 which gets modified by TR-DOS save)
	readData1, err := disk2.ReadSector(5, 0, 1)
	if err != nil {
		t.Fatalf("Failed to read test sector 1: %v", err)
	}

	readData2, err := disk2.ReadSector(10, 0, 1)
	if err != nil {
		t.Fatalf("Failed to read test sector 2: %v", err)
	}

	if !bytes.Equal(readData1, testData1) {
		t.Errorf("Test data 1 doesn't match after round trip")
	}

	if !bytes.Equal(readData2, testData2) {
		t.Errorf("Test data 2 doesn't match after round trip")
	}

	t.Logf("Disk round-trip with metadata works correctly:")
	t.Logf("   Original: %s (%d KB, %d sectors)",
		info1.Name, info1.SizeKB, info1.Sectors)
	t.Logf("   Restored: %s (%d KB, %d sectors)",
		info2.Name, info2.SizeKB, info2.Sectors)

}

// TestMultipleSectorWrite tests writing multiple sectors to ensure consistency
func TestMultipleSectorWrite(t *testing.T) {
	disk := NewTRDisk()

	// Write to 3 different sectors (TR-DOS requires exactly 256 bytes per sector)
	testData1 := make([]byte, 256)
	copy(testData1, []byte("SECTOR 1"))
	testData2 := make([]byte, 256)
	copy(testData2, []byte("SECTOR 2"))
	testData3 := make([]byte, 256)
	copy(testData3, []byte("SECTOR 3"))

	err := disk.WriteSector(0, 0, 1, testData1)
	if err != nil {
		t.Fatalf("Failed to write sector 1: %v", err)
	}

	err = disk.WriteSector(1, 0, 1, testData2)
	if err != nil {
		t.Fatalf("Failed to write sector 2: %v", err)
	}

	err = disk.WriteSector(2, 0, 1, testData3)
	if err != nil {
		t.Fatalf("Failed to write sector 3: %v", err)
	}

	// Read back and verify
	readData1, err := disk.ReadSector(0, 0, 1)
	if err != nil {
		t.Fatalf("Failed to read sector 1: %v", err)
	}

	readData2, err := disk.ReadSector(1, 0, 1)
	if err != nil {
		t.Fatalf("Failed to read sector 2: %v", err)
	}

	readData3, err := disk.ReadSector(2, 0, 1)
	if err != nil {
		t.Fatalf("Failed to read sector 3: %v", err)
	}

	if !bytes.Equal(readData1, testData1) {
		t.Errorf("Sector 1 data mismatch")
	}

	if !bytes.Equal(readData2, testData2) {
		t.Errorf("Sector 2 data mismatch")
	}

	if !bytes.Equal(readData3, testData3) {
		t.Errorf("Sector 3 data mismatch")
	}

	t.Logf("Multiple sector write/read test passed")
}

// TestDiskLargeData tests handling of larger data sets
func TestDiskLargeData(t *testing.T) {
	disk := NewTRDisk()

	// Create data that fills a whole sector (256 bytes)
	largeData := make([]byte, 256)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	err := disk.WriteSector(0, 0, 1, largeData)
	if err != nil {
		t.Fatalf("Failed to write large sector: %v", err)
	}

	readData, err := disk.ReadSector(0, 0, 1)
	if err != nil {
		t.Fatalf("Failed to read large sector: %v", err)
	}

	if !bytes.Equal(readData, largeData) {
		t.Error("Large sector data mismatch")
	}

	// Test writing a smaller dataset padded to sector size
	partialData := make([]byte, 256)
	copy(partialData, largeData[50:100]) // Try to write just 50 bytes

	err = disk.WriteSector(1, 0, 1, partialData)
	if err != nil {
		t.Fatalf("Failed to write partial data: %v", err)
	}

	readPartial, err := disk.ReadSector(1, 0, 1)
	if err != nil {
		t.Fatalf("Failed to read partial data: %v", err)
	}

	// Verify first part matches partial data
	expectedPartial := make([]byte, 256)
	copy(expectedPartial, largeData[50:100])

	if !bytes.Equal(readPartial, expectedPartial) {
		t.Error("Partial data mismatch")
	}

	// Verify rest is zeros
	for i := 50; i < len(readPartial); i++ {
		if readPartial[i] != 0 {
			t.Errorf("Expected zeros from offset %d, got %d", i, readPartial[i])
		}
	}

	t.Logf("Large data handling test passed")
}