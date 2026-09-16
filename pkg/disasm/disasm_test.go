package disasm

import "testing"

// TestDecodeUnprefixed pins a representative sample of unprefixed instructions.
func TestDecodeUnprefixed(t *testing.T) {
	d := New()
	cases := []struct {
		bytes []byte
		mnem  string
		len   int
	}{
		{[]byte{0x00}, "NOP", 1},
		{[]byte{0x04}, "INC B", 1},
		{[]byte{0x06, 0x42}, "LD B, $42", 2},
		{[]byte{0x01, 0x34, 0x12}, "LD BC, $1234", 3},
		{[]byte{0x21, 0x00, 0x80}, "LD HL, $8000", 3},
	}
	for _, c := range cases {
		ins, err := d.Decode(c.bytes)
		if err != nil {
			t.Fatalf("Decode(% X) error: %v", c.bytes, err)
		}
		if ins.Mnemonic != c.mnem {
			t.Errorf("Decode(% X) = %q, want %q", c.bytes, ins.Mnemonic, c.mnem)
		}
		if ins.Length != c.len {
			t.Errorf("Decode(% X) length = %d, want %d", c.bytes, ins.Length, c.len)
		}
	}
}

// TestDecodePrefixes pins CB / ED / DD / FD instructions.
func TestDecodePrefixes(t *testing.T) {
	d := New()
	cases := []struct {
		bytes []byte
		mnem  string
		len   int
	}{
		{[]byte{0xCB, 0x00}, "RLC B", 2},
		{[]byte{0xCB, 0x46}, "BIT 0, (HL)", 2},
		{[]byte{0xED, 0x40}, "IN B, (C)", 2},
		{[]byte{0xED, 0x43, 0x34, 0x12}, "LD ($1234), BC", 4},
		{[]byte{0xDD, 0x09}, "ADD IX, BC", 2},
		{[]byte{0xDD, 0x21, 0x34, 0x12}, "LD IX, $1234", 4},
		{[]byte{0xDD, 0x23}, "INC IX", 2},
		{[]byte{0xFD, 0x09}, "ADD IY, BC", 2},
		{[]byte{0xFD, 0x21, 0x34, 0x12}, "LD IY, $1234", 4},
		{[]byte{0xFD, 0x23}, "INC IY", 2},
	}
	for _, c := range cases {
		ins, err := d.Decode(c.bytes)
		if err != nil {
			t.Fatalf("Decode(% X) error: %v", c.bytes, err)
		}
		if ins.Mnemonic != c.mnem {
			t.Errorf("Decode(% X) = %q, want %q", c.bytes, ins.Mnemonic, c.mnem)
		}
		if ins.Length != c.len {
			t.Errorf("Decode(% X) length = %d, want %d", c.bytes, ins.Length, c.len)
		}
	}
}

// TestDDFDSymmetry verifies the DD and FD decoders produce identical mnemonics
// apart from the register name, for every opcode (including DDCB/FDCB). This is
// the regression net for the shared decodeIndexed body: decode_fd.go was
// previously a ~780-line copy that could drift from decode_dd.go.
func TestDDFDSymmetry(t *testing.T) {
	d := New()
	for op := 0; op < 256; op++ {
		dd, errD := d.Decode([]byte{0xDD, byte(op), 0x00, 0x00})
		fd, errF := d.Decode([]byte{0xFD, byte(op), 0x00, 0x00})
		if errD != nil || errF != nil {
			t.Fatalf("opcode %02X: unexpected error (dd=%v, fd=%v)", op, errD, errF)
		}
		if dd.Length != fd.Length {
			t.Errorf("opcode %02X: length dd=%d fd=%d", op, dd.Length, fd.Length)
		}
		if want := fdMnemonic(dd.Mnemonic); fd.Mnemonic != want {
			t.Errorf("opcode %02X: fd mnemonic %q, want %q (from dd %q)", op, fd.Mnemonic, want, dd.Mnemonic)
		}
	}
}

// TestDecodeShortInput verifies truncated buffers do not panic (the decoders
// index data[0..3] directly). Empty input is an error; short input is padded.
func TestDecodeShortInput(t *testing.T) {
	d := New()
	if _, err := d.Decode(nil); err == nil {
		t.Error("Decode(nil) should return an error, not panic")
	}
	// These must not panic (they would before the padding fix).
	for _, b := range [][]byte{
		{0xCB},
		{0xDD},
		{0xED},
		{0xFD},
		{0x01},       // LD BC, nn truncated to its opcode
		{0xDD, 0x21}, // LD IX, nn truncated
		{0xED, 0x43}, // LD (nn), BC truncated
	} {
		if ins, err := d.Decode(b); err != nil {
			t.Errorf("Decode(% X) unexpectedly errored: %v", b, err)
		} else if ins == nil {
			t.Errorf("Decode(% X) returned nil instruction", b)
		}
	}
}
