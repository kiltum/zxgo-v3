package state

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// The fakes stand in for the two shapes a real chunk takes: the emulator's own
// clock, and a device with a mixed payload. They are deliberately small - what
// is being tested here is the container, not the machine.

// fakeClock is the shape IDEmulator has: a few integers and a flag.
type fakeClock struct {
	totalTicks int64
	frameCount int64
	lastInt    int64
	fastTape   bool
}

func (c *fakeClock) SaveState(e *Encoder) error {
	e.I64(c.totalTicks)
	e.I64(c.frameCount)
	e.I64(c.lastInt)
	e.Bool(c.fastTape)
	return nil
}

func (c *fakeClock) LoadState(d *Decoder) error {
	// Parse fully, then assign: a payload that stops early must leave c alone.
	ticks, frames, lastInt := d.I64(), d.I64(), d.I64()
	fast := d.Bool()
	if err := d.Err(); err != nil {
		return err
	}
	c.totalTicks, c.frameCount, c.lastInt, c.fastTape = ticks, frames, lastInt, fast
	return nil
}

// fakeDevice touches every primitive the encoder offers, so the codec is
// covered by one round trip rather than one test per type.
type fakeDevice struct {
	a, f     uint8
	pc, sp   uint16
	wide     uint32
	wider    uint64
	signed   int64
	signedLo int32
	fx       float32
	fy       float64
	halted   bool
	banks    [][]byte
	names    []string
	words    []uint16
	selected uint8
}

func (v *fakeDevice) SaveState(e *Encoder) error {
	e.U8(v.a)
	e.U8(v.f)
	e.U16(v.pc)
	e.U16(v.sp)
	e.U32(v.wide)
	e.U64(v.wider)
	e.I64(v.signed)
	e.I32(v.signedLo)
	e.F32(v.fx)
	e.F64(v.fy)
	e.Bool(v.halted)
	e.U8(v.selected)
	// A slice of blobs is a count and a loop, not an encoder method: there is
	// no way to spell "a slice of arbitrary byte slices" that keeps the file
	// readable, and every component's loop is three lines.
	e.U32(uint32(len(v.banks)))
	for _, b := range v.banks {
		e.Bytes(b)
	}
	e.Strings(v.names)
	e.U16s(v.words)
	return nil
}

func (v *fakeDevice) LoadState(d *Decoder) error {
	var out fakeDevice
	out.a = d.U8()
	out.f = d.U8()
	out.pc = d.U16()
	out.sp = d.U16()
	out.wide = d.U32()
	out.wider = d.U64()
	out.signed = d.I64()
	out.signedLo = d.I32()
	out.fx = d.F32()
	out.fy = d.F64()
	out.halted = d.Bool()
	out.selected = d.U8()
	n := d.count(4)
	for i := 0; i < n && d.Err() == nil; i++ {
		// Bytes aliases the payload, so a component that keeps it copies it.
		// Clone keeps an empty blob empty rather than nil, which DeepEqual in
		// these tests would otherwise see as a different value.
		out.banks = append(out.banks, bytes.Clone(d.Bytes()))
	}
	out.names = d.Strings()
	out.words = d.U16s()
	if err := d.Err(); err != nil {
		return err
	}
	*v = out
	return nil
}

func sampleDevice() *fakeDevice {
	return &fakeDevice{
		a: 0x12, f: 0xC1, pc: 0x8000, sp: 0xFFFE,
		wide: 0xDEADBEEF, wider: 0x0123456789ABCDEF, signed: -1234567, signedLo: -4242,
		fx: 3.5, fy: -0.125, halted: true, selected: 7,
		banks: [][]byte{{1, 2, 3}, {}, {9, 8, 7, 6}},
		names: []string{"one", "", "three"},
		words: []uint16{0, 1, 0xFFFF},
	}
}

// specsFor builds the table the coordinator would: an emulator chunk and a
// device chunk, in load order.
func specsFor(clock *fakeClock, dev *fakeDevice) []Spec {
	return []Spec{
		Section(IDEmulator, 1, true, clock),
		Section(IDCPU, 1, true, dev),
	}
}

