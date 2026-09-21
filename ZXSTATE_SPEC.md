# ZXSTATE_SPEC.md - the `.zxstate` machine-state format

Revision 1. Reference implementation: zxgo-v3 (`pkg/state`,
`internal/emulator/state.go`), with `STATE_DESIGN.md` as the design document.

This document is written for someone implementing the format in another
emulator. It is normative: where it says MUST, a writer or reader that does not
do it is not interoperable, and every such rule has a matching check in the
reference implementation.

---

## 1. What this format is for

`.zxstate` is a **native, exact, self-contained machine state**: everything an
emulator holds, so that a machine can be stopped at tick *N* and resumed at tick
*N* with nothing missing and nothing drifting.

It is not an interchange format. SNA and Z80 remain the formats for moving a
snapshot into another emulator or into a tool; they are lossy by construction
(no AY phase, no FDC, no mounted media, and the 128K SNA shape cannot hold a
Pentagon's extra banks). `.zxstate` is the opposite trade: complete and
versioned, at the cost of needing a reader that knows what it is reading.

What "exact" means, concretely, and why it is a format requirement rather than a
quality goal: a machine that is restored must produce the **same video frames**
and the **same audio samples** as the machine it was copied from. Visible state
alone - registers, memory, border - is not enough. The failures this format
exists to prevent are all invisible at the moment of the restore:

- a raster position off by twelve T-states puts the next instruction's
  contention and the floating bus in the wrong place, and the two machines
  diverge on the first frame;
- an AY tone counter or envelope position off by a few chip cycles is inaudible
  for a third of a second and then is not;
- an FDC's deadline measured from the wrong origin changes which sector is under
  the head;
- a mixer that resumes its sample grid from zero drops or duplicates a window.

So a conforming writer stores phase, deadlines and event cursors, not just
levels.

### 1.1 Scope

- The Spectrum side: Z80, RAM banks, paging, ULA, keyboard-independent video
  state, the audio sources and the mixer, the disk controllers, the mounted
  media, the tape.
- The General Sound card as a machine inside the machine: its own Z80, its RAM,
  its paging register, its DAC.
- Whatever a future model adds: a new device is a new chunk id, and an unknown
  chunk is skipped rather than fatal, so a reader does not have to be updated to
  keep working.

### 1.2 Non-goals

- Interchange with other emulators' snapshots (section 13.3 says what that costs
  in practice).
- Forward compatibility in the writing direction: an older build reading a newer
  file **refuses** rather than guessing.
- Host state: which window is open, the keymap, the audio device's queue, the
  currently pressed keys.
- Metadata: there is no timestamp, no screenshot, no author field. A session is
  identified by its contents.

### 1.3 The file name

`.zxstate` is the extension the reference emulator uses, and it is a convention
rather than part of the format: nothing in the file records it, and the magic
decides what a file is. An implementation is free to use its own, but it should
not *require* the extension when reading - a file renamed by hand is still a
valid file.

The reference CLI treats the extension as a default rather than a rule: a path
given to a save without it has it appended (`-session game` writes
`game.zxstate`, so a session's name says what is in it and a typo in the
extension is a new file rather than an overwritten one), and a load looks for the
path as given and then for the suffixed name, so that `-session game` is the same
request whether or not the file was created by a previous run.

---

## 2. Conventions

### 2.1 Byte order

**Little-endian throughout**, with no exceptions. Every integer is stored in its
natural width; nothing is variable-length except strings and byte blobs, and
those carry an explicit 32-bit length.

### 2.2 Floats

IEEE-754 bit patterns, stored through their integer form: `F32` is 4 bytes, `F64`
is 8. A float is never stored as text.

### 2.3 Strings

A string is a `U32` byte length followed by that many bytes. The bytes are
conventionally UTF-8 but the format never inspects them; a path may be anything
the writer's filesystem accepts. A length above 33554432 (32 MiB) is invalid.

### 2.4 Primitive types

These are the names used in section 8's field tables. They are also the
reference implementation's encoder methods.

| Name | Size | Encoding |
|---|---|---|
| `U8` | 1 | unsigned byte |
| `U16` | 2 | unsigned, little-endian |
| `U32` | 4 | unsigned, little-endian |
| `U64` | 8 | unsigned, little-endian |
| `I16` | 2 | signed, two's complement, little-endian |
| `I32` | 4 | signed, two's complement, little-endian |
| `I64` | 8 | signed, two's complement, little-endian |
| `F32` | 4 | IEEE-754 single, little-endian bits |
| `F64` | 8 | IEEE-754 double, little-endian bits |
| `Bool` | 1 | `0x00` false, `0x01` true. A reader MUST treat any non-zero byte as true |
| `Count(n)` | 4 | a `U32` element count for a count-and-loop encoding. See the bound rule below |
| `Bytes` | 4+n | `U32` length, then n bytes |
| `String` | 4+n | as `Bytes`; the bytes are the text |
| slices | 4+ | `U32` count, then each element in order (`U16s`, `U32s`, `U64s`, `I64s`, `Strings`) |

`I16`, `I32` and `I64` are the same bytes as their unsigned counterparts; the
distinction exists only so a reader sign-extends.

**The count bound (normative).** A reader MUST NOT allocate from a `Count` or a
`Bytes` length before checking it against the remaining payload: a `Count(n)`
whose elements could not fit in what is left is a refusal, not an allocation. The
reference implementation checks `count * elementSize <= remaining` for a `Count`
and `length <= remaining` for `Bytes`. Without this rule a 30-byte file can ask a
reader for a terabyte.

### 2.5 The tick clock

Two timelines appear in this format and they are not the same one.

- **Spectrum T-states.** The machine's own clock: 3.5 MHz on a 48K, 3.5469 MHz on
  a 128K, 3.584 MHz on a Pentagon. Every *absolute tick* in this document is a
  value on this clock, counted from the machine's reset, and the emulator chunk's
  `totalTicks` is where that counter stood when the file was written.
- **The audio sample index.** An integer, not a time. Sample *n* covers the
  T-state window `[n * cpuHz / sampleRate, (n+1) * cpuHz / sampleRate)`, computed
  from the index rather than accumulated, so rounding cannot build up. The mixer
  chunk stores the index it had reached.

Consequences a reader must honour:

1. Every absolute tick in the file - FDC deadlines, the ULA's interrupt
   deadline and deferred border change, tape ticks, AY write stamps, the GS's
   `gsTicks` - is **relative to the same clock as `totalTicks`**, so a reader
   MUST restore the machine's tick counter to exactly `totalTicks` before those
   values mean anything. A reader that cannot position its machine at an
   arbitrary T-state cannot read this format exactly; it should refuse rather
   than approximate.
2. A state is written at an **instruction boundary**. The program counter is the
   next instruction to execute, and no instruction is half-executed. A reader
   MUST apply the state at an instruction boundary too.

### 2.6 Checksums

Every chunk carries a CRC-32 in the **IEEE** form: polynomial `0xEDB88320`
reflected, initial value `0xFFFFFFFF`, final XOR `0xFFFFFFFF` - the same CRC as
zlib, PNG and `crc32.ChecksumIEEE`. Over the chunk's payload bytes only, not the
envelope.

### 2.7 Compression

