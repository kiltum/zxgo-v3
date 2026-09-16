package media

// AlternativeDSKFormats contains signatures for non-CPC DSK formats
// These formats use .dsk extension but are different disk formats
var AlternativeDSKFormats = []struct {
	Name      string
	Signature []byte
	MinSize   int
}{
	{
		Name:      "SAMDOS2",
		Signature: []byte{0x13, 0x73, 0x61, 0x6d, 0x64, 0x6f, 0x73, 0x32}, // 0x13 samdos2 (CP/M end-of-text byte)
		MinSize:   512,
	},
	{
		Name:      "SAMDOS",
		Signature: []byte{0xd3, 0x73, 0x61, 0x6d, 0x64, 0x6f, 0x73}, // 0xD3 samdos (alternative samdos format)
		MinSize:   512,
	},
	{
		Name:      "MDOS42",
		Signature: []byte{0x13, 0x4d, 0x44, 0x4f, 0x53, 0x34, 0x32}, // 0x13 MDOS42 (CP/M end-of-text byte)
		MinSize:   512,
	},
	{
		Name:      "MDOS21",
		Signature: []byte{0x13, 0x6d, 0x64, 0x6f, 0x73, 0x32, 0x31}, // 0x13 mdos21 (CP/M end-of-text byte)
		MinSize:   512,
	},
	{
		Name:      "FRED",
		Signature: []byte{0x13, 0x46, 0x52, 0x45, 0x44}, // 0x13 FRED (CP/M magazine disks)
		MinSize:   16,
	},
	{
		Name:      "FRED_ISSUE",
		Signature: []byte{0x13, 0x7f, 0x20, 0x46, 0x72, 0x65, 0x64}, // 0x13, 0x7F, then " Fred" (FRED magazine issues)
		MinSize:   16,
	},
	{
		Name:      "QDOS",
		Signature: []byte{0x13, 0x51, 0x44, 0x4f, 0x53}, // 0x13 QDOS (alternative DOS format)
		MinSize:   16,
	},
	{
		Name:      "ZIP",
		Signature: []byte{0x50, 0x4b, 0x03, 0x04}, // ZIP file signature
		MinSize:   22,
	},
	{
		Name:      "RAR",
		Signature: []byte{0x52, 0x61, 0x72, 0x21}, // RAR file signature
		MinSize:   7,
	},
	{
		Name:      "GZ",
		Signature: []byte{0x1f, 0x8b}, // GZIP file signature
		MinSize:   10,
	},
}

// containsSub checks if a string contains a substring at the beginning
func containsSub(s, sub string) bool {
	return len(s) >= len(sub) && s[:len(sub)] == sub
}

// min helper (note: min is defined in tzx.go, avoiding duplicate by keeping this file's functions using containsSub)
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// CheckForAlternativePatterns does flexible pattern matching for alternative formats
func CheckForAlternativePatterns(data []byte) string {
	if len(data) < 16 {
		return ""
	}

	sig := string(data[0:minInt(16, len(data))])

	// Check for various SAMDOS/MDOS patterns with different leading bytes
	if containsSub(sig, "samdos") || containsSub(sig, "SAMDOS") {
		if containsSub(sig, "2") {
			return "SAMDOS2"
		}
		return "SAMDOS"
	}

	if containsSub(sig, "mdos") || containsSub(sig, "MDOS") {
		if containsSub(sig, "42") || containsSub(sig, "2") {
			return "MDOS42" // or MDOS21
		}
		return "MDOS"
	}

	// Check for FRED magazine patterns
	if containsSub(sig, "FRED") || containsSub(sig, "fred") {
		return "FRED"
	}

	return ""
}

// DetectAlternativeDSKFormat checks if a file is an alternative DSK format
func DetectAlternativeDSKFormat(data []byte) string {
	if len(data) < 16 {
		return ""
	}

	// First try exact signature match
	for _, alt := range AlternativeDSKFormats {
		if len(data) >= len(alt.Signature) {
			found := true
			for i, sigByte := range alt.Signature {
				if data[i] != sigByte {
					found = false
					break
				}
			}
			if found {
				return alt.Name
			}
		}
	}

	// If no exact match, try flexible pattern matching
	patternResult := CheckForAlternativePatterns(data)
	if patternResult != "" {
		return patternResult
	}

	// Additional flexible matching for common alternative format patterns
	sig := string(data[0:minInt(16, len(data))])

	// Check for SAMDOS variants with various leading bytes
	// Common leading bytes: 0x13 (CP/M symbol), 0xd3 (0xD3 ), 0xc9, 0x89, etc.
	for i := 0; i < len(sig) && i < 6; i++ {
		remaining := sig[i:]
		if len(remaining) >= 6 && (remaining[:6] == "samdos" || remaining[:6] == "SAMDOS") {
			if len(remaining) >= 7 && remaining[6] == '2' {
				return "SAMDOS2"
			}
			return "SAMDOS"
		}
	}

	// Check for MDOS variants
	for i := 0; i < len(sig) && i < 6; i++ {
		remaining := sig[i:]
		if len(remaining) >= 4 && (remaining[:4] == "mdos" || remaining[:4] == "MDOS") {
			if len(remaining) >= 6 && remaining[4:6] == "42" {
				return "MDOS42"
			}
			if len(remaining) >= 6 && remaining[4:6] == "21" {
				return "MDOS21"
			}
			return "MDOS"
		}
	}

	// Check for FRED magazine disks with various patterns
	for i := 0; i < len(sig) && i < 4; i++ {
		remaining := sig[i:]
		if len(remaining) >= 4 && (remaining[:4] == "FRED" || remaining[:4] == "fred") {
			return "FRED"
		}
	}

	return ""
}