func saveFile(t *testing.T, h Header, specs []Spec, opts Options) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := Save(&buf, h, specs, opts); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return buf.Bytes()
}

func testHeader() Header {
	return Header{CPUHz: 3546900, Model: "test", Build: "test-build"}
}

func TestRoundTrip(t *testing.T) {
	clock := &fakeClock{totalTicks: 1234567, frameCount: 42, lastInt: 1234500, fastTape: true}
	dev := sampleDevice()
	data := saveFile(t, testHeader(), specsFor(clock, dev), Options{})

	gotClock, gotDev := &fakeClock{}, &fakeDevice{}
	res, err := Load(bytes.NewReader(data), "test", specsFor(gotClock, gotDev))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(clock, gotClock) {
		t.Errorf("clock = %+v, want %+v", gotClock, clock)
	}
	if !reflect.DeepEqual(dev, gotDev) {
		t.Errorf("device = %+v, want %+v", gotDev, dev)
	}
	if res.Header != testHeader() {
		t.Errorf("header = %+v, want %+v", res.Header, testHeader())
	}
	want := []ID{IDEmulator, IDCPU}
	if !reflect.DeepEqual(res.Applied, want) {
		t.Errorf("applied = %v, want %v", res.Applied, want)
	}
	if len(res.Skipped) != 0 || len(res.Defaulted) != 0 {
		t.Errorf("skipped = %v, defaulted = %v, want neither", res.Skipped, res.Defaulted)
	}
}

func TestRoundTripDeflated(t *testing.T) {
	// Repeating bytes compress, so a deflated file is measurably smaller - the
	// point of the flag is session size, and a flag that did nothing would
	// still round-trip.
	dev := sampleDevice()
	dev.banks = [][]byte{bytes.Repeat([]byte{0xAA}, 8192)}
	clock := &fakeClock{}

	plain := saveFile(t, testHeader(), specsFor(clock, dev), Options{})
	deflated := saveFile(t, testHeader(), specsFor(clock, dev), Options{Deflate: true})
	if len(deflated) >= len(plain) {
		t.Errorf("deflated file is %d bytes, plain is %d: the flag did nothing", len(deflated), len(plain))
	}
	if flags := binary.LittleEndian.Uint16(deflated[10:]); flags&flagDeflate == 0 {
		t.Errorf("header flags = %#04x, want the deflate bit set", flags)
	}

	gotClock, gotDev := &fakeClock{}, &fakeDevice{}
	if _, err := Load(bytes.NewReader(deflated), "test", specsFor(gotClock, gotDev)); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(dev, gotDev) {
		t.Errorf("device survived deflate as %+v, want %+v", gotDev, dev)
	}
}

func TestUnknownChunkIsSkipped(t *testing.T) {
	clock := &fakeClock{totalTicks: 5}
	dev := sampleDevice()
	// A file written by a build that has one more device than this one does.
	extra := &fakeClock{totalTicks: 99}
	writer := append(specsFor(clock, dev), Section(IDGeneralSound, 1, false, extra))
	data := saveFile(t, testHeader(), writer, Options{})

	gotClock, gotDev := &fakeClock{}, &fakeDevice{}
	res, err := Load(bytes.NewReader(data), "test", specsFor(gotClock, gotDev))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(res.Skipped, []ID{IDGeneralSound}) {
		t.Errorf("skipped = %v, want [general-sound]", res.Skipped)
	}
	if clock.totalTicks != gotClock.totalTicks || !reflect.DeepEqual(dev, gotDev) {
		t.Errorf("known chunks did not load: clock %+v device %+v", gotClock, gotDev)
	}
}