`flags` bit 0 of the header marks a **raw DEFLATE** stream (RFC 1951) covering
the **body only**. It is not zlib (no 2-byte header, no Adler-32) and not gzip
(no 10-byte header, no CRC). In Go this is `compress/flate`; in C, libz's
`deflateInit2` with `windowBits = -15`; in Java, `Deflater(level, true)`; in
Python, `zlib.compressobj(wbits=-15)`. A reader that reaches for `inflate` (zlib)
instead of `inflateRaw` will fail on the first byte.

The header is never compressed, so magic, version and the model key can be
checked before a decompressor is started. A writer MAY use any DEFLATE level;
readers MUST NOT care which. The reference writer uses the default level.

---

## 3. File layout

```
+---------------------------------------------------+
| header                                            |
|   magic     [8]    "ZXGOSTAT"                     |
|   version   U16    container format version        |
|   flags     U16    bit 0 = body deflated           |
|   cpuHz     U32    the machine's T-state clock     |
|   model     String the model key                   |
|   build     String the writer's build id           |
|   chunks    U32    how many chunks the body holds  |
+---------------------------------------------------+
| body: 0 .. chunks chunks                           |
|   id        U32    chunk identifier                |
|   version   U16    the payload's version           |
|   flags     U16    reserved, MUST be zero          |
|   length    U32    payload byte count              |
|   crc32     U32    IEEE CRC-32 of the payload      |
|   payload   [length]                               |
+---------------------------------------------------+
```

The header's fixed part is 20 bytes; the two strings follow; the body follows
those. There is no padding and no alignment anywhere.

### 3.1 Header fields

| Field | Type | Meaning |
|---|---|---|
| `magic` | 8 bytes | The ASCII bytes `ZXGOSTAT`. A file whose first eight bytes are not these is not this format, whatever its extension |
| `version` | `U16` | Container format version. This revision is `1`. A reader MUST refuse a **greater** value; a lesser one is a file from before a change and the reader MAY accept it if it knows the older layout (there is none yet) |
| `flags` | `U16` | Bit 0 set means the body is DEFLATE-compressed. Every other bit is reserved: a reader MUST refuse a header with an unknown bit set, because flags change what the bytes mean and so cannot be skipped the way an unknown chunk can |
| `cpuHz` | `U32` | The machine's clock in T-states per second (3500000, 3546900, 3584000, ...). Redundant with the model but stored as a cross-check, and it is what a reader needs to convert the audio sample index to ticks |
| `model` | `String` | The model key, an identifier agreed between versions of a writer rather than a display name. zxgo-v3 uses `model.AllModels` keys: `48k`, `128k`, `2a3`, `pentagon`, `pentagon512`. A reader MUST refuse a file whose key is not its own machine's |
| `build` | `String` | The writer's build id, for bug reports and refusal messages. **Informational only**: a reader MUST NOT use it to decide whether a file loads |
| `chunks` | `U32` | How many chunks the body holds. The body's actual chunk count MUST equal this; a mismatch is a refusal. Readers MUST refuse a value above 4096 |

Why `chunks` is stored and checked: it is the only thing that catches a body
that was truncated exactly on a chunk boundary, where every remaining envelope
parses cleanly.

### 3.2 Chunk envelope fields

| Field | Type | Meaning |
|---|---|---|
| `id` | `U32` | Which component this chunk belongs to (section 7). Ids are never reused for a different component |
| `version` | `U16` | The payload layout's version, per component. A reader MUST refuse a chunk whose version is **greater** than the version it implements, and MUST pass a lesser one to the component so it can migrate or default the fields it does not find |
| `flags` | `U16` | Reserved. A writer MUST write zero; a reader MUST refuse a non-zero value |
| `length` | `U32` | Payload length in bytes. A reader MUST refuse a length above 33554432 (32 MiB), and a length that runs past the end of the body |
| `crc32` | `U32` | IEEE CRC-32 of the payload. A reader MUST verify it before the payload is used |

### 3.3 Body-level order

Chunk order in the body is the **writer's save order and carries no meaning**. A
reader applies chunks in its own load order, which is what resolves the one
dependency in the format: everything that holds an absolute tick is only
meaningful once the machine's tick counter is in place. A reader that applies
chunks in file order is not wrong as long as it restores the tick counter before
anything that depends on it is *used*; the reference writer happens to save the
emulator chunk first, so file order works too.

---

## 4. Limits

All limits are normative for readers. A writer that exceeds them is producing a
file this format does not describe.

| Limit | Value | Applies to |
|---|---|---|
| Container version | 1 | header `version` |
| Max header string | 33554432 bytes | `model`, `build` |
| Max chunk count | 4096 | header `chunks` |
| Max chunk payload | 33554432 bytes (32 MiB) | chunk `length` |
| Max body, decompressed | 67108864 bytes (64 MiB) | the whole body |

The largest legitimate chunk today is a Pentagon 512's RAM (512 KiB, the
biggest machine in the registry) or a disk image (~800 KiB); the ceilings are an
order of magnitude above that so a reader can bound its allocations without
being tight for a future model.

---

## 5. Refusal rules

A reader decides all of these **before applying anything** (section 6).

| Condition | Behaviour |
|---|---|
| Magic is not `ZXGOSTAT` | Refuse: not this format |
| Container version > the reader's | Refuse |
| Header flag bit above 0 set | Refuse |
| Header string longer than 32 MiB | Refuse |
| Model key is not the running machine's | Refuse |
| Chunk count > 4096 | Refuse |
| Body decompresses to more than 64 MiB | Refuse |
| Body ends mid-envelope, or a chunk runs past the end | Refuse: truncated |
| Chunk `flags` non-zero | Refuse |
| Chunk `length` > 32 MiB | Refuse |
| Chunk CRC does not match | Refuse: corrupt |
| The same chunk id appears twice | Refuse |
| Body holds a different number of chunks than the header declared | Refuse |
| A **required** chunk is absent | Refuse |
| A chunk version is greater than the reader's | Refuse |
| An **unknown** chunk id | Skip it, and report which |
| An **optional** chunk is absent | Use the machine's own default, and report which |

"Refuse" means: leave the machine exactly as it was and return a typed error
carrying the reason. It never means a partial apply and never a crash. Whether a
refusal is fatal is the caller's decision - a session restore typically reports
it and starts clean, while an explicit "load this file" reports it to the user.

Required-ness is a property of the **writer's** table, not of the id: see
section 7's "reference writer" column. A reader that does not implement a chunk
its writer marks required MUST refuse the file, because the machine would come
back wrong in a way that is hard to see.

### 5.1 The residual, stated honestly

There is one failure the envelope pass cannot catch: a payload whose CRC is valid
but whose contents do not decode (a field claiming more bytes than the payload
holds, a length that contradicts another field). This should be unreachable - a
valid CRC means the bytes are what the writer wrote, and section 2.4's bound rule
covers the lengths - but it is not *proved* unreachable.

The format bounds the consequence rather than pretending it is impossible: each
chunk is decoded into locals and assigned only after its whole payload decoded,
so a component is never half-restored. A reader may still end up with earlier
chunks applied and a later one untouched. A reader that wants whole-file atomicity
must decode every chunk into a staging structure first and apply afterwards; the
reference implementation takes the simpler route and documents the bound.

---

## 6. Reading algorithm

