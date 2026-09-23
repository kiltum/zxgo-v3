# zxgo-v3

A ZX Spectrum emulator written in Go, for macOS, Linux and Windows.

This is the third generation of the project. The first two - `zxgo` in Go and
`zxcpp` in C++ - taught the lessons; this one was written from scratch with them
in mind, so the parts that were hard to get right the first two times (timing,
sound, the machine state) are built the right way round from the start.

## Accurate where it counts

**The processor is exact.** The Z80 core runs every documented and undocumented
instruction with the flags and the internal quirks of the real chip. It is not
"close enough": it is checked line by line against the Fuse test suite and
ZEXALL - both pass - and it passes a dedicated third-party test for MEMPTR, the
internal register that trips up most implementations, 98 cases out of 98.

**The picture is drawn clock by clock.** The video chip is not simulated frame
by frame, or scanline by scanline, but one tick of the machine's own clock at a
time.
That is what makes the hard things work: border effects, multicolour pictures
that change colour mid-line, the "snow" that appears on a 48K when the screen is
read at the wrong moment, and the floating bus. Each machine is drawn at its own
geometry and its own timing, so the picture appears where that machine puts it
and not where a fixed window would.

Software that depends on the clock - the border and raster demos people use to
judge an emulator - runs correctly, and every test in the tree passes.

**Everything else follows the same rule.** Memory contention is charged at the
exact machine cycle it happens in, which is what the fast loaders and the
full-screen scroll effects depend on. Disk drives take their rotational and seek
latency into account. Sound is generated on a fixed sample grid driven by the
sound card's own clock, so it neither drifts nor crackles over a long session.

## The machines

Five, each with its own memory layout, timing and contention:

| machine | notes |
|---|---|
| ZX Spectrum 48K | the original, with the snow effect |
| ZX Spectrum 128K | shadow screen, AY sound, paging |
| ZX Spectrum +2A / +3 | the later Amstrad machine, its own contention pattern |
| Pentagon 128 | the Russian clone, its own geometry and interrupt timing |
| Pentagon 512 | 32 memory banks |

## The sound

Seven devices, from the simple one to a whole computer on a card:

- **Beeper** - the one-bit speaker of the original.
- **AY-3-8912** - the chip inside the 128K, in stereo, with a DC block.
- **TurboSound** - two AY chips.
- **TurboSound FM** - a YM2203, two AY-style chips plus FM.
- **Covox** - the simple digital-to-analogue card.
- **SounDrive** - the four-channel digital card.
- **General Sound** - a card with its own Z80 processor, its own 512 KB of
  memory and four DAC channels; it runs its own programs independently of the
  Spectrum.

## Storage and media

Everything is loaded as it comes; a `.zip` is opened for you and the right file
inside it is picked (tape, then disk, then snapshot, then ROM), and it tells you
which one it used.

| what | formats |
|---|---|
| Tape | .tap, .tzx |
| TR-DOS disks | .trd, .scl |
| +3 disks | .dsk, .edsk, including extended images with weak-bit protection |
| Snapshots | .sna, .z80 (both with the 128K extension) |

Both disk controllers work end to end - cataloguing, loading, running, formatting
and track-level access - and copy-protected releases load and run. The tape
plays the way a real one does: type `LOAD ""` and press play.

## Sessions and replays

**Sessions.** The emulator remembers where you were. When you exit, it writes the
whole machine out - memory, processor, screen, sound chips, drives with their
disks, the tape position - and puts it back the next time you start. One file per
model, so switching machines does not throw away the other one's place. Saving
and loading a session by name is a switch away.

The state is exact, not approximate: two machines started from the same session
run on identically, frame for frame and sample for sample, which is how it is
tested.

**Replays.** A `.replay` file is a recording of a session: every key press, every
joystick move and every tape action, each stamped with the moment it happened in
*emulated* time, plus the machine it happened on and the media it used. Hand one
to someone else and they see exactly what you did.