func TestChunkVersionNewerIsRefused(t *testing.T) {
	clock := &fakeClock{}
	data := saveFile(t, testHeader(), specsFor(clock, &fakeDevice{}), Options{})

	// This build speaks chunk id 2 at version 1; the file claims 2. A component
	// cannot know what a field it has never seen means, so this is the one
	// version mismatch that is not a skip and not a migration.
	reader := []Spec{
		Section(IDEmulator, 1, true, &fakeClock{}),
		Section(IDCPU, 1, true, &fakeDevice{}),
	}
	patched := patchChunkVersion(t, data, 1, 2)
	if _, err := Load(bytes.NewReader(patched), "test", reader); !errors.Is(err, ErrChunkVersion) {
		t.Fatalf("err = %v, want ErrChunkVersion", err)
	}
}

func TestChunkVersionOlderMigrates(t *testing.T) {
	// A writer that stored two fields, read by a component that has since grown
	// a third: the older payload must load with the new field defaulted.
	old := &oldShape{a: 0x77, b: 0x1234}
	data := saveFile(t, testHeader(), []Spec{Section(IDCPU, 1, true, old)}, Options{})

	newer := &newShape{}
	spec := Section(IDCPU, 2, true, newer)
	if _, err := Load(bytes.NewReader(data), "test", []Spec{spec}); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if newer.a != 0x77 || newer.b != 0x1234 {
		t.Errorf("migrated = %+v, want the two old fields", newer)
	}
	if newer.added != 0x99 {
		t.Errorf("added = %#x, want the default 0x99", newer.added)
	}
}

// oldShape is version 1 of a component: two fields.
type oldShape struct {
	a uint8
	b uint16
}

func (o *oldShape) SaveState(e *Encoder) error { e.U8(o.a); e.U16(o.b); return nil }
func (o *oldShape) LoadState(d *Decoder) error {
	a, b := d.U8(), d.U16()
	if err := d.Err(); err != nil {
		return err
	}
	o.a, o.b = a, b
	return nil
}

// newShape is version 2: the same two, plus one appended. It fills the new
// field's default when the payload is older than it is, which is the rule the
// format relies on rather than a fixed layout.
type newShape struct {
	a, added uint8
	b        uint16
}

func (n *newShape) SaveState(e *Encoder) error {
	e.U8(n.a)
	e.U16(n.b)
	e.U8(n.added)
	return nil
}

func (n *newShape) LoadState(d *Decoder) error {
	a, b, added := d.U8(), d.U16(), uint8(0x99)
	if d.Version() >= 2 && d.More() {
		added = d.U8()
	}
	if err := d.Err(); err != nil {
		return err
	}
	n.a, n.b, n.added = a, b, added
	return nil
}

func TestMissingRequiredChunkIsRefused(t *testing.T) {
	// A file with only the emulator chunk, read by a machine that requires both.
	data := saveFile(t, testHeader(), []Spec{Section(IDEmulator, 1, true, &fakeClock{})}, Options{})
	specs := specsFor(&fakeClock{}, &fakeDevice{})
	if _, err := Load(bytes.NewReader(data), "test", specs); !errors.Is(err, ErrMissingChunk) {
		t.Fatalf("err = %v, want ErrMissingChunk", err)
	}
}

func TestMissingOptionalChunkIsDefaulted(t *testing.T) {
	data := saveFile(t, testHeader(), []Spec{Section(IDEmulator, 1, true, &fakeClock{})}, Options{})
	specs := []Spec{
		Section(IDEmulator, 1, true, &fakeClock{}),
		Section(IDCPU, 1, false, &fakeDevice{}),
	}
	res, err := Load(bytes.NewReader(data), "test", specs)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(res.Defaulted, []ID{IDCPU}) {
		t.Errorf("defaulted = %v, want [cpu]", res.Defaulted)
	}
}

func TestModelMismatchIsRefused(t *testing.T) {
	data := saveFile(t, testHeader(), specsFor(&fakeClock{}, &fakeDevice{}), Options{})
	if _, err := Load(bytes.NewReader(data), "pentagon512", specsFor(&fakeClock{}, &fakeDevice{})); !errors.Is(err, ErrModel) {
		t.Fatalf("err = %v, want ErrModel", err)
	}
}