```
1. Read the 20-byte fixed header. If fewer than 20 bytes are available and the
   bytes present contain the whole magic, the file is truncated; otherwise it is
   not this format. (A file that stops inside the magic itself reads as "not this
   format", which is the more useful answer for a file that was never one.)
2. Check magic, container version, header flags.
3. Read `model` and `build` as length-prefixed strings (bounded).
4. Compare `model` with the running machine's key. Refuse on a mismatch.
5. Read `chunks`; refuse above 4096.
6. Read the body: if the flag is set, wrap the remaining input in a raw-DEFLATE
   reader. Read at most 64 MiB + 1; a longer stream is refused.
7. PASS ONE - envelope only:
     walk the body chunk by chunk, and for each one check the flags, the length
     bound, that the payload is inside the body, the CRC, and that the id has not
     already appeared. Collect (id, version, payload) but do not decode any
     payload yet.
     Then, against your own section table: refuse if a chunk's version is greater
     than yours; refuse if a required section is absent.
8. PASS TWO - apply:
     for each section in YOUR load order, if the body holds it, decode and apply
     it. Nothing is applied before every check above has passed.
9. Recompute anything your machine derives rather than stores, and discard any
   audio still buffered on your side (see section 12.4).
```

Two properties this buys, both worth having:

- Every refusal the reader can see happens before the machine is touched, so a
  corrupt file cannot leave a half-restored machine.
- An unknown chunk costs a length-prefixed skip, so a reader from before a device
  existed still reads a file that has one.

---

## 7. Chunk registry

Ids are assigned once and never reused; a retired component leaves a gap, which
is safer than handing its number to something else. Ids are stable across
container versions.

| Id | Name | Written by | Reference writer marks |
|---|---|---|---|
| 1 | `emulator` | the emulator itself | required |
| 2 | `cpu` | the Spectrum's Z80 | required |
| 3 | `cpu-gs` | the General Sound's Z80 (reserved; the card nests its CPU inside id 14 rather than using this) | - |
| 4 | `ram` | the memory mapper | required |
| 5 | `paging` | the banking scheme | required |
| 6 | `ula` | the video/border/interrupt chip | required |
| 7 | `beeper` | the 1-bit speaker | required |
| 8 | `mixer` | the audio sample grid | required |
| 9 | `ay` | one AY-3-8912 | required when the machine has a single AY |
| 10 | `turbosound` | two AY-8912s | required when the machine has one |
| 11 | `ym2203` | one YM2203 (OPN) | - (see 12) |
| 12 | `covox` | the Covox 8-bit DAC | required |
| 13 | `soundrive` | four DACs on ports 0x0F/0x1F/0x4F/0x5F | - (no model registers one today) |
| 14 | `general-sound` | the GS card | required when the machine has one |
| 15 | `beta-disk` | the WD1793 controller | required |
| 16 | `upd765` | the +2A/+3 floppy controller | required on a +2A/+3 |
| 17 | `disk-images` | the mounted disks | optional |
| 18 | `tape` | the mounted tape | optional |
| 19 | `turbosound-fm` | two YM2203s | required when the machine has one |

Ids 9, 10 and 19 are **mutually exclusive**: a machine has one AY chip, and the
chunk id says which. Id 11 (a bare YM2203) exists for a machine that fits one
chip; the reference implementation's FM machine is the two-chip board, so it
writes 19.

Chunk **versions** are per component and all start at 1. A component that adds a
field appends it and bumps its version; a reader sees the older version and
treats the missing field as absent (section 9).

---

## 8. Chunk payloads

Every payload below is written in the order given, with no padding. Offsets are
given so an implementer can check a hand-written encoder against a dumped file.

### 8.1 Id 1 - `emulator`, version 1 (25 bytes)

The machine's own counters. It is the first thing to restore, because every
absolute tick in the rest of the file is on this clock.

| Offset | Type | Field |
|---|---|---|
| 0 | `I64` | `totalTicks` - T-states executed since reset |
| 8 | `I64` | `frameCount` - frames completed since reset |
| 16 | `I64` | `lastIntTick` - the tick of the last accepted interrupt (diagnostics) |
| 24 | `Bool` | `fastTape` - the fast-tape switch, which changes how the run loop paces itself, not the machine |

`fastTape` is a host-side switch that nevertheless belongs here: it is part of
what the machine was doing, and a session restored without it would run the tape
at a different speed from the one it was saved in.

### 8.2 Id 2 - `cpu`, version 1 (36 bytes)

A Z80's full register file, including the parts a program can observe.

| Offset | Type | Field |
|---|---|---|
| 0 | `U8` | `A` |
| 1 | `U8` | `F` |
| 2 | `U8` | `B` |
| 3 | `U8` | `C` |
| 4 | `U8` | `D` |
| 5 | `U8` | `E` |
| 6 | `U8` | `H` |
| 7 | `U8` | `L` |
| 8 | `U8` | `A'` |
| 9 | `U8` | `F'` |
| 10 | `U8` | `B'` |
| 11 | `U8` | `C'` |
| 12 | `U8` | `D'` |
| 13 | `U8` | `E'` |
| 14 | `U8` | `H'` |
| 15 | `U8` | `L'` |
| 16 | `U16` | `IX` |
| 18 | `U16` | `IY` |
| 20 | `U16` | `SP` |
| 22 | `U16` | `PC` - the **next** instruction to execute |
| 24 | `U8` | `I` |
| 25 | `U8` | `R` - the whole byte, including bit 7: the reference CPU keeps whatever was written there |
| 26 | `U8` | `IM` (0, 1 or 2) |
| 27 | `Bool` | `IFF1` |
| 28 | `Bool` | `IFF2` |
| 29 | `Bool` | `HALT` - the CPU is halted and is executing NOPs |
| 30 | `U16` | `MEMPTR` / `WZ`, the undocumented register |
| 32 | `Bool` | `eipending` - an `EI` has executed and interrupts enable after the next instruction. Spans the save boundary, so it is state |
| 33 | `Bool` | `interruptPending` - the `/INT` level the CPU sampled at the last instruction boundary (see below) |
| 34 | `Bool` | `cpuTypeNMOS` - true for an NMOS Z80, false for CMOS |
| 35 | `Bool` | `memptrReal` - which of the two documented behaviours the repeating block I/O instructions use for `MEMPTR` (see below) |

`interruptPending` deserves a note, because the obvious implementation of a
reader disagrees with its own emulator here. `/INT` is level-triggered and
sampled at the end of an instruction; if the pulse has ended by then, no
interrupt is taken. A reader that recomputes this from "is the ULA asserting
`/INT` now?" is right for the Spectrum's own CPU - that is what the reference
implementation does after a load - but a CPU *inside a device* has no such
source: the General Sound's Z80 latches its level when the card crosses its
interrupt period, and nothing outside the card can recompute it. So the field is
stored, and a reader that also recomputes its Spectrum CPU's level is doing the
right thing twice.

`memptrReal` and `cpuTypeNMOS` are behaviour switches rather than registers, and
they belong in the file for the same reason the flags do: an NMOS Z80 and a CMOS
Z80 differ in the flags of half a dozen instructions, and the two `MEMPTR`
behaviours differ on the repeat path of `INIR`/`INDR`/`OTIR`/`OTDR`. A restored
machine with the wrong pair diverges on the first instruction that touches them.

This payload is also used, nested, for the General Sound's Z80 (section 8.11).

