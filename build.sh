#!/usr/bin/env bash
#
# build.sh - cross-compile zxgo into self-contained binaries for macOS, Linux
# and Windows, linking SDL3 statically from the vendored archives in static/.
#
# Outputs (repository root unless OUT_DIR is set):
#   zxgo-macos         macOS arm64 (Apple Silicon)
#   zxgo-linux         Linux x86-64
#   zxgo-windows.exe   Windows x86-64 (PE32+)
#
# "Static" here means SDL3 is linked into the executable, so none of these
# binaries needs an SDL3 library installed. System libraries stay dynamic:
# macOS frameworks, glibc, and the Windows CRT are part of the OS.
#
# Usage:
#   ./build.sh                 build all three targets
#   ./build.sh macos           build one target (macos | linux | windows)
#   ./build.sh linux windows   build several
#   OUT_DIR=dist ./build.sh    write the binaries elsewhere
#
# Environment:
#   OUT_DIR             output directory (default: the repository root)
#   SDL3_INCLUDE        directory containing the SDL3/ header dir
#   ZXGO_STATIC_CACHE   build cache root (default: ~/.cache/zxgo-static-build)
#
# Toolchains:
#   macos    clang from the Xcode command line tools (any dev Mac)
#   linux    zig, used as the cross C compiler:   brew install zig
#   windows  mingw-w64 gcc:                       brew install mingw-w64
#
# The SDL3 headers must match the vendored archives (3.4.x). Per-platform
# headers in static/<dir>/include take priority; otherwise the headers of a
# host SDL3 installation are used.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STATIC_DIR="$ROOT/static"
OUT_DIR="${OUT_DIR:-$ROOT}"
CACHE_ROOT="${ZXGO_STATIC_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/zxgo-static-build}"

# SDL3 release the vendored archives were built from, for the generated .pc.
SDL3_VERSION="3.4.17"

# The Linux link needs glibc 2.38 or newer: the vendored archive references the
# C23 entry points (__isoc23_strtoll and friends) that glibc only added in 2.38.
LINUX_GLIBC="2.38"

WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/zxgo-build.XXXXXX")"
trap 'rm -rf "$WORKDIR"' EXIT

die() {
    echo "build.sh: $*" >&2
    exit 1
}

# static_libdir <platform> - locate the vendored archive directory. The Linux
# directory is spelled "linux-adm64" in the tree; accept "linux-amd64" too.
static_libdir() {
    local dir="$1"
    [ -d "$STATIC_DIR/$dir" ] && { echo "$STATIC_DIR/$dir"; return 0; }
    if [ "$dir" = "linux-adm64" ] && [ -d "$STATIC_DIR/linux-amd64" ]; then
        echo "$STATIC_DIR/linux-amd64"
        return 0
    fi
    return 1
}

# resolve_include <platform> - directory that contains the SDL3/ header dir.
# Runs before PKG_CONFIG_LIBDIR is pointed at our generated .pc, so a host
# pkg-config still sees the real SDL3 installation here.
resolve_include() {
    local plat="$1" inc

    if [ -n "${SDL3_INCLUDE:-}" ]; then
        echo "$SDL3_INCLUDE"
        return 0
    fi
    if [ -d "$STATIC_DIR/$plat/include/SDL3" ]; then
        echo "$STATIC_DIR/$plat/include"
        return 0
    fi

    inc="$(pkg-config --variable=includedir sdl3 2>/dev/null || true)"
    if [ -n "$inc" ] && [ -f "$inc/SDL3/SDL.h" ]; then
        echo "$inc"
        return 0
    fi
    for inc in /opt/homebrew/include /usr/local/include /usr/include; do
        if [ -f "$inc/SDL3/SDL.h" ]; then
            echo "$inc"
            return 0
        fi
    done

    return 1
}