```bash
./zxgo -model 128k -tap game.tzx -save-replay myrun.replay   # record
./zxgo -replay myrun.replay                                  # watch it back
```

`-replay` alone is enough: the file carries its own machine, mounts the media the
session used and plays the input back at the right moments. Because the stamps
are in emulated time and not wall-clock time, a replay of a fast tape load looks
the same on a slow machine as on a fast one - and the keys are stored by position,
not by host key, so your keyboard layout does not travel with it.

## Loading things quickly

Two switches skip emulated waiting when you do not care about the accuracy of the
waiting:

```bash
./zxgo -model 48k -tap game.tap -fast-tape       # tape loads at host speed
./zxgo -model pentagon -disk game.trd -no-fdc-timing   # disk loads without seek and rotation delay
```

`-fast-tape` runs unthrottled while a tape plays, so a five-minute load takes a
few seconds; sound is dropped for the duration. `-no-fdc-timing` does the same for
the floppy controller, which helps on a large disk image that does not need to be
loaded accurately. Both have buttons in the tape and disks windows, so you can
turn them on and off mid-session.

## Quick reference

```bash
go build -o zxgo ./cmd/zxgo

./zxgo                                      # 48K
./zxgo -model 128k                          # 128K
./zxgo -model 2a3 -disk game.dsk            # +3 disk
./zxgo -model pentagon -disk game.trd       # TR-DOS disk
./zxgo -model 48k -tap game.tzx             # tape; CMD+P (on macOS) starts it
./zxgo -model 48k -tap game.tzx -fast-tape  # the same load, at host speed
./zxgo -model pentagon -turbosound -gs      # add sound cards
```

`-model` takes `48k`, `128k`, `2a3`, `pentagon` or `pentagon512`. With no
`-model`, the emulator starts the machine you used last.

Loading a tape is two steps, as on the real machine: `-tap` puts the tape in and
does not start it, then type `LOAD ""` on the emulated machine and press play
(the tape window has the transport; the default key is CMD+P). A disk is mounted
by `-disk` and needs no further step - at the TR-DOS prompt, `CAT` and `RUN`, or
`RANDOMIZE USR 15616` from BASIC to get to TR-DOS.

Sound cards are switches: `-turbosound`, `-turbosoundfm`, `-gs` (General Sound)
and `-snow` for the 48K snow artefact. Everything a switch sets is also in the
settings window (`Cmd+,`), and either place is remembered.

**Joysticks.** Plug in a gamepad and it just works: a pad the system recognises is
opened automatically - including one connected while the emulator is already
running - and drives the machine's Kempston joystick, with the D-pad or the stick
for directions and the buttons for fire.

## More of what it does

- **A desktop front end**, one window per tool: the machine's screen and menubar,
  machine control, a disk window showing the controller's live state, a tape
  window with the block list, a clickable keyboard, settings and file dialogs.
- **Key bindings you can change.** Two small files in the config directory list
  only your changes: `keymap.json` for what a host key does, `matrix.json` for
  which machine key a host key stands for.
- **Screenshots** and **profiling** on request (`-prof` writes eight profiles when
  the emulator exits, with no cost at all when it is off).
- **Logging** per subsystem with `-log debug`, for when something does not do what
  you expect.
- **A documented session format** (`ZXSTATE_SPEC.md`), so another program can read
  what this one wrote.
- **A headless mode** for driving the emulator from scripts and from Claude Code:
  it can be stepped instruction by instruction, its registers and memory read and
  written, its screen read as text, its input driven and its execution traced, all
  without a window (`MCP_SERVER.md`).

## Reading more

**[MANUAL.md](MANUAL.md)** is everything a user needs: every switch, every window,
every key binding, sessions, replays and profiling.

## Screenshots

![screenshot](screenshots/screenshot-20260916-091828.791.png)
![screenshot](screenshots/screenshot-20260916-091923.013.png)
![screenshot](screenshots/screenshot-20260916-092006.465.png)
![screenshot](screenshots/screenshot-20260916-092141.776.png)