### 8.3 Id 3 - `cpu-gs`

Reserved and currently unwritten: the General Sound nests its CPU inside its own
chunk so that the card is one unit. A future card whose CPU is restored
independently can use this id with the id 2 layout.

### 8.4 Id 4 - `ram`, version 1

Every RAM bank, in bank order, as a count and then each bank as a `Bytes` blob.

```
U32   bank count
then, for each bank:
  U32   bank length in bytes
  [bank data]
```

Two things a reader must not assume:

- **The bank count is the writer's mapper's, not the model's.** A 48K machine in
  the reference implementation is built on a generic mapper that allocates eight
  16K banks for every model, so its file carries eight banks (128 KiB) even
  though the machine is a 48K one; only banks 5 (screen at 0x4000), 2 (0x8000)
  and the paged one (0xC000, bank 0 by default) hold anything. A reader MUST
  accept whatever count the file has, refuse a count it cannot map, and MUST NOT
  assume the count identifies the model - the model key does that. The reference
  reader refuses a count that is not its own mapper's.
- **Bank width is 16 KiB** in every model the reference implementation has, and
  it refuses any other width. A reader with a different bank granularity (say
  8 KiB) must regroup.

ROM is **not** in this chunk. ROMs come from the model's configuration and are
never restored from a state file, so a session cannot smuggle a ROM into a
machine.

### 8.5 Id 5 - `paging`, version 1 (46 bytes for a 128K; varies with the scheme and the blob)

```
String  scheme     the banking scheme's name
Bytes   blob       that scheme's own encoding (below)
U8      write7FFD  the last byte written to port 0x7FFD
```

The scheme name is the **discriminator**: the 128K's slot-3 page is three bits,
the Pentagon 512's is five bits in the same field, and the +2A/+3 has a whole
extra paging mode, so a blob read by the wrong scheme produces a machine that
looks restored and pages to the wrong banks. A reader MUST refuse a scheme name
that is not its own machine's.

`write7FFD` is the last byte *written* to port 0x7FFD, which is not recoverable
from the effective paging state once the lock bit (bit 5) has been set and later
writes have been ignored. It exists because the 128K SNA format stores exactly
this byte, so a machine restored from a `.zxstate` and then saved as an SNA must
write what the original would have written. A model with no 0x7FFD port (a 48K)
writes 0.

#### 8.5.1 Blob `standard128` (also used by `pentagon512`) - 26 bytes

| Offset | Type | Field |
|---|---|---|
| 0 | `I64` | active ROM bank |
| 8 | `I64` | slot 3 RAM bank (0-7, or 0-31 on the Pentagon 512) |
| 16 | `Bool` | shadow screen: the ULA reads bank 7 instead of bank 5 for 0x4000-0x7FFF |
| 17 | `Bool` | paging locked: further 0x7FFD writes are ignored until reset |
| 18 | `I64` | the ROM to return to when a PC-activated ROM deactivates |

The scheme name in the enclosing chunk is what distinguishes the 128K's 3-bit
page from the Pentagon 512's 5-bit one; the blob is identical for both.

#### 8.5.2 Blob `plus3` - 58 bytes

| Offset | Type | Field |
|---|---|---|
| 0 | `I64` | active ROM bank (0-3) |
| 8 | `I64` | slot 3 RAM bank (0x7FFD D0-D2) |
| 16 | `Bool` | shadow screen (0x7FFD D3) |
| 17 | `Bool` | paging locked (0x7FFD D5) |
| 18 | `Bool` | special paging mode (0x1FFD D0): all 64K is RAM |
| 19 | `I64` | special-mode map selector (0x1FFD D1-D2) |
| 27 | `I64` | ROM select low bit (0x7FFD D4) |
| 35 | `I64` | ROM select high bit (0x1FFD D2) |
| 43 | `I64` | the ROM to return to when a PC-activated ROM deactivates |

The +2A/+3 special mode maps the whole address space to RAM pages, so a reader
must keep the map selector: it is what decides which physical page appears at
0x0000, where the ROM normally lives and where a `+3` disk's loader often runs
from.

### 8.6 Id 6 - `ula`, version 1 (198 bytes)

The video chip's position and its timing deadlines.

| Offset | Type | Field |
|---|---|---|
| 0 | `I64` | `absoluteClock` - cumulative T-states since reset. Equal to `totalTicks` by construction; stored so the chunk stands alone |
| 8 | `I64` | `clock` - the T-state position within the current frame |
| 16 | `I64` | `line` - the current scanline |
| 24 | `U8` | border colour (0-7) |
| 25 | `U8` | the border colour waiting to be applied |
| 26 | `I64` | the tick at which that pending colour takes effect (0 if none) |
| 34 | `Bool` | flash phase |
| 35 | `U8` | frames since the flash phase last toggled (0-15) |
| 36 | 16 x (`I64` + `U8`) | the snow ring: 16 entries of (tick, byte), then |
| 180 | `U8` | the snow ring's next write index |
| 181 | `I64` | `intAssertedUntil` - the tick at which `/INT` deasserts |
| 189 | `I64` | the frame count at the last interrupt (diagnostics) |
| 197 | `Bool` | the EAR bit the ULA is presenting on port 0xFE bit 6, as driven by the tape |

The **raster position** is the load-bearing field: the next instruction's memory
contention and the `floating bus` byte both depend on where the beam is, so a
machine restored with it at zero diverges on the first frame.

The **snow ring** is the 48K artefact where a CPU write to contended memory puts
its byte on the data bus and a ULA screen fetch overlapping that window reads the
CPU's byte instead. It is a ring of the last 16 contended writes with their
ticks; a reader that does not model the effect can ignore the whole ring (it is
self-contained), but a reader that does model it must keep the ticks, because
the effect is only live for three T-states after the write.

The **deferred border write** is the other sub-instruction detail: the border
register latches near the end of an `OUT (n),A`, not at its start, so a write
that happens to land just before the save is still pending and must be applied at
its tick.

### 8.7 Id 7 - `beeper`, version 1 (16 bytes with no pending events)

| Offset | Type | Field |
|---|---|---|
| 0 | `Bool` | EAR bit (port 0xFE bit 4) |
| 1 | `Bool` | MIC bit (port 0xFE bit 3) |
| 2 | `I16` | the output level in effect at the mixer's cursor (see below) |
| 4 | `I64` | the tick a write made now would be stamped with |
| 12 | `Count` | how many unconsumed level events follow |
| 16 | n x (`I64` + `I16`) | (tick, level) pairs |

Three concepts here recur in every audio source, so they are worth stating once:

- **`level`** is the source's output in the writer's own scale. It is not
  normalised across implementations: what matters is that the same machine
  produces the same numbers, and an emulator porting this format must map these
  onto its own scale **and must say so**, because a file written by one will not
  sound identical on the other unless the scales agree.
- **`resolved`** (here offset 2) is the level in effect at the mixer's cursor -
  the value the generator had the last time the mixer asked. Restoring it as zero
  puts a step into the first samples after a load.
- **The event list** is the writes the mixer has not consumed yet: a level change
  at a tick. A reader may ignore it and resume from `resolved`, at the cost of
  losing the exact placement of level changes that had already happened but not
  yet been sampled. The reference writer drops the events the mixer has already
  consumed, so what is in the file is exactly what the restored mixer will read.

