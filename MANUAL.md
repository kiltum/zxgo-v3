# zxgo-v3 manual

How to run the emulator, load software, set it up, change the key bindings and
profile it. Everything a user needs is in this file.

## What it can do

**Machines**: 48K, 128K, +2A/+3, Pentagon 128 and Pentagon 512.

**Video**: per-T-state ULA rendering with per-model geometry (the picture sits
where the machine's own timing puts it, not where a fixed 352x288 window does),
border effects, contention, the floating bus, and the 48K "snow" artefact
(opt-in).

**Sound**: beeper and AY-3-8912 resolved onto a fixed sample grid, stereo with a
DC block, plus TurboSound, TurboSound FM (YM2203), Covox, SounDrive and the
General Sound card.

**Storage**: TR-DOS (WD1793) with multi-sector transfers; the +3 uPD765 controller
with .DSK images (standard and extended, including weak-bit protection); tape from
.TAP and .TZX. Every loader also takes a `.zip`, and picks the file to load by its
name inside - tape before disk before snapshot before ROM - so a release that ships
packed needs no unpacking first. An archive holding several images (a 48k and a
128k tape, say) loads the first, and prints which entry it used.

**Front end**: a desktop window per tool over Dear ImGui - the machine's screen, a
machine-control window, a disks window with the controller's live state, a tape
window with the block list, an on-screen ZX keyboard, a settings window, and the
file dialogs. The window layout, the key bindings and the settings are files in the
config directory, and the machine's state is carried between runs.

## Build and run

You need Go (1.26 or later), SDL3 with its development headers, `pkg-config`, and a
C++ compiler - the desktop windows compile the vendored Dear ImGui sources through
cgo. On macOS and Linux that compiler is part of the normal toolchain (Xcode command
line tools, or `build-essential`).

```bash
# SDL3, one of:
brew install sdl3 pkg-config            # macOS
sudo apt install libsdl3-dev pkg-config # Debian / Ubuntu
sudo dnf install SDL3-devel pkg-config  # Fedora
sudo pacman -S sdl3 pkg-config          # Arch
# Windows: MSYS2, then
#   pacman -S mingw-w64-x86_64-SDL3 mingw-w64-x86_64-pkg-config

go build -o zxgo ./cmd/zxgo

./zxgo                                  # 48K, desktop windows
./zxgo -model 128k                      # 128k
./zxgo -model 48k -tap game.tzx         # load a tape, CMD+P to play
./zxgo -model pentagon -disk game.trd   # TR-DOS disk
./zxgo -model 2a3 -disk game.dsk        # +3 disk
./zxgo -model 48k -tap game.tap -fast-tape   # load at host speed
./zxgo -model 48k -tap game.tzx.zip     # zipped images load as they are
```

`-model` takes `48k`, `128k`, `2a3`, `pentagon` or `pentagon512`. With no `-model`
the emulator starts the machine that was used last.

`./build.sh` builds release binaries for macOS, Linux and Windows with SDL3 linked
in from `static/`, which is what the packaged releases are made with; a plain
`go build` needs SDL3 installed on the machine you run it on.

### The switches

| switch | what it does |
|---|---|
| `-model <name>` | which machine to build (see above) |
| `-tap <file>` | load a tape (.tap/.tzx, or a .zip holding one); it does not start playing |
| `-disk <file>` | mount a disk (.trd/.scl/.dsk/.edsk, or a .zip) in the first drive |
| `-sna`, `-z80 <file>` | load a snapshot |
| `-save-sna <file>` | write a snapshot on exit |
| `-fast-tape` | run unthrottled while a tape plays, so a load takes host time; sound is dropped for the duration |
| `-no-fdc-timing` | drop the floppy controller's seek and rotation latency, for a big disk image that does not need loading accurately |
| `-turbosound`, `-turbosoundfm`, `-gs`, `-snow` | add a sound card, or the 48K snow artefact |
| `-replay <file>`, `-save-replay <file>` | play or record a session (below) |
| `-session`, `-load-state`, `-save-state <file>` | name a session file explicitly instead of the config directory's |
| `-log debug` | per-subsystem logging to stderr |
| `-prof` | write profiling files on exit (below) |

Everything the switches set is also settable in the settings window (`Cmd+,`), and
either place is remembered; a switch given on the command line wins for the run it
is given on. `-fast-tape` and `-no-fdc-timing` are also buttons in the tape and
disks windows - one setting with two places to change it.

## The windows

- **Main** - the machine's screen and the menubar. ESC shows and hides the menubar.
- **Machine control** (`Cmd+M`) - pause/resume, step one instruction, reset, NMI,
  fast tape.
- **Disks** (`Cmd+D`) - what is in the drive, the controller's live state, mount and
  eject, and the disk-timing switch.
- **Tape** (`Cmd+T`) - the file, the position, the block list, and the transport.
  The tape loads the way a real one does: type `LOAD ""`, then `Cmd+P` to start it.
- **Keyboard** (`Cmd+K`) - the machine's forty keys, clickable. The window never
  takes the keyboard, so you can play with the mouse and type on the host keyboard
  at the same time.
- **Key bindings** (`Cmd+B`) - reserved for the editor; today it draws a placeholder,
  so bindings are edited in the file (see "Key bindings" below).
- **Settings** (`Cmd+,`) - the machine, the Z80 variant, the MEMPTR behaviour, the
  sound devices, the two speed switches and the session. Rows that say "restart
  required" are applied by the **Relaunch** button; the rest take effect at once.
- **Debugger** (`Cmd+G`) - reserved for the debugger, which is not built yet.

## Sessions

The emulator saves the machine's state and its mounted media when it exits, and
resumes them at the next start, so it comes back to the game you were in. One
session file per model (`session-<model>.zxstate` in the config directory), so
switching machines does not throw the other one's place away. The **Session**
setting in the settings window turns the behaviour off: turning it off stops the
session being written when you exit.

`-load-state`, `-save-state` and `-session` name a file explicitly instead; a path
without the extension is given it. A session carries the machine, so one recorded
on a different model is refused with a message rather than half-applied - the
emulator starts clean.

## Replays

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
replay is independent of your keyboard layout and of how fast your machine is -
including across a `-fast-tape` load, which is where host and emulated time part
company. An archive that ships without its tape still plays its keys.

`File > Record replay...` starts a recording from the window. One started that way is
offset from the machine's history: it reproduces the inputs from that moment, not the
whole session, so hand it over together with the session it came from. A recording
made with `-save-replay` on the command line has no such offset.

## Files the emulator keeps

Everything is in one directory per platform, named `zxgo-v3`:

| System | Path |
|---|---|
| macOS | `~/Library/Application Support/zxgo-v3/` |
| Linux | `~/.config/zxgo-v3/` (or `$XDG_CONFIG_HOME/zxgo-v3/`) |
| Windows | `%AppData%\zxgo-v3\` |

| file | who writes it |
|---|---|
| `session-<model>.zxstate` | the emulator, on a graceful exit |
| `layout.json` | the emulator, when a window moves or opens |
| `settings.json` | the emulator, when a setting changes |
| `keymap.json` | **you** (see below) |
| `matrix.json` | **you** |

## Key bindings

Two files, both read at startup, both listing **only your changes** against what the
emulator ships with:

- `keymap.json` - what a *host* key does: quit, pause, open a window, and so on.
- `matrix.json` - which key of the *machine* a host key stands for.

```json
// keymap.json
{
  "version": 1,
  "bindings": {
    "cmd+shift+s": "screenshot",
    "cmd+q": "none",
    "f5": "none",
    "f7": "reset",
    "ctrl+p": "pause-toggle"
  }
}
```

```json
// matrix.json
{
  "version": 1,
  "keys": {
    "backspace": "symbol-shift+x",
    "up":        "caps-shift+7",
    "tab":       "none"
  }
}
```

- Anything you do not mention keeps its default, so **moving a key takes two
  entries**: bind the action to the new key *and* unbind the old one.
- `"none"` removes a binding or a matrix entry the emulator ships with.
- A binding is modifiers and a key joined with `+`: `cmd+shift+q`, `f5`,
  `ctrl+alt+delete`. In `keymap.json`, `shift` means the modifier *you* hold.
- A matrix entry is a **chord of machine keys** joined with `+`, because a ZX
  Spectrum has no shift-modified keys: what the PC calls BACKSPACE is CAPS SHIFT
  and 0 (the machine's DELETE), and the cursor keys are CAPS SHIFT with 5-8.
- A file that cannot be read at all is reported on startup and ignored - the
  emulator starts with the defaults rather than refusing to run. Only the entries
  it could not read are skipped.

**Modifiers:** `cmd` (also `super`, `win`, `meta`), `ctrl` (also `control`), `alt`
(also `opt`, `option`), `shift`.

**Key names:** letters `a`-`z` and digits `0`-`9` as themselves, plus

```
backspace  delete  down  end  enter  escape  home  insert  left  pagedown
pageup  right  space  tab  up  f1-f12
,  -  .  /  ;  =  [  \  ]  `
lshift  rshift  lctrl  rctrl  lalt  ralt  lcmd  rcmd
```

**Actions** (the right-hand side of `keymap.json`) - the full set:

```
quit                  reset                 nmi                   pause-toggle
step-one              turbo-toggle          screenshot            log-mark
record-replay         open-tape             tape-playpause        tape-rewind
open-disk             eject-disk            open-snapshot         save-snapshot
toggle-control        toggle-disks          toggle-tape           toggle-keyboard
toggle-bindings       toggle-settings       toggle-debugger       fdc-timing-toggle
model-48k             model-128k            model-2a3             model-pentagon
model-pentagon512     z80-nmos              z80-cmos              memptr-real
memptr-documented     turbosound-toggle     turbosoundfm-toggle   gs-toggle
snow-toggle           session-toggle        relaunch              none
```

The settings window's rows are actions like any other, so any of them can have a
key. `model-*` and the sound `*-toggle` switches need a relaunch to take effect
(the machine's memory map and its sound cards are built once), while `z80-*`,
`memptr-*`, `turbo-toggle`, `fdc-timing-toggle` and `session-toggle` apply at once.

**Machine key names** (the right-hand side of `matrix.json`): the key legends,
lowercased - `a-z`, `0-9`, `caps-shift`, `symbol-shift`, `space`, `enter`.

**What the matrix does by default**, so you know what you are changing:

| host key | machine keys | what it is |
|---|---|---|
| `a`-`z`, `0`-`9` | themselves | the same letters and digits |
| `enter`, `space` | ENTER, SPACE | |
| `lshift` | CAPS SHIFT | |
| `rshift` | SYMBOL SHIFT | |
| `backspace`, `delete` | CAPS SHIFT + 0 | DELETE |
| `up` / `down` / `left` / `right` | CAPS SHIFT + 7/6/5/8 | the cursor keys |

**Escape is deliberately left out of the matrix**: it is the emulator's own key (it
shows and hides the menubar), so a mapping for it would never fire. So are the
combinations the operating system keeps for itself (`cmd+space`, `cmd+tab`,
`alt+tab`): a binding for one of those will never fire either.

## Profiling

`-prof` writes eight profiles to the working directory on exit:

```bash
./zxgo -prof
go tool pprof -text cpu.prof | head -20       # hottest functions
go tool pprof -http=:8080 mem.prof            # interactive
go tool trace trace.out                       # the execution trace
```

The files are `cpu.prof`, `mem.prof`, `heap.prof`, `allocs.prof`,
`goroutine.prof`, `block.prof`, `mutex.prof` and `trace.out`, written when the
emulator exits. With the flag off there is no cost at all; with it on the machine
still runs normally, and the profiles describe it.