func TestCorruptChunkIsRefused(t *testing.T) {
	data := saveFile(t, testHeader(), specsFor(&fakeClock{}, &fakeDevice{}), Options{})
	// Flip one payload bit, leaving every length and offset intact, so the only
	// thing that can catch it is the checksum.
	data[len(data)-1] ^= 0x01
	if _, err := Load(bytes.NewReader(data), "test", specsFor(&fakeClock{}, &fakeDevice{})); !errors.Is(err, ErrCRC) {
		t.Fatalf("err = %v, want ErrCRC", err)
	}
}

func TestRefusalAppliesNothing(t *testing.T) {
	// The whole point of the envelope pass: a file whose *last* chunk is corrupt
	// must not leave the first chunk applied. The clock here starts from a known
	// value and has to still hold it afterwards.
	clock := &fakeClock{}
	dev := sampleDevice()
	data := saveFile(t, testHeader(), specsFor(clock, dev), Options{})
	data[len(data)-1] ^= 0xFF

	clock.totalTicks = 777
	got := &fakeClock{}
	_, err := Load(bytes.NewReader(data), "test", specsFor(got, &fakeDevice{}))
	if !errors.Is(err, ErrCRC) {
		t.Fatalf("err = %v, want ErrCRC", err)
	}
	if got.totalTicks != 0 {
		t.Errorf("totalTicks = %d, want 0: a refused load must not apply part of the file", got.totalTicks)
	}
	if clock.totalTicks != 777 {
		t.Errorf("the source machine was modified: %d", clock.totalTicks)
	}
}

func TestComponentRefusalLeavesItUntouched(t *testing.T) {
	// A payload that decodes some fields and then stops. The component must not
	// have taken the fields it did read.
	payload := NewEncoder()
	payload.I64(1)
	payload.I64(2)
	// The third I64 and the Bool are missing.
	data := buildFile(t, "test", testHeader(), nil, []rawChunk{
		{id: IDEmulator, version: 1, payload: payload.Payload()},
	})

	got := &fakeClock{totalTicks: 4242}
	_, err := Load(bytes.NewReader(data), "test", []Spec{Section(IDEmulator, 1, true, got)})
	if !errors.Is(err, ErrLoad) {
		t.Fatalf("err = %v, want ErrLoad", err)
	}
	if got.totalTicks != 4242 {
		t.Errorf("totalTicks = %d, want the pre-load 4242", got.totalTicks)
	}
}

func TestNotAStateFile(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"short", []byte("ZXGOS")},
		{"wrong magic", []byte("PK\x03\x04whatever")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Load(bytes.NewReader(tc.data), "test", specsFor(&fakeClock{}, &fakeDevice{})); !errors.Is(err, ErrNotState) {
				t.Fatalf("err = %v, want ErrNotState", err)
			}
		})
	}
}