### 8.8 Id 8 - `mixer`, version 1 (32 bytes)

The audio sample grid's position and the DC-block filter's memory.

| Offset | Type | Field |
|---|---|---|
| 0 | `I64` | `nextSample` - the sample index the grid will produce next |
| 8 | `I64` | `generated` - total samples produced since reset (an invariant check) |
| 16 | `I32` | left channel: the DC-block filter's previous input |
| 20 | `I32` | left channel: the filter's previous output |
| 24 | `I32` | right channel: the filter's previous input |
| 28 | `I32` | right channel: the filter's previous output |

`nextSample` is machine state, not host state: every sample's timestamp is
derived from the machine's tick counter, so a reader that resumes the grid from
zero either drops or duplicates a window (and if it resynchronises by
jumping, that is audible). It converts to ticks exactly as section 2.5
describes.

The **DC-block filter** models the series capacitor at a real machine's output:
`y = x - xPrev + R*yPrev` with `R = 255/256`, per ear. The previous input and
output are its whole state; starting from zero after a load puts a decaying step
into the output.

### 8.9 Id 9 - `ay`, version 1 (119 bytes with an empty write queue)

One AY-3-8912, including every divider the generator advances.

| Offset | Type | Field |
|---|---|---|
| 0 | `I64` | the tick a register write made now would be stamped with |
| 8 | 16 x `U8` | the 16 shadow registers. R0-R13 as written; R14/R15 read as 0xFF |
| 24 | `U8` | the latched register select (persists across data writes) |
| 25 | 3 x (`U32` + `I64` + `Bool`) | per channel: the tone period, the countdown to the next toggle, the current output state |
| 64 | `U32` + `I64` + `U32` + `Bool` | the noise period, its countdown, the 17-bit LFSR, the current noise output |
| 81 | `U32` + `I64` + `U8` + `U8` + 5 x `Bool` | the envelope period, its countdown, the envelope volume, the shape byte, and the five derived shape bits (hold, alternate, attack, continue, holding) |
| 100 | `U8` | the mixer register R7 (a set bit *disables* that generator on that channel) |
| 101 | 3 x (`U8` + `Bool`) | per channel: the amplitude and whether the envelope drives it |
| 107 | `I64` | the tick-to-chip-cycle remainder (see below) |
| 115 | `Count` | how many queued register writes follow |
| 119 | n x (`I64` + `U8` + `U8`) | (tick, register, value) |

Details that matter:

- **The chip clock is the CPU clock divided by two** on a 128K (1.7734 MHz). A
  reader with a different clock ratio must rescale the counters, or refuse.
- **The counters are in chip cycles and they count *down***: each holds the
  number of cycles until its next event. This is an implementation choice that
  leaks into the format; a reader that counts up must convert (subtract from the
  reload value), and a reader that computes its dividers differently must map
  them consistently.
- **`chipAcc`** (offset 107) is the remainder of the tick-to-chip-cycle
  conversion, carried so the ratio stays exact across calls. It is the single
  most easily discarded field and dropping it makes the pitch wobble by a cycle
  per audio window.
- **The queued writes** (the last field) are *not* history: each entry is a write
  that has not reached its tick yet. They are stamped on the machine's tick
  clock, so they mean the same thing after a restore. A reader may instead apply
  them immediately at load time, which moves a write up to one audio window early
  - acceptable, and the reference writer does not do it.

### 8.10 Id 10 - `turbosound`, version 1

Two AY-8912s and which one the select port last chose.

```
Bool   which: false = chip 0, true = chip 1
Bytes  chip 0, as an id 9 payload
Bytes  chip 1, as an id 9 payload
```

The nested payloads use the **same chunk version** as the enclosing chunk. Chip
0 is panned left, chip 1 right.

### 8.11 Id 11 - `ym2203`, version 1

One YM2203 (OPN): three 4-operator FM channels plus an AY-compatible SSG.

```
Bytes  the SSG, as an id 9 payload
U32    the envelope-generator counter (egCnt)
I64    the envelope-generator timer
I64    the current tick (the write timestamp)
U8     the latched register address
U8     the prescaler selector (clock divisor is {24,24,72,36}[sel & 3])
U8     register 0x27: bit 6 = channel 3 three-slot mode, low 6 bits = timer control
3 x U32  the channel-3 independent frequencies used in three-slot mode
U32    timer A period
U32    timer B period
I64    timer A countdown
I64    timer B countdown
U8     the timer overflow flags (bit 0 = A, bit 1 = B)
I64    the timer accumulator
I64    the tick-to-sample remainder
I32    the last output value
I64    the tick until which the busy flag is set
3 x channel:
  4 x operator: U32 phase, U32 increment, I32 detune, U8 detune index,
                U8 key-scale-rate register, U8 computed key-scale rate,
                U32 ar, U32 d1r, U32 d2r, U32 rr, U32 mul, U32 tl, U32 sl,
                U8 ssg mode, U8 ssg sign latch, I64 envelope state,
                I32 envelope volume, Bool key on,
                U8 egShAr, U8 egSelAr, U8 egShD1r, U8 egSelD1r,
                U8 egShD2r, U8 egSelD2r, U8 egShRr, U8 egSelRr
  U8 algo, U8 feedback, U32 channel frequency,
  I32 op1 output[2], I32 memory value,
  I64 connect1, I64 connect2, I64 connect3, I64 connect4, I64 memConnect
```

This is the largest and the most implementation-shaped payload in the format.
The operator fields include values decoded from registers (`ar`, `d1r`, `mul`,
`tl`, ... and the whole envelope-rate table) rather than the registers
themselves, because the reference implementation does not keep the FM register
file as bytes; reconstructing them would mean replaying register writes in
order. A conforming reader must map them onto whatever its own FM core keeps, and
section 13.3 explains why byte-for-byte FM equivalence between two emulators is
not achievable in general.

### 8.12 Id 12 - `covox`, version 1 (14 bytes with no pending events)

The Covox 8-bit DAC, which is a generic DAC plus the port it decodes.

| Offset | Type | Field |
|---|---|---|
| 0 | `I16` | the level in effect at the mixer's cursor |
| 2 | `I64` | the write timestamp |
| 10 | `Count` | unconsumed level events |
| 14 | n x (`I64` + `I16`) | (tick, level) |

The port (0xFB for the Pentagon/ATM standard, 0xDD for the Scorpion variant) is a
property of the model, not of the file.

### 8.13 Id 13 - `soundrive`

Reserved. The reference implementation implements the device (four DACs on ports
0x0F, 0x1F, 0x4F, 0x5F, mixed A+C left and B+D right) but no model registers one,
because it cannot share a machine with a Beta Disk interface. When a machine has
one, the payload is four id 12-shaped DACs. A reader should skip this id.

### 8.14 Id 14 - `general-sound`, version 1

The GS card: a machine inside the machine.

```
Bytes  the card's Z80, as an id 2 payload
Bytes  the card's RAM: exactly 524288 bytes
U8     the memory-mapping page register
U8     the host command register
U8     the host data register
U8     the card's status register
U8     the card's output register
4 x U8 the DAC levels (volume-applied)
4 x U8 the channel volumes (0-63)
I64    gsTicks - the card's own T-state counter (12 MHz clock)
I64    the interrupt counter
I16    the resolved DAC output, left
I16    the resolved DAC output, right
Count  unconsumed DAC events
then n x: I64 gsTick, I16 left, I16 right
```

