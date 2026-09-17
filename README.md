# zxgo-v3

A ZX Spectrum emulator written in Go.

Third generation of the project (after `zxgo` in Go and `zxcpp` in C++), written
from scratch .

## What it can do

**Machines**: 48K, 128K, +2A/+3, Pentagon 128 and Pentagon 512.

**Video**: per-T-state ULA rendering with per-model geometry (the picture sits
where the machine's own timing puts it, not where a fixed 352x288 window does),
border effects, contention, the floating bus, and the 48K "snow" artefact
(opt-in).

**Sound**: beeper and AY-3-8912 resolved onto a fixed sample grid, stereo with a
DC block, plus TurboSound, TurboSound FM (YM2203), Covox, SounDrive and the
General Sound card.

**Storage**: TR-DOS (WD1793) with multi-sector
transfers; the +3 uPD765 controller with .DSK images (standard and extended,
including weak-bit protection); tape from .TAP and .TZX.

## Build and run

Requires Go and SDL3, which `build.sh` links statically from `static/` on Linux,
macOS and Windows.

```bash
go build -o zxgo ./cmd/zxgo

./zxgo -model 128k                      # 128k
./zxgo -model 48k -tap game.tzx         # load a tape, CMD+P to play
./zxgo -model pentagon -disk game.trd   # TR-DOS disk
./zxgo -model 2a3 -disk game.dsk        # +3 disk
./zxgo -model 48k -tap game.tap -fast-tape   # load at host speed
./zxgo -model 48k -tap game.tzx.zip     # zipped images load as they are
```

Every image loader also takes a `.zip`: `-tap`, `-sna`, `-z80` and `-disk`
unpack the archive and pick the file to load by its name inside, so a release
that ships packed needs no unpacking first. An archive holding several images
(a 48k and a 128k tape, say) loads the first; the entry used is printed.

Useful switches: `-turbosound`, `-turbosoundfm`, `-gs`, `-log debug` for
subsystem logging.

### Replays

A `.replay` file is a recording of a session: every key press with the moment it
happened, plus the machine it happened on. Hand one to someone else and they see
what you did.

```bash
./zxgo -model 128k -tap game.tzx -save-replay myrun.replay   # record
./zxgo -replay myrun.replay                                  # watch it back
```

The file carries the model and the switches, so `-replay` alone is enough: it
builds the recorded machine, mounts the media the session used, and plays the
keys at the ticks they were pressed. A switch given on the command line is added
to the session's, and `-model` overrides the recorded one (the timings are then
scaled by the clock ratio, and it says so).

Keys are stored as matrix positions and stamped in T-states, not host time, so a
replay is independent of your keyboard layout and of how fast your machine is --
including across a `-fast-tape` load, which is where host and emulated time part
company. An archive that ships without its tape still plays its keys.

## Screenshots

![screenshot](screenshots/screenshot-20260916-091828.791.png)
![screenshot](screenshots/screenshot-20260916-091923.013.png)
![screenshot](screenshots/screenshot-20260916-092006.465.png)
![screenshot](screenshots/screenshot-20260916-092141.776.png)