func TestHeaderRefusals(t *testing.T) {
	base := saveFile(t, testHeader(), specsFor(&fakeClock{}, &fakeDevice{}), Options{})

	t.Run("newer format version", func(t *testing.T) {
		data := append([]byte(nil), base...)
		binary.LittleEndian.PutUint16(data[8:], FormatVersion+1)
		if _, err := Load(bytes.NewReader(data), "test", specsFor(&fakeClock{}, &fakeDevice{})); !errors.Is(err, ErrFormatVersion) {
			t.Fatalf("err = %v, want ErrFormatVersion", err)
		}
	})

	t.Run("unknown header flag", func(t *testing.T) {
		data := append([]byte(nil), base...)
		binary.LittleEndian.PutUint16(data[10:], 0x8000)
		if _, err := Load(bytes.NewReader(data), "test", specsFor(&fakeClock{}, &fakeDevice{})); !errors.Is(err, ErrUnknownFlag) {
			t.Fatalf("err = %v, want ErrUnknownFlag", err)
		}
	})

	t.Run("implausible chunk count", func(t *testing.T) {
		data := append([]byte(nil), base...)
		binary.LittleEndian.PutUint32(data[16:], MaxChunks+1)
		if _, err := Load(bytes.NewReader(data), "test", specsFor(&fakeClock{}, &fakeDevice{})); !errors.Is(err, ErrTooManyChunks) {
			t.Fatalf("err = %v, want ErrTooManyChunks", err)
		}
	})

	t.Run("chunk count does not match the body", func(t *testing.T) {
		data := append([]byte(nil), base...)
		binary.LittleEndian.PutUint32(data[16:], 7)
		if _, err := Load(bytes.NewReader(data), "test", specsFor(&fakeClock{}, &fakeDevice{})); !errors.Is(err, ErrChunkCount) {
			t.Fatalf("err = %v, want ErrChunkCount", err)
		}
	})

	t.Run("truncated", func(t *testing.T) {
		data := base[:len(base)-3]
		if _, err := Load(bytes.NewReader(data), "test", specsFor(&fakeClock{}, &fakeDevice{})); !errors.Is(err, ErrTruncated) {
			t.Fatalf("err = %v, want ErrTruncated", err)
		}
	})
}

func TestChunkEnvelopeRefusals(t *testing.T) {
	payload := NewEncoder()
	payload.I64(1)

	t.Run("chunk longer than a chunk may be", func(t *testing.T) {
		over := uint32(MaxChunkSize + 1)
		data := buildFile(t, "test", testHeader(), nil, []rawChunk{
			{id: IDEmulator, version: 1, payload: payload.Payload(), length: &over},
		})
		if _, err := Load(bytes.NewReader(data), "test", []Spec{Section(IDEmulator, 1, true, &fakeClock{})}); !errors.Is(err, ErrChunkTooLarge) {
			t.Fatalf("err = %v, want ErrChunkTooLarge", err)
		}
	})

	t.Run("unknown chunk flag", func(t *testing.T) {
		data := buildFile(t, "test", testHeader(), nil, []rawChunk{
			{id: IDEmulator, version: 1, flags: 0x0002, payload: payload.Payload()},
		})
		if _, err := Load(bytes.NewReader(data), "test", []Spec{Section(IDEmulator, 1, true, &fakeClock{})}); !errors.Is(err, ErrUnknownFlag) {
			t.Fatalf("err = %v, want ErrUnknownFlag", err)
		}
	})

	t.Run("duplicate chunk id", func(t *testing.T) {
		data := buildFile(t, "test", testHeader(), nil, []rawChunk{
			{id: IDEmulator, version: 1, payload: payload.Payload()},
			{id: IDEmulator, version: 1, payload: payload.Payload()},
		})
		if _, err := Load(bytes.NewReader(data), "test", []Spec{Section(IDEmulator, 1, true, &fakeClock{})}); !errors.Is(err, ErrDuplicateChunk) {
			t.Fatalf("err = %v, want ErrDuplicateChunk", err)
		}
	})

	t.Run("chunk stops short of its declared length", func(t *testing.T) {
		long := uint32(64)
		data := buildFile(t, "test", testHeader(), nil, []rawChunk{
			{id: IDEmulator, version: 1, payload: payload.Payload(), length: &long},
		})
		if _, err := Load(bytes.NewReader(data), "test", []Spec{Section(IDEmulator, 1, true, &fakeClock{})}); !errors.Is(err, ErrTruncated) {
			t.Fatalf("err = %v, want ErrTruncated", err)
		}
	})
}