# run_build <name> <goos> <goarch> <cc> <libdir> <libs> <extldflags>
#
# Each target gets its own GOCACHE. This is not a nicety: Go folds the
# pkg-config flags into a cached cgo link step, so sharing a cache with a
# regular host build silently produces a binary linked against the *dynamic*
# host SDL3 instead of the vendored archive.
run_build() {
    local name="$1" goos="$2" goarch="$3" cc="$4" libdir="$5" libs="$6"
    local extldflags="$7"

    # Windows refuses to run a PE without the extension, even though it is a
    # perfectly valid executable without one.
    local outname="$name"
    [ "$goos" = "windows" ] && outname="$name.exe"

    [ -f "$libdir/libSDL3.a" ] || die "missing $libdir/libSDL3.a"

    local pcdir="$WORKDIR/pc-$name"
    mkdir -p "$pcdir"
    cat >"$pcdir/sdl3.pc" <<EOF
prefix=$libdir
libdir=\${prefix}
includedir=$SDL3_INCLUDE

Name: sdl3
Description: SDL3 cross-compiled static build for $name
Version: $SDL3_VERSION
Libs: $libs
Cflags: -I\${includedir} -DSDL_MAIN_HANDLED
EOF

    # SDL_MAIN_HANDLED above is required on Windows: without it SDL_main.h
    # rewrites main() into SDL_main(), which nothing then provides. The cgo
    # code already calls SDL_SetMainReady(), so this matches its intent.

    local ldflags="-s -w"
    if [ -n "$extldflags" ]; then
        ldflags="$ldflags -linkmode external -extldflags '$extldflags'"
    fi

    echo "==> $name  ($goos/$goarch)"
    echo "    cgo:        $cc"
    echo "    SDL3:       $libdir/libSDL3.a"
    echo "    headers:    $SDL3_INCLUDE"
    echo "    cache:      $CACHE_ROOT/$name"

    GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=1 CC="$cc" \
        PKG_CONFIG_LIBDIR="$pcdir" GOCACHE="$CACHE_ROOT/$name" \
        go build -trimpath -ldflags="$ldflags" -o "$OUT_DIR/$outname" ./cmd/zxgo

    report "$name" "$outname"
}

# report <name> <outname> - describe the artifact and confirm SDL3 was not left
# dynamic.
report() {
    local name="$1" out="$OUT_DIR/$2"
    local size
    size="$(wc -c <"$out" | tr -d ' ')"
    echo "    -> $out (${size} bytes)"

    case "$name" in
    zxgo-macos)
        if command -v otool >/dev/null 2>&1; then
            if otool -L "$out" | grep -q libSDL3; then
                die "$name still links a dynamic libSDL3"
            fi
            echo "    static SDL3 confirmed (only system frameworks)"
        fi
        ;;
    zxgo-linux)
        local needed=""
        if command -v llvm-readelf >/dev/null 2>&1; then
            needed="$(llvm-readelf --dynamic-table "$out" 2>/dev/null | grep -i libSDL3 || true)"
        else
            # NEEDED names sit in .dynstr as standalone strings.
            needed="$(strings -a "$out" | grep -x 'libSDL3\.so[^ ]*' || true)"
        fi
        if [ -n "$needed" ]; then
            die "$name still links a dynamic libSDL3"
        fi
        echo "    static SDL3 confirmed (needs libc.so.6, libm.so.6)"
        ;;
    zxgo-windows)
        if command -v x86_64-w64-mingw32-objdump >/dev/null 2>&1; then
            if x86_64-w64-mingw32-objdump -p "$out" 2>/dev/null | grep -qi "DLL Name: SDL3"; then
                die "$name still links a dynamic SDL3.dll"
            fi
            echo "    static SDL3 confirmed (only system DLLs)"
        fi
        ;;
    esac
    echo
}

build_macos() {
    local libdir
    libdir="$(static_libdir macos-silicon)" ||
        die "missing static/macos-silicon (the vendored macOS arm64 libSDL3.a)"
    SDL3_INCLUDE="$(resolve_include macos-silicon)" ||
        die "no SDL3 headers found; set SDL3_INCLUDE to the dir containing SDL3/"

    # SDL3 uses Cocoa, Metal, CoreAudio and friends at runtime; each one needs
    # its framework linked explicitly now that libSDL3 is an archive.
    # UniformTypeIdentifiers and CoreHaptics are declared weak upstream, but
    # cgo rejects -weak_framework, and both exist on every arm64 macOS.
    local libs="-L$libdir -lSDL3"
    libs="$libs -Wl,-framework,CoreMedia -Wl,-framework,CoreVideo"
    libs="$libs -Wl,-framework,Cocoa -Wl,-framework,UniformTypeIdentifiers"
    libs="$libs -Wl,-framework,IOKit -Wl,-framework,ForceFeedback"
    libs="$libs -Wl,-framework,Carbon -Wl,-framework,CoreAudio"
    libs="$libs -Wl,-framework,AudioToolbox -Wl,-framework,AVFoundation"
    libs="$libs -Wl,-framework,Foundation -Wl,-framework,GameController"
    libs="$libs -Wl,-framework,Metal -Wl,-framework,QuartzCore"
    libs="$libs -Wl,-framework,CoreHaptics -lobjc -lpthread -lm"

    run_build zxgo-macos darwin arm64 cc "$libdir" "$libs" ""
}