The card's flat memory is 32768 bytes of ROM followed by 524288 bytes of RAM, and
**only the RAM is stored**: writes to the ROM segment are refused by the card's
own paging, so the ROM never changes and the machine reading the file already has
it. A reader MUST NOT expect the ROM here.

The card's DAC values are written by the card's Z80 through memory-mapped
addresses (bits 8-9 of the address select the channel), so its DAC state is a
consequence of its own execution - which is why the card's CPU has to be restored
exactly, and why the latched `/INT` level in section 8.2 matters.

### 8.15 Id 15 - `beta-disk`, version 1 (84 bytes with an empty buffer)

The WD1793 controller of a Beta 128 interface.

| Offset | Type | Field |
|---|---|---|
| 0 | `U8` | status register |
| 1 | `U8` | track register |
| 2 | `U8` | sector register |
| 3 | `U8` | data register |
| 4 | `U8` | the command being executed |
| 5 | `Bool` | interrupt request pending (INTRQ) |
| 6 | `Bool` | data request (DRQ) |
| 7 | `Bool` | which of the two status layouts the STATUS register reports |
| 8 | `I64` | the controller's copy of the tick clock |
| 16 | `I64` | its CPU clock (for converting step rates) |
| 24 | `U8` | drive select |
| 25 | `U8` | side select |
| 26 | `Bool` | system reset held |
| 27 | `I64` | the active drive |
| 35 | `U8` | the state machine's phase |
| 36 | `I64` | the transfer byte counter |
| 44 | `Bytes` | the in-flight transfer buffer |
| then | `Bool` | a multi-sector transfer is running |
| | `Bool` | a WRITE TRACK is running |
| | `I64` | its start tick |
| | `I64` | the current transfer's start tick |
| | `Bool` | a Type I command (seek/step) is running |
| | `I64` | the tick it completes |
| | `I64` | the tick the data becomes ready |
| | `Bool` | FDC timing is disabled (`-no-fdc-timing`) |

Every tick here is absolute on the machine's clock, so the reader must have
restored `totalTicks` first. The four deadlines are what make a mid-transfer
session resume: a sector that is half-read when the state is taken continues from
the same byte.

The **mounted disks are not here** - they are mutable and live in id 17.

### 8.16 Id 16 - `upd765`, version 1

The +2A/+3's floppy controller, in the same shape as id 15.

| Type | Field |
|---|---|
| `U8` | selected drive |
| `U8`, `U8`, `U8` | the current track, side and sector as the CPU's registers report them |
| `U8`, `U8`, `U8` | the physical cylinder (`curPCN`), head and sector-size code |
| `U8` | the command byte |
| `Bytes` | its parameters |
| `I64` | how many parameters have been supplied |
| `Bytes` | the result queue |
| `I64` | the result queue's read index |
| `Bytes` | the data-transfer buffer |
| `I64` | the transfer's read/write index |
| `Bool` | true for WRITE DATA, false for READ DATA |
| `Bool` | the command expects deleted data sectors |
| `Count` + n x (`U8` + `I64`) | the write plan: per sector, its ID and its length |
| `Bool` | the write was refused because the disk is read-only |
| `Bool` | a FORMAT TRACK is in progress |
| `U8` | the FORMAT TRACK fill byte |
| `I64` | the phase: 0 command/params, 1 transfer, 2 result |
| `U8` | status register 0 |
| `U8` | the physical cylinder as the controller knows it |
| `Bool` | an interrupt awaits SENSE INTERRUPT STATUS |
| `U8` | the seek-in-progress bits, one per drive |
| `U8` | the last main status register value read |
| `I64` | the consecutive-read counter for the weak-sector behaviour |
| `I64` | the key of the last sector read |
| `I64` | the current tick |
| `I64` | the tick the target sector is under the head |
| `I64` | the tick the transfer phase must complete by (overrun) |
| `I64` | the CPU clock |
| `Bool` | a SEEK/RECALIBRATE is running |
| `I64` | the tick it completes |
| `Bool` | FDC timing is disabled |

The three deadlines are again absolute ticks. `dataDeadline` models the real
controller's behaviour when a CPU stops draining a read-data phase: the real chip
raises an overrun and terminates the phase, which several copy-protection loaders
depend on.

### 8.17 Id 17 - `disk-images`, version 1 (optional)

The mounted disks, embedded whole. A disk is mutable, so it is *not* a reference
to a file: it is the disk as the machine sees it, including anything the machine
has written and anything the file on disk no longer has.

```
U32   how many mounted disks follow
then, per disk:
  String  slot name
  <disk>
```

**Slot names** are `beta:N` and `upd765:N`, N being the drive index (0-based). A
reader that has fewer drives must skip the slots it cannot place and report them
rather than refusing the file: a session saved on a machine with two +3 drives is
still usable on one with a single drive.

**The disk record:**

```
String  the format label ("TRD", "SCL", "DSK", ...). Informational
I64     sides (1 or 2)
I64     tracks per side
I64     sectors per track (nominal)
I64     sector size in bytes (nominal)
Bool    the disk is write-protected
U32     track count, then per track:
  U32     sector count, then per sector:
    U8    the C field of the sector's ID
    U8    the H field
    U8    the R field (sector number)
    U8    the N field (size code: 128 << N bytes)
    Bytes the sector's data
    Bool  the sector is marked deleted
    U8    status register 1 as read (bit 5 = data error, ...)
    U8    status register 2 as read
    U16   the ID field's CRC
    Bytes the weak-bit mask (below)
```

Unlike the SNA/Z80/TRD/DSK formats, this record carries the fields a
copy-protection loader depends on: per-sector status, deleted marks, and the
weak-bit mask - which is why a disk is written as a structure rather than
re-encoded into an image format that cannot hold them.

**The weak-bit mask** is bit-packed: byte `i/8`, bit `i%8` is set when data byte
`i` reads back unstably. Its length is exactly `ceil(len(data)/8)`; an empty mask
means the sector has no weak bits. A mask that does not match the data length is
a refusal, not something to pad.

Reader bounds: at most 256 tracks, 256 sectors per track, 16384 bytes per sector.

### 8.18 Id 18 - `tape`, version 1 (optional)

A tape is **referenced**, not embedded: tape writing is out of scope for the
reference emulator, so a tape's pulse stream is a pure function of the file it
came from and a reference is enough. This is the opposite of the disk decision,
and deliberately so.

```
Bool    is a tape loaded at all? false ends the payload
then, if true:
  String  the path the tape was loaded from
  U32     a hash of the pulse stream (below)
  U32     the pulse count
  I64     the pulse index playback had reached
  I64     the tape's own tick counter
  I64     the tape tick at which the next level change happens
  I64     the last CPU tick the tape saw
  Bool    playback is active (playing, not paused)
  Bool    the end-of-tape notification has already fired
```

**The pulse-stream hash** is an IEEE CRC-32 over, for each pulse in order, nine
bytes: one byte holding the level (0 or 1) followed by the duration in T-states
as an 8-byte little-endian unsigned integer. It is taken over the decoded pulse
stream rather than the file, so it is computable whatever the file has since
done, and a tape loaded from inside a `.zip` is identified by its contents rather
than by the archive entry's name.

