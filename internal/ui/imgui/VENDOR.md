# Vendored Dear ImGui

Upstream: https://github.com/ocornut/imgui
Tag:      v1.92.9b
Commit:   f1cc2ae15e53a861a874c3034aae6798fde194ab
Dated:    2026-07-31

Nothing in this directory is ours. Every file below was copied verbatim from that
commit and is replaced wholesale on an upgrade; the Go sources in this directory
(the package `imgui`) are ours and are the only files an upgrade does not touch.

## Files

    LICENSE.txt
    imconfig.h  imgui.h  imgui_internal.h
    imstb_rectpack.h  imstb_textedit.h  imstb_truetype.h
    imgui.cpp  imgui_draw.cpp  imgui_tables.cpp  imgui_widgets.cpp
    imgui_impl_sdl3.cpp  imgui_impl_sdl3.h
    imgui_impl_sdlrenderer3.cpp  imgui_impl_sdlrenderer3.h

`imgui_demo.cpp` is not vendored: it is a demo, it is not compiled into the
emulator, and leaving it out keeps `SayHello`-style sample code out of the tree.

## Why the sources are in the package directory

cgo compiles the `.c`/`.cpp` files that sit in a package's own directory, and only
those. The upstream files have to be here for `go build` to compile them, which is
also why they are not under `third_party/`: a vendored tree elsewhere would need a
build step of its own to be compiled at all. The `#include`s are then relative
neighbours, which is how upstream expects them to be laid out.

## Why a tagged release and not the docking branch

UI_DESIGN.md D1 gives each window its own ImGui context instead of using
multi-viewport, so the docking branch is not needed and a tagged release is
enough - the smaller dependency to track.

That is a deliberate departure from upstream's own advice: `imgui_impl_sdlrenderer3.cpp:72`
says "It is STRONGLY preferred that you use docking branch with multi-viewports
(== single Dear ImGui context + multiple windows) instead of multiple Dear ImGui
contexts." The documentation for multiple contexts is nonetheless complete and the
backends support it by construction (see below), which is what D1 was betting on.

## What was verified before writing any Go

Two claims in UI_DESIGN.md D1 were checked in the vendored source rather than
assumed:

1. **The backend data is per context.** `imgui_impl_sdl3.cpp:139` -
   "Backend data stored in io.BackendPlatformUserData to allow support for multiple
   Dear ImGui contexts" - and `ImGui_ImplSDL3_GetBackendData()` reads it back from
   `ImGui::GetIO()` of the *current* context. The struct holds `SDL_Window* Window`,
   its `WindowID` and `SDL_Renderer* Renderer`, so window and renderer are per
   context. `imgui_impl_sdlrenderer3.cpp:71` does the same for the renderer in
   `io.BackendRendererUserData`. `ImGui_ImplSDL3_InitForSDLRenderer(window, renderer)`
   and `ImGui_ImplSDLRenderer3_Init(renderer)` take both. **D1 holds; no docking
   branch.**
2. **`ImGui_ImplSDL3_ProcessEvent` filters by window itself.** Every event type
   goes through `ImGui_ImplSDL3_GetViewportForWindowID(...)` and is dropped unless
   the id matches this context's window. So the backend broadcasts every SDL event
   to every context and each context ignores what is not its own - no hand-written
   dispatch by windowID, and no way for an event to be lost to the wrong context.

The `NOT_FOCUSABLE` risk of UI_DESIGN.md section 12 was checked too, and is
benign: `ImGui_ImplSDL3_UpdateMouseData` gates only the "focused but not hovered"
fallback on `SDL_GetKeyboardFocus()`; positions arrive through
`SDL_EVENT_MOUSE_MOTION`, which does not depend on keyboard focus. A not-focusable
window therefore still sees the mouse, which is what M5 needs. It is not provable
headlessly, so the M5 *done when* stays the place it is checked for real.

## Upgrading

1. Copy the file list above from the new tag.
2. Update the tag, commit and date here.
3. Re-check the two claims above if the backends have changed.
4. `go build ./...` and run the smoke test (`go test ./internal/ui/imgui/`).