build_linux() {
    local libdir
    libdir="$(static_libdir linux-adm64)" ||
        die "missing static/linux-adm64 (the vendored Linux amd64 libSDL3.a)"
    SDL3_INCLUDE="$(resolve_include linux-adm64)" ||
        die "no SDL3 headers found; set SDL3_INCLUDE to the dir containing SDL3/"
    command -v zig >/dev/null 2>&1 || die "zig not found; install it with: brew install zig"

    local cc="zig cc -target x86_64-linux-gnu.$LINUX_GLIBC"

    # No -extldflags -static here, on purpose. SDL3 loads its video and audio
    # backends (X11, Wayland, ALSA, PulseAudio) through dlopen at runtime, and
    # a fully static link would have to resolve those at build time; the musl
    # target that would allow -static cannot dlopen at all. Linking SDL3 in
    # statically while glibc stays dynamic is what keeps both properties.
    run_build zxgo-linux linux amd64 "$cc" "$libdir" "-L$libdir -lSDL3 -lm -ldl -lpthread" ""
}

build_windows() {
    local libdir
    libdir="$(static_libdir windows-amd64)" ||
        die "missing static/windows-amd64 (the vendored Windows amd64 libSDL3.a)"
    SDL3_INCLUDE="$(resolve_include windows-amd64)" ||
        die "no SDL3 headers found; set SDL3_INCLUDE to the dir containing SDL3/"
    command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1 ||
        die "mingw-w64 not found; install it with: brew install mingw-w64"

    # Win32 import libraries SDL3 pulls in. Derived from the archive's
    # undefined symbols; the linker ignores any that go unused.
    local libs="-L$libdir -lSDL3 -lmingw32"
    libs="$libs -lsetupapi -limm32 -lversion -lole32 -loleaut32 -luuid"
    libs="$libs -lwinmm -lgdi32 -luser32 -lkernel32 -ladvapi32 -lcfgmgr32"
    libs="$libs -lshell32 -lshlwapi -lws2_32 -ldinput8 -lhid -lsecur32"
    libs="$libs -lbcrypt -luserenv -lntdll -lksuser -lmmdevapi -lavrt"
    libs="$libs -lwtsapi32 -lshcore -ldwmapi -loleacc -lcomdlg32"
    libs="$libs -lcomctl32 -ldxguid -lgdiplus -lmsimg32 -lwinhttp"

    run_build zxgo-windows windows amd64 x86_64-w64-mingw32-gcc "$libdir" "$libs" "-static"
}

usage() {
    cat <<'EOF'
build.sh - cross-compile zxgo for macOS, Linux and Windows with SDL3 linked in.

Usage:
  ./build.sh                 build all three targets
  ./build.sh macos           build one target (macos | linux | windows)
  ./build.sh linux windows   build several
  OUT_DIR=dist ./build.sh    write the binaries elsewhere

Outputs: zxgo-macos, zxgo-linux, zxgo-windows.exe (see the header of this file).
EOF
}

main() {
    cd "$ROOT"

    if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
        usage
        exit 0
    fi

    local requested=("$@")
    if [ ${#requested[@]} -eq 0 ]; then
        requested=(macos linux windows)
    fi

    for target in "${requested[@]}"; do
        case "$target" in
        macos | linux | windows) ;;
        *) die "unknown target '$target' (expected macos, linux or windows)" ;;
        esac
    done

    mkdir -p "$OUT_DIR"

    for target in "${requested[@]}"; do
        "build_$target"
    done

    echo "done: ${requested[*]}"
}

main "$@"