func TestBadSpecTable(t *testing.T) {
	clock := &fakeClock{}
	t.Run("duplicate id", func(t *testing.T) {
		specs := []Spec{
			Section(IDEmulator, 1, true, clock),
			Section(IDEmulator, 1, false, &fakeClock{}),
		}
		if err := Save(&bytes.Buffer{}, testHeader(), specs, Options{}); !errors.Is(err, ErrBadSpec) {
			t.Fatalf("Save err = %v, want ErrBadSpec", err)
		}
		if _, err := Load(bytes.NewReader(nil), "test", specs); !errors.Is(err, ErrBadSpec) {
			t.Fatalf("Load err = %v, want ErrBadSpec", err)
		}
	})
	t.Run("nil codec", func(t *testing.T) {
		if err := Save(&bytes.Buffer{}, testHeader(), []Spec{{ID: IDCPU, Version: 1}}, Options{}); !errors.Is(err, ErrBadSpec) {
			t.Fatalf("Save err = %v, want ErrBadSpec", err)
		}
		if _, err := Load(bytes.NewReader(nil), "test", []Spec{{ID: IDCPU, Version: 1}}); !errors.Is(err, ErrBadSpec) {
			t.Fatalf("Load err = %v, want ErrBadSpec", err)
		}
	})
}

func TestDecoderBounds(t *testing.T) {
	// Every one of these is a length field lying about how much is there. The
	// decoder must refuse rather than allocate.
	enc := NewEncoder()
	enc.U32(1 << 30)
	blob := NewDecoder(enc.Payload(), 1)
	if b := blob.Bytes(); b != nil || blob.Err() == nil {
		t.Errorf("a blob claiming 1 GiB was accepted: %v", blob.Err())
	}

	enc.Reset()
	enc.U32(1 << 28)
	slice := NewDecoder(enc.Payload(), 1)
	if v := slice.U64s(); v != nil || slice.Err() == nil {
		t.Errorf("a slice claiming 2^28 elements was accepted: %v", slice.Err())
	}

	// A read past the end, and the sticky error that keeps the rest of a
	// component's reads from reporting anything else.
	short := NewDecoder([]byte{1, 2}, 1)
	if got := short.U8(); got != 1 {
		t.Errorf("first byte = %d, want 1", got)
	}
	_ = short.U32()
	if !short.Failed() {
		t.Error("reading four bytes from a two-byte payload did not fail")
	}
	if short.U8() != 0 {
		t.Error("a read after the failure returned data")
	}
}

func TestKnownIDs(t *testing.T) {
	if !Known(IDCPU) {
		t.Error("IDCPU should be known")
	}
	if Known(ID(9999)) {
		t.Error("id 9999 should not be known")
	}
	if got := ID(9999).String(); got != "chunk-9999" {
		t.Errorf("String() = %q, want chunk-9999", got)
	}
	if got := IDTurboSound.String(); got != "turbosound" {
		t.Errorf("String() = %q, want turbosound", got)
	}
}

// --- fixture -------------------------------------------------------------

// fixturePath is the committed file that pins the on-disk format. The format is
// a contract with every session a user has already saved, so a change that
// breaks it should fail the build rather than a user's restore.
const fixturePath = "testdata/state_v1.zxstate"