Three outcomes, all of which a reader should report rather than refuse:

| State | Reader behaviour |
|---|---|
| File present, hash and pulse count match | Resume at the saved pulse index |
| File present, hash or count differs | Load from the start; the saved position belongs to a different tape |
| File absent or unreadable | Leave the machine with no tape mounted |

### 8.19 Id 19 - `turbosound-fm`, version 1

Two YM2203s and which one the select port last chose.

```
Bool   which: false = chip 0, true = chip 1
Bool   the ready-flag poll state of the pseudo-register
Bytes  chip 0, as an id 11 payload
Bytes  chip 1, as an id 11 payload
```

The nested payloads use the same chunk version as the enclosing chunk. Chip 0 is
panned left, chip 1 right.

---

## 9. Versioning

Three numbers, three different jobs:

- **Container version** (header `version`): the envelope. Bump it only for a
  change to the header or to a chunk envelope - never because a component's
  payload changed. Currently 1.
- **Chunk version** (per chunk): one component's payload layout. A component that
  adds a field appends it and bumps its version.
- **Model key** (header `model`): which machine. A different key is a different
  machine and the file is refused.

Reading rules:

- A **greater** chunk version is a refusal. The reader cannot know what a field
  it has never seen means, and guessing would produce a machine that looks
  restored.
- A **lesser** chunk version is the component's business. The intended pattern is
  to check whether the payload has more bytes (`More()`) than the fields you have
  read, and default the rest:

  ```go
  a, b := d.U8(), d.U16()
  added := uint8(0x99)                 // default for a field this build has
  if d.Version() >= 2 && d.More() {
      added = d.U8()
  }
  ```

- **Unknown chunk ids are skipped**, and the reader may say which it skipped.
  This is what lets a reader from before a device existed keep working.
- A file written by a *newer* build of the same writer refuses rather than
  limping: only one direction is guaranteed, new builds reading old files.

---

## 10. Worked example

A real file: zxgo-v3, 48K model, three frames after reset, RAM mostly zero,
nothing mounted, written with compression off so the bytes are readable. Total
size 131909 bytes, 12 chunks.

### 10.1 Header

```
offset  bytes                       meaning
0       5a 58 47 4f 53 54 41 54     "ZXGOSTAT"
8       01 00                       version = 1
10      00 00                       flags = 0: the body is not compressed
12      e0 67 35 00                 cpuHz = 0x003567e0 = 3500000
16      0c 00 00 00                 chunks = 12
20      03 00 00 00 34 38 6b        model = length 3, "48k"
27      03 00 00 00 64 65 76        build = length 3, "dev"
34      <body>
```

### 10.2 The chunk stream

| Offset | Id | Name | Version | Length | CRC-32 | Notes |
|---|---|---|---|---|---|---|
| 34 | 1 | `emulator` | 1 | 25 | `85756204` | 3 ticks + a flag |
| 75 | 4 | `ram` | 1 | 131108 | `b23364a6` | 4 + 8 x (4 + 16384): **eight** 16K banks, because the writer's 48K machine is built on a mapper that allocates eight |
| 131199 | 5 | `paging` | 1 | 46 | `afa0981f` | `standard128`: 4 + 8 + 4 + 26 + 4 + 1 |
| 131261 | 2 | `cpu` | 1 | 36 | `197d9a50` | |
| 131313 | 6 | `ula` | 1 | 198 | `9d593d64` | |
| 131527 | 8 | `mixer` | 1 | 32 | `65cf602c` | |
| 131575 | 9 | `ay` | 1 | 119 | `8e082a11` | a 48K machine still has an AY fitted by the writer, with an empty write queue |
| 131710 | 7 | `beeper` | 1 | 16 | `099275cd` | no pending events |
| 131742 | 12 | `covox` | 1 | 14 | `e13a7bed` | no pending events |
| 131772 | 15 | `beta-disk` | 1 | 84 | `bbf69310` | no transfer in flight |
| 131872 | 17 | `disk-images` | 1 | 4 | `2144df1c` | count = 0: nothing mounted |
| 131892 | 18 | `tape` | 1 | 1 | `d202ef8d` | `false`: no tape |

### 10.3 The first two chunks, byte by byte

```
34  01 00 00 00     id        = 1 (emulator)
38  01 00           version   = 1
40  00 00           flags     = 0
42  19 00 00 00     length    = 25
46  04 62 75 85     crc32     = 0x85756204
50  <payload, 25 bytes>
      I64 totalTicks
      I64 frameCount
      I64 lastIntTick
      U8  fastTape

75  04 00 00 00     id        = 4 (ram)
79  01 00           version   = 1
81  00 00           flags     = 0
83  24 00 02 00     length    = 0x00020024 = 131108
87  a6 64 33 b2     crc32     = 0xb23364a6
91  <payload>
      08 00 00 00              bank count = 8
      00 40 00 00              first bank's length = 0x00004000 = 16384
      <16384 bytes>
      00 40 00 00              second bank
      ...
```

Note the offsets: the body begins at 34 because the header is 20 bytes plus two
length-prefixed strings, and every chunk begins at the previous one's end. There
is no alignment anywhere, so this arithmetic is exact and an implementer can check
a writer against it byte for byte.

### 10.4 Reproducing this example

```bash
go run ./cmd/zxgo -model 48k -log error     # in a second terminal, then Ctrl-C
# or, in tree, the example the numbers above came from:
go test ./internal/emulator/ -run TestRunForwardEquality -count=1
```

The reference writer emits this file uncompressed when asked
(`state.Options{Deflate: false}`), which is what makes the dump above readable.
Session files written by the CLI are deflated by default.

---

## 11. Reference reader

The envelope, in Go, with the reference implementation's identifiers so the
constants can be looked up. This is the whole of what a reader must get right
before it thinks about any payload.

```go
const magic = "ZXGOSTAT"

type chunk struct {
	id      uint32
	version uint16
	payload []byte
}

func readEnvelope(r io.Reader) (header, []chunk, error) {
	// Fixed header.
	var fixed [20]byte
	if _, err := io.ReadFull(r, fixed[:]); err != nil {
		return header{}, nil, errNotState
	}
	if string(fixed[0:8]) != magic {
		return header{}, nil, errNotState
	}
	var h header
	if v := le16(fixed[8:]); v > formatVersion {
		return h, nil, errVersion
	}
	flags := le16(fixed[10:])
	if flags&^1 != 0 {
		return h, nil, errFlags
	}
	h.cpuHz = le32(fixed[12:])
	declared := le32(fixed[16:])
	if declared > maxChunks {
		return h, nil, errChunks
	}
	// Two length-prefixed strings.
	var err error
	if h.model, err = readString(r); err != nil {
		return h, nil, err
	}
	if h.build, err = readString(r); err != nil {
		return h, nil, err
	}
	// The body: decompressed if flagged, and bounded on the way in.
	var src io.Reader = r
	if flags&1 != 0 {
		fr := flate.NewReader(r) // RAW deflate, not zlib
		defer fr.Close()
		src = fr
	}
	body, err := io.ReadAll(io.LimitReader(src, maxBody+1))
	if err != nil {
		return h, nil, err
	}
	if len(body) > maxBody {
		return h, nil, errTooBig
	}
	// Pass one: envelopes only.
	var chunks []chunk
	seen := map[uint32]bool{}
	for pos := 0; pos < len(body); {
		if pos+16 > len(body) {
			return h, nil, errTruncated
		}
		id := le32(body[pos:])
		ver := le16(body[pos+4:])
		fl := le16(body[pos+6:])
		length := le32(body[pos+8:])
		sum := le32(body[pos+12:])
		pos += 16
		if fl != 0 || length > maxChunk || uint64(length) > uint64(len(body)-pos) {
			return h, nil, errEnvelope
		}
		payload := body[pos : pos+int(length)]
		pos += int(length)
		if crc32.ChecksumIEEE(payload) != sum {
			return h, nil, errCRC
		}
		if seen[id] {
			return h, nil, errDuplicate
		}
		seen[id] = true
		chunks = append(chunks, chunk{id, ver, payload})
	}
	if uint32(len(chunks)) != declared {
		return h, nil, errCount
	}
	return h, chunks, nil
}
```