// TestFixtureV1 reads the committed file and checks it against the values it was
// written with. Regenerate it with:
//
//	ZXGO_STATE_WRITE_FIXTURE=1 go test ./pkg/state/ -run TestFixtureV1
func TestFixtureV1(t *testing.T) {
	if os.Getenv("ZXGO_STATE_WRITE_FIXTURE") == "1" {
		if err := os.MkdirAll(filepath.Dir(fixturePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fixturePath, fixtureBytes(t), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", fixturePath)
		return
	}

	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("%s: %v (regenerate with ZXGO_STATE_WRITE_FIXTURE=1)", fixturePath, err)
	}
	clock, dev := &fakeClock{}, &fakeDevice{}
	res, err := Load(bytes.NewReader(data), "test", specsFor(clock, dev))
	if err != nil {
		t.Fatalf("the committed fixture no longer loads: %v", err)
	}
	// Values a v1 writer produced. If the format changes, these are what a user
	// restoring an existing session would have got instead.
	if clock.totalTicks != 987654321 || clock.frameCount != 1234 || clock.lastInt != 987650000 || !clock.fastTape {
		t.Errorf("clock = %+v, want the v1 values", clock)
	}
	if !reflect.DeepEqual(dev, fixtureDevice()) {
		t.Errorf("device = %+v, want %+v", dev, fixtureDevice())
	}
	if res.Header.CPUHz != 3500000 || res.Header.Model != "test" || res.Header.Build != "fixture-v1" {
		t.Errorf("header = %+v, want the v1 header", res.Header)
	}
}

func fixtureDevice() *fakeDevice {
	d := sampleDevice()
	d.banks = [][]byte{{0xDE, 0xAD}, {0xBE, 0xEF, 0x00}}
	d.names = []string{"fixture"}
	d.words = []uint16{42, 43}
	return d
}

func fixtureBytes(t *testing.T) []byte {
	t.Helper()
	clock := &fakeClock{totalTicks: 987654321, frameCount: 1234, lastInt: 987650000, fastTape: true}
	h := Header{CPUHz: 3500000, Model: "test", Build: "fixture-v1"}
	return saveFile(t, h, specsFor(clock, fixtureDevice()), Options{})
}

// --- helpers for building malformed files --------------------------------

// rawChunk is one chunk written by hand, with any field able to lie, so a
// malformed file is spelled out where it is used instead of being produced by
// patching offsets in a valid one.
type rawChunk struct {
	id      ID
	version uint16
	flags   uint16
	payload []byte
	length  *uint32 // nil = the payload's real length
	crc     *uint32 // nil = the payload's real checksum
}

// buildFile writes a file from raw chunks. declaredChunks nil means the real
// count.
func buildFile(t *testing.T, model string, h Header, declaredChunks *uint32, chunks []rawChunk) []byte {
	t.Helper()
	var buf bytes.Buffer
	var fixed [headerFixedSize]byte
	copy(fixed[0:8], Magic)
	binary.LittleEndian.PutUint16(fixed[8:], FormatVersion)
	binary.LittleEndian.PutUint32(fixed[12:], uint32(h.CPUHz))
	count := uint32(len(chunks))
	if declaredChunks != nil {
		count = *declaredChunks
	}
	binary.LittleEndian.PutUint32(fixed[16:], count)
	buf.Write(fixed[:])
	for _, s := range []string{model, h.Build} {
		var n [4]byte
		binary.LittleEndian.PutUint32(n[:], uint32(len(s)))
		buf.Write(n[:])
		buf.WriteString(s)
	}
	for _, c := range chunks {
		length := uint32(len(c.payload))
		if c.length != nil {
			length = *c.length
		}
		sum := crc32.ChecksumIEEE(c.payload)
		if c.crc != nil {
			sum = *c.crc
		}
		var env [chunkHeaderSize]byte
		binary.LittleEndian.PutUint32(env[0:], uint32(c.id))
		binary.LittleEndian.PutUint16(env[4:], c.version)
		binary.LittleEndian.PutUint16(env[6:], c.flags)
		binary.LittleEndian.PutUint32(env[8:], length)
		binary.LittleEndian.PutUint32(env[12:], sum)
		buf.Write(env[:])
		buf.Write(c.payload)
	}
	return buf.Bytes()
}

// patchChunkVersion rewrites the version field of the nth chunk in a file the
// writer produced with no compression, where the offsets are computable.
func patchChunkVersion(t *testing.T, data []byte, index int, version uint16) []byte {
	t.Helper()
	out := append([]byte(nil), data...)
	pos := headerFixedSize
	pos += 4 + len(testHeader().Model)
	pos += 4 + len(testHeader().Build)
	for i := 0; i < index; i++ {
		if pos+chunkHeaderSize > len(out) {
			t.Fatalf("chunk %d is past the end of the file", index)
		}
		length := binary.LittleEndian.Uint32(out[pos+8:])
		pos += chunkHeaderSize + int(length)
	}
	if pos+chunkHeaderSize > len(out) {
		t.Fatalf("chunk %d is past the end of the file", index)
	}
	binary.LittleEndian.PutUint16(out[pos+4:], version)
	return out
}