Pass two is the reader's own: for each of its sections, in its own order, take
the matching chunk from the list (absent optional ones default, absent required
ones refuse, versions above its own refuse), and call the component's load.

---

## 12. Porting notes

### 12.1 What a reader must map, not copy

The format stores the reference implementation's *structure* for anything the
hardware does not dictate. An implementer should expect to translate, and should
document what they translated:

| Concept | Why it is not universal |
|---|---|
| Audio level scales | `level`, `resolved` and the DAC values are in the writer's own scale. Two emulators must agree on the scale for a file to sound identical, and nothing in the format enforces that |
| Divider counters | The reference counts *down* in chip cycles. A chip model that counts up, or in T-states, must convert |
| Sample grid | The grid is `sample index -> tick = index * cpuHz / sampleRate`. A reader whose mixer works on wall-clock samples, or that resamples differently, cannot resume exactly and should say so |
| FM operator state | The decoded rate tables and envelope state are internal to one port of MAME's `fm.c`. Another FM core must map them onto its own |
| Raster position | The ULA's `clock` is a T-state index into the writer's frame geometry. Two emulators with the same frame length can share it; two with a different flyback split must convert |
| Bank numbering | Banks are numbered by the writer's mapper. A reader must derive the same numbering or map it |
| `memptrReal`, `cpuTypeNMOS` | These are behaviour switches between *documented* alternatives. A reader that does not implement both behaviours should refuse a file that asks for the one it lacks, rather than silently doing the other |

### 12.2 What is genuinely portable

Everything hardware-defined: the Z80 register file and its flags, the RAM banks
and the paging registers, the AY-8912's registers and the datasheet's divider
ratios, the WD1793's registers, the uPD765's, the disk contents, the tape's pulse
stream, and the machine's tick counter. A reader that implements the same hardware
will find these mean what they say.

### 12.3 A minimal conforming reader

A reader does not have to implement everything. The smallest useful one:

1. Parses the header and refuses a foreign magic, version or model key.
2. Walks the chunk envelopes, verifying CRCs and the required set.
3. Applies ids 1 (emulator), 4 (ram), 5 (paging), 2 (cpu) and 6 (ula).
4. **Skips** every other id, and reports which it skipped.

That machine will boot to the right screen and run the right program. It will not
sound right, and a restored disk or tape will be missing - which is exactly what
the report is for. Implementing 7, 8, 9/10/19 and 12 adds exact audio; 15/16/17
and 18 add storage.

### 12.4 Two things a loader must do beyond restoring fields

- **Recompute what the machine derives.** The most common example is the
  Spectrum CPU's `/INT` level, which the reference recomputes from the ULA's
  interrupt deadline after a load so that no stale level survives into the first
  instruction. Anything else your machine derives from several components belongs
  here too.
- **Discard buffered audio.** Whatever your mixer has staged belongs to a machine
  that no longer exists. The restored grid position is authoritative, so flush
  the buffer rather than pushing the old samples out after the load.

### 12.5 Writing a file others can read

- Write the header uncompressed unless size matters; a reader must handle both,
  but an uncompressed file is inspectable and diffable.
- Write every chunk your machine has, including the ones you think are
  uninteresting. A missing optional chunk is legal but it is a loss.
- Mark a chunk required only when the machine cannot be resumed without it.
- Never reorder or reuse ids (section 7).
- If you change a payload, **append** the new field and bump the chunk version.
  Never change the meaning of an existing field in place: an old file's bytes do
  not change when your code does.

### 12.6 Testing an implementation

- **Round trip**: save, load into a freshly built machine, compare every field.
  This catches a missing field but not a wrong one.
- **Run forward**: save; restore into a second machine; run both for N frames and
  compare a hash of the framebuffer and the tick count *every frame*. This is the
  test that matters - it fails on phase drift, which a field-by-field comparison
  does not see. The reference implementation runs it for every model.
- **Run forward on audio**: the same comparison against a hash of the samples the
  mixer emits per frame, with the chips actually producing sound (sweep the AY's
  registers, toggle the beeper). This is what catches a dropped carry remainder
  or a missing write timestamp.
- **Refusals**: corrupt a chunk and check the machine is untouched; feed a
  truncated file; feed another model's key; feed an unknown chunk id and check it
  is skipped rather than fatal.
- **The committed fixture**: `pkg/state/testdata/state_v1.zxstate` pins the
  envelope (`pkg/state`'s own tests load it byte for byte). Its two chunks carry
  synthetic payloads chosen to exercise the primitive types, so it is **not** a
  readable machine state: a reader that parses chunk 2 as a real CPU payload will
  reject it, correctly. Use it to check an envelope parser, and test payload
  parsing against a file your own writer produced or against the example in
  section 10.

---

## 13. Limitations and honest caveats

### 13.1 What is not in the file

| Missing | Why | Consequence |
|---|---|---|
| ROM contents | the model defines them | a session cannot carry a patched ROM; a reader must have the same ROM images |
| Keyboard matrix | a key held at save time is not held now | every key is released on load |
| Joystick, mouse | host input, re-read from the device | state lost across a save |
| Printer | not implemented in the reference emulator | would be a new chunk id |
| Replay recorder state | session, not machine state | a load during a recording is refused |
| Window and UI state | front-end concern | - |

### 13.2 What a reader cannot verify

The format carries a CRC per chunk, so corruption is caught. It carries no
signature, checksum of the whole file, or provenance: a file is trusted once its
per-chunk CRCs match. A file from another emulator that claims the same model key
will be applied as if it were ours, and the only defence is that the model key
must match.

### 13.3 Byte-exact interchange across emulators

The honest position: a `.zxstate` file resumes *exactly* on the emulator that
wrote it, and resumes *usefully* elsewhere. Full byte-exact interchange between
two different emulators is not achievable in general, because the file carries
each implementation's internal structure (FM operator state, audio level scales,
sample grid, contention model) for the parts the hardware does not dictate. Two
emulators whose cores are written independently will diverge on those, and no
format can fix that.

What the format does give an implementer is:

- a complete list of *what* must be carried, which is a specification of the
  problem even where the encoding is not portable;
- an envelope that fails loudly instead of silently when something is missing;
- a reader that keeps working when a device it does not have is present in the
  file.

---

## 14. Change log

| Revision | Date | Change |
|---|---|---|
| 1 | 2026-09-19 | First revision: container, ids 1-19, all payloads as implemented in zxgo-v3 |
