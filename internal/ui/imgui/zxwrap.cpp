// zxwrap.cpp is the C++ side of the ImGui backend: it owns one SDL window, one
// SDL renderer and one ImGui context per view, and exposes them to Go through the
// small extern "C" API declared in imgui.go.
//
// It is deliberately mechanical. Every decision - which windows exist, where they
// are, what the menubar says, whether to present - belongs to internal/frontend,
// and this file only carries it out (UI_DESIGN.md section 5.4). Nothing here reads
// App state or invents an item.
//
// One context per window is UI_DESIGN.md D1. The backends support it by
// construction: each keeps its SDL_Window and SDL_Renderer in per-context backend
// data, so every call below sets the context it means to talk to first. See
// VENDOR.md for the two places that was checked in the vendored source.

#include "imgui.h"
#include "imgui_impl_sdl3.h"
#include "imgui_impl_sdlrenderer3.h"

#include <SDL3/SDL.h>

#include <math.h>
#include <stdlib.h>
#include <string.h>

// ZxView is one OS window: its SDL window, its renderer, and the ImGui context
// bound to both.
struct ZxView
{
    SDL_Window*   Window;
    SDL_Renderer* Renderer;
    ImGuiContext* Ctx;
    SDL_WindowID  WindowID;

    // The machine's framebuffer, kept as a streaming texture so a frame is one
    // upload rather than a texture created per present.
    SDL_Texture*  Screen;
    int           ScreenW;
    int           ScreenH;

    // IniFilename is copied: ImGuiIO holds the pointer for as long as the context
    // lives, so it cannot be the caller's memory.
    char*         IniPath;
    bool          InFrame;
};

extern "C" {

// zx_view_new creates a window with a size and no position: SDL_CreateWindow has no
// position argument, and leaving it to the window manager is what a first launch
// wants. zx_view_set_geometry places it once there is somewhere to put it.
ZxView* zx_view_new(const char* title, int w, int h,
                    unsigned long long sdl_flags, const char* ini_path, int vsync)
{
    Uint64 flags = (Uint64)sdl_flags | SDL_WINDOW_RESIZABLE;
    SDL_Window* window = SDL_CreateWindow(title, w, h, flags);
    if (window == nullptr)
        return nullptr;

    SDL_Renderer* renderer = SDL_CreateRenderer(window, nullptr);
    if (renderer == nullptr)
    {
        SDL_DestroyWindow(window);
        return nullptr;
    }
    SDL_SetRenderVSync(renderer, vsync);

    ImGuiContext* ctx = ImGui::CreateContext();
    ImGui::SetCurrentContext(ctx);

    ImGuiIO& io = ImGui::GetIO();
    // One ini file per context: several contexts sharing one path each rewrite it
    // keeping only the entries they know, so the last writer wins and the rest are
    // dropped (UI_DESIGN.md section 5.6).
    if (ini_path != nullptr && ini_path[0] != '\0')
    {
        size_t n = strlen(ini_path) + 1;
        char* copy = (char*)malloc(n);
        memcpy(copy, ini_path, n);
        io.IniFilename = copy;
    }
    else
    {
        io.IniFilename = nullptr;
    }

    if (!ImGui_ImplSDL3_InitForSDLRenderer(window, renderer))
    {
        ImGui::DestroyContext(ctx);
        SDL_DestroyRenderer(renderer);
        SDL_DestroyWindow(window);
        return nullptr;
    }
    if (!ImGui_ImplSDLRenderer3_Init(renderer))
    {
        ImGui_ImplSDL3_Shutdown();
        ImGui::DestroyContext(ctx);
        SDL_DestroyRenderer(renderer);
        SDL_DestroyWindow(window);
        return nullptr;
    }

    ZxView* v = (ZxView*)calloc(1, sizeof(ZxView));
    v->Window = window;
    v->Renderer = renderer;
    v->Ctx = ctx;
    v->WindowID = SDL_GetWindowID(window);
    v->IniPath = (char*)io.IniFilename;
    return v;
}

void zx_view_free(ZxView* v)
{
    if (v == nullptr)
        return;
    ImGui::SetCurrentContext(v->Ctx);
    if (v->Screen != nullptr)
        SDL_DestroyTexture(v->Screen);
    ImGui_ImplSDLRenderer3_Shutdown();
    ImGui_ImplSDL3_Shutdown();
    ImGui::DestroyContext(v->Ctx);
    if (v->Renderer != nullptr)
        SDL_DestroyRenderer(v->Renderer);
    if (v->Window != nullptr)
        SDL_DestroyWindow(v->Window);
    free(v->IniPath);
    free(v);
}

unsigned int zx_view_id(ZxView* v) { return v == nullptr ? 0 : (unsigned int)v->WindowID; }

void zx_view_geometry(ZxView* v, int* x, int* y, int* w, int* h)
{
    if (v == nullptr)
        return;
    SDL_GetWindowPosition(v->Window, x, y);
    SDL_GetWindowSize(v->Window, w, h);
}

void zx_view_set_geometry(ZxView* v, int x, int y, int w, int h)
{
    if (v == nullptr)
        return;
    SDL_SetWindowPosition(v->Window, x, y);
    SDL_SetWindowSize(v->Window, w, h);
}

void zx_view_size(ZxView* v, int* w, int* h)
{
    if (v == nullptr)
        return;
    SDL_GetWindowSize(v->Window, w, h);
}

// zx_view_pixel_size reports the client area in pixels, which is what the screen
// is scaled to fill; the point size above is what a saved rect is measured in.
void zx_view_pixel_size(ZxView* v, int* w, int* h)
{
    if (v == nullptr)
        return;
    SDL_GetWindowSizeInPixels(v->Window, w, h);
}

void zx_view_show(ZxView* v) { if (v) SDL_ShowWindow(v->Window); }
void zx_view_hide(ZxView* v) { if (v) SDL_HideWindow(v->Window); }
void zx_view_raise(ZxView* v) { if (v) SDL_RaiseWindow(v->Window); }
int  zx_view_minimized(ZxView* v) { return v && (SDL_GetWindowFlags(v->Window) & SDL_WINDOW_MINIMIZED); }

// zx_view_focused reports whether this window is the one with keyboard focus. The
// front end needs it for the D5 routing, and only SDL knows.
int zx_view_keyboard_focused(ZxView* v) { return v && SDL_GetKeyboardFocus() == v->Window; }
int zx_view_mouse_focused(ZxView* v) { return v && SDL_GetMouseFocus() == v->Window; }

int zx_view_process_event(ZxView* v, SDL_Event* event)
{
    if (v == nullptr)
        return 0;
    ImGui::SetCurrentContext(v->Ctx);
    return ImGui_ImplSDL3_ProcessEvent(event) ? 1 : 0;
}

// ---------------------------------------------------------------- file dialogs
//
// SDL's dialog is asynchronous: it returns immediately and the answer arrives in a
// callback that "may be called from a different thread than the one the function was
// invoked on" (SDL_dialog.h). So the answer goes into a mutex-protected slot that the
// next zx_dialog_take drains, and nothing is written into the caller's memory from
// the callback - which is the first of the four constraints UI_DESIGN.md D2 lists.
//
// The filters are file-static because SDL requires them to "remain valid at least
// until the callback is invoked", and their patterns are string literals, which live
// for the life of the program.

static SDL_Mutex* g_dialog_lock = nullptr;
static int        g_dialog_state = 0; // 0 = none, 1 = chosen, 2 = cancelled, 3 = error
static int        g_dialog_open = 0;
static char       g_dialog_path[4096];

static void zx_dialog_callback(void* userdata, const char* const* filelist, int filter)
{
    (void)userdata;
    (void)filter;
    SDL_LockMutex(g_dialog_lock);
    // SDL distinguishes the three outcomes by the list itself: NULL is an error, an
    // empty list is a cancel, and a list with a name is a choice.
    if (filelist == nullptr)
        g_dialog_state = 3;
    else if (filelist[0] == nullptr)
        g_dialog_state = 2;
    else
    {
        g_dialog_state = 1;
        SDL_strlcpy(g_dialog_path, filelist[0], sizeof(g_dialog_path));
    }
    g_dialog_open = 0;
    SDL_UnlockMutex(g_dialog_lock);
}

// zx_file_dialog_begin is the shared part of both dialogs: the filters, the pending
// flag and the state the callback writes into.
static void zx_file_dialog_begin(ZxView* v, const char* kind, const char* default_location,
                                 int save)
{
    if (g_dialog_lock == nullptr)
        g_dialog_lock = SDL_CreateMutex();

    const char* pattern = "*";
    const char* label = "All files";
    if (SDL_strcmp(kind, "tape") == 0)
    {
        pattern = "tap;tzx;zip";
        label = "Tape images";
    }
    else if (SDL_strcmp(kind, "disk") == 0)
    {
        pattern = "trd;scl;dsk;zip";
        label = "Disk images";
    }
    else if (SDL_strncmp(kind, "snapshot", 8) == 0)
    {
        pattern = "sna;z80;zip";
        label = "Snapshots";
    }

    static SDL_DialogFileFilter filters[1];
    filters[0].name = label;
    filters[0].pattern = pattern;

    SDL_LockMutex(g_dialog_lock);
    g_dialog_state = 0;
    g_dialog_open = 1;
    SDL_UnlockMutex(g_dialog_lock);

    if (save)
        SDL_ShowSaveFileDialog(zx_dialog_callback, nullptr, v ? v->Window : nullptr,
                               filters, 1, default_location);
    else
        SDL_ShowOpenFileDialog(zx_dialog_callback, nullptr, v ? v->Window : nullptr,
                               filters, 1, default_location, false);
}

// zx_open_file_dialog asks the user to choose an existing file of one kind. kind names
// the kind - "tape", "disk" or "snapshot" - and it is what picks the filter;
// default_location is where to start, which the front end takes from the last file of
// that kind.
void zx_open_file_dialog(ZxView* v, const char* kind, const char* default_location)
{
    zx_file_dialog_begin(v, kind, default_location, 0);
}

// zx_save_file_dialog asks the user to name a file to write. The platform's save
// dialog is a separate call: it names a file that need not exist yet, which is why the
// default location is a *name* rather than a place.
void zx_save_file_dialog(ZxView* v, const char* kind, const char* default_location)
{
    zx_file_dialog_begin(v, kind, default_location, 1);
}

// zx_dialog_pending reports whether a dialog is up, which is what D5 rule 10 needs:
// while one is, the machine must not hear the keyboard.
int zx_dialog_pending(ZxView* v)
{
    (void)v;
    if (g_dialog_lock == nullptr)
        return 0;
    SDL_LockMutex(g_dialog_lock);
    int open = g_dialog_open;
    SDL_UnlockMutex(g_dialog_lock);
    return open;
}

// zx_dialog_take drains the answer: 0 for nothing yet, 1 for a chosen file (copied
// into out), 2 for a cancel, 3 for an error.
int zx_dialog_take(char* out, int size)
{
    if (g_dialog_lock == nullptr)
        return 0;
    SDL_LockMutex(g_dialog_lock);
    int state = g_dialog_state;
    if (state != 0)
    {
        if (state == 1 && out != nullptr && size > 0)
            SDL_strlcpy(out, g_dialog_path, (size_t)size);
        g_dialog_state = 0;
    }
    SDL_UnlockMutex(g_dialog_lock);
    return state;
}

// --------------------------------------------------------------------- frames

void zx_frame_begin(ZxView* v)
{
    ImGui::SetCurrentContext(v->Ctx);
    ImGui_ImplSDLRenderer3_NewFrame();
    ImGui_ImplSDL3_NewFrame();
    ImGui::NewFrame();
    v->InFrame = true;
}

void zx_frame_end(ZxView* v, int r, int g, int b, int a)
{
    ImGui::SetCurrentContext(v->Ctx);
    ImGui::Render();
    SDL_SetRenderDrawColor(v->Renderer, (Uint8)r, (Uint8)g, (Uint8)b, (Uint8)a);
    SDL_RenderClear(v->Renderer);
    ImGui_ImplSDLRenderer3_RenderDrawData(ImGui::GetDrawData(), v->Renderer);
    SDL_RenderPresent(v->Renderer);
    v->InFrame = false;
}

int zx_wants_keyboard(ZxView* v)
{
    if (v == nullptr)
        return 0;
    ImGui::SetCurrentContext(v->Ctx);
    return ImGui::GetIO().WantCaptureKeyboard ? 1 : 0;
}

int zx_wants_mouse(ZxView* v)
{
    if (v == nullptr)
        return 0;
    ImGui::SetCurrentContext(v->Ctx);
    return ImGui::GetIO().WantCaptureMouse ? 1 : 0;
}

// -------------------------------------------------------------------- drawing

// zx_menubar_begin draws the menu bar over the top of the picture, and reports whether
// there is a bar to fill. zx_menubar_end closes it.
//
// It is ImGui's own main menu bar, drawn after the machine window, and the reason it
// ends up *over* the picture rather than above it is the pair of flags the machine
// window carries (see zx_main_window_frame): that window takes the whole client area
// whatever the bar reserves, and it can never be hovered or focused, so ImGui never
// brings it in front. The bar, which is created later and is not pinned back, is drawn
// last and therefore on top.
//
// This is what keeps the picture still. A bar that reserved room in the layout it is
// drawn into would rescale the screen when it appeared: the picture is scaled to fill
// the space it is given, so showing the bar would shrink it and hiding it would grow
// it back - and when the window is exactly an integer scale of the screen, the bar's
// nineteen pixels are the whole difference between 2x and 1x. (D4)
int zx_menubar_begin(ZxView* v)
{
    ImGui::SetCurrentContext(v->Ctx);
    return ImGui::BeginMainMenuBar() ? 1 : 0;
}

void zx_menubar_end(ZxView* v)
{
    ImGui::SetCurrentContext(v->Ctx);
    ImGui::EndMainMenuBar();
}

// zx_menu_begin opens a menu and reports whether it is open. zx_menu_end must be
// called only when it says yes: ImGui asserts on the mismatch, and a menu that is
// not open has no widgets to close.
int zx_menu_begin(ZxView* v, const char* label)
{
    ImGui::SetCurrentContext(v->Ctx);
    return ImGui::BeginMenu(label) ? 1 : 0;
}

void zx_menu_end(ZxView* v)
{
    ImGui::SetCurrentContext(v->Ctx);
    ImGui::EndMenu();
}

// zx_menu_item draws one entry and returns the id of the clicked one, or 0.
// checked < 0 draws a plain item, 0 and 1 draw a checkbox. 0 is never a valid id,
// so ids start at 1.
unsigned int zx_menu_item(ZxView* v, unsigned int id, const char* label,
                          const char* shortcut, int enabled, int checked)
{
    ImGui::SetCurrentContext(v->Ctx);
    bool clicked = false;
    if (checked >= 0)
        clicked = ImGui::MenuItem(label, shortcut, checked != 0, enabled != 0);
    else
        clicked = ImGui::MenuItem(label, shortcut, false, enabled != 0);
    return clicked ? id : 0;
}

void zx_menu_separator(ZxView* v)
{
    ImGui::SetCurrentContext(v->Ctx);
    ImGui::Separator();
}

// zx_status_text draws right-aligned text in the menu bar, which is where D4 puts
// the status read-outs.
void zx_status_text(ZxView* v, const char* text)
{
    ImGui::SetCurrentContext(v->Ctx);
    float avail = ImGui::GetContentRegionAvail().x;
    float w = ImGui::CalcTextSize(text).x;
    if (w < avail)
        ImGui::SetCursorPosX(ImGui::GetCursorPosX() + avail - w);
    ImGui::TextUnformatted(text);
}

// zx_screen_upload makes the view's screen texture match the pixels, creating it
// the first time or when the size changes. Returns 1 on success.
int zx_screen_upload(ZxView* v, const unsigned int* pixels, int w, int h)
{
    if (v == nullptr || w <= 0 || h <= 0)
        return 0;
    if (v->Screen == nullptr || v->ScreenW != w || v->ScreenH != h)
    {
        if (v->Screen != nullptr)
            SDL_DestroyTexture(v->Screen);
        v->Screen = SDL_CreateTexture(v->Renderer, SDL_PIXELFORMAT_ARGB8888,
                                      SDL_TEXTUREACCESS_STREAMING, w, h);
        if (v->Screen == nullptr)
            return 0;
        SDL_SetTextureScaleMode(v->Screen, SDL_SCALEMODE_NEAREST);
        v->ScreenW = w;
        v->ScreenH = h;
    }
    if (!SDL_UpdateTexture(v->Screen, nullptr, pixels, w * 4))
        return 0;
    return 1;
}

// zx_draw_screen draws the uploaded screen as the largest whole-number scale that
// fits the space left under the menubar, centred, with nearest sampling. Integer
// scaling is what the SDL path does and what a ZX picture wants: at 1x the result
// is a straight blit of the framebuffer.
void zx_draw_screen(ZxView* v)
{
    if (v == nullptr || v->Screen == nullptr)
        return;
    ImGui::SetCurrentContext(v->Ctx);

    ImVec2 avail = ImGui::GetContentRegionAvail();
    if (avail.x <= 0.0f || avail.y <= 0.0f)
        return;

    // Whole-number scales while there is room for one, which is what the SDL path
    // does and what a ZX picture wants. A window too small for even 1x gets the
    // picture scaled down to fit rather than cropped: seeing the whole screen
    // blurry beats seeing a corner of it sharply.
    float fit_x = avail.x / (float)v->ScreenW;
    float fit_y = avail.y / (float)v->ScreenH;
    float scale = fit_x < fit_y ? fit_x : fit_y;
    if (scale >= 1.0f)
        scale = (float)(int)scale;
    if (scale <= 0.0f)
        return;

    // Whole pixels for both the size and the offset. A fractional offset puts the
    // blit between texels, and nearest sampling then repeats one row and drops
    // another - a one-pixel jitter in the picture that the SDL path, whose logical
    // presentation centres on a whole rect, does not have.
    ImVec2 size(floorf((float)v->ScreenW * scale), floorf((float)v->ScreenH * scale));
    if (size.x < 1.0f || size.y < 1.0f)
        return;
    ImVec2 pos(ImGui::GetCursorScreenPos());
    pos.x = floorf(pos.x + (avail.x - size.x) * 0.5f);
    pos.y = floorf(pos.y + (avail.y - size.y) * 0.5f);
    ImGui::SetCursorScreenPos(pos);

    // Nearest sampling for the screen: the SDL path sets it on the texture, and
    // ImGui's draw commands carry their own sampler. The callbacks are the ones the
    // backend registers in platform_io, which is the supported way to ask for them.
    ImGuiPlatformIO& pio = ImGui::GetPlatformIO();
    ImDrawList* dl = ImGui::GetWindowDrawList();
    dl->AddCallback(pio.DrawCallback_SetSamplerNearest, nullptr);
    ImGui::Image((ImTextureID)(intptr_t)v->Screen, size);
    dl->AddCallback(pio.DrawCallback_SetSamplerLinear, nullptr);
}

// zx_screen_window makes the main window's content area exactly the screen area, so
// the menubar sits above it and nothing else competes with the picture. The frame
// is one full-viewport window with no decoration.
void zx_main_window_frame(ZxView* v)
{
    ImGui::SetCurrentContext(v->Ctx);
    ImGuiViewport* vp = ImGui::GetMainViewport();
    // The whole client area, not the viewport's *work* area: the menu bar reserves the
    // work area for itself, and the picture is meant to be under the bar rather than
    // beside it. This is what makes showing the bar leave the picture where it was.
    ImGui::SetNextWindowPos(vp->Pos);
    ImGui::SetNextWindowSize(vp->Size);
    // No padding and no border: the client area is the screen's, to the pixel. With a
    // border on, a small window loses two pixels each way and the picture is scaled or
    // clipped to fit what remains.
    ImGui::PushStyleVar(ImGuiStyleVar_WindowPadding, ImVec2(0, 0));
    ImGui::PushStyleVar(ImGuiStyleVar_WindowBorderSize, 0.0f);
    // NoBringToFrontOnFocus and NoMouseInputs together are what keep this window
    // behind the menu bar: with no mouse inputs it is never hovered and never focused,
    // so ImGui never brings it to the front. It has nothing to click - the emulated
    // mouse is the machine's business, not ImGui's - so nothing is lost.
    ImGui::Begin("##machine", nullptr,
                 ImGuiWindowFlags_NoDecoration | ImGuiWindowFlags_NoResize |
                 ImGuiWindowFlags_NoMove | ImGuiWindowFlags_NoBringToFrontOnFocus |
                 ImGuiWindowFlags_NoMouseInputs | ImGuiWindowFlags_NoSavedSettings);
}

void zx_main_window_frame_end(ZxView* v)
{
    ImGui::End();
    ImGui::PopStyleVar(2);
}

// zx_tool_begin fills a tool window's client area with one ImGui window and
// reports whether it is visible. wants_close is set when the user used the window's
// own close button: the caller closes the tool through the front end rather than
// here, so there is one place that decides what closing means (D4, D5).
//
// A tool with nothing in it yet still gets this: M2's done-when is a window that
// opens, moves, closes and comes back where it was.
int zx_tool_begin(ZxView* v, const char* title, int* wants_close)
{
    ImGui::SetCurrentContext(v->Ctx);
    ImGuiViewport* vp = ImGui::GetMainViewport();
    ImGui::SetNextWindowPos(vp->WorkPos);
    ImGui::SetNextWindowSize(vp->WorkSize);
    bool open = true;
    bool visible = ImGui::Begin(title, &open, ImGuiWindowFlags_NoSavedSettings);
    *wants_close = open ? 0 : 1;
    return visible ? 1 : 0;
}

void zx_tool_end(ZxView* v)
{
    ImGui::SetCurrentContext(v->Ctx);
    ImGui::End();
}

// zx_read_pixels copies what the renderer last drew into dst as ARGB8888. It is how
// a test checks that a 1x present is a straight copy of the framebuffer rather than
// something that merely looks right.
int zx_read_pixels(ZxView* v, void* dst, int w, int h)
{
    if (v == nullptr || dst == nullptr || w <= 0 || h <= 0)
        return 0;
    // SDL3 returns a surface rather than filling a caller's buffer, and the
    // surface's format is the renderer's rather than the one wanted.
    SDL_Surface* surf = SDL_RenderReadPixels(v->Renderer, nullptr);
    if (surf == nullptr)
        return 0;
    bool ok = SDL_ConvertPixels(surf->w, surf->h, surf->format, surf->pixels, surf->pitch,
                                SDL_PIXELFORMAT_ARGB8888, dst, w * 4);
    SDL_DestroySurface(surf);
    return ok ? 1 : 0;
}

void zx_text(ZxView* v, const char* text)
{
    ImGui::SetCurrentContext(v->Ctx);
    ImGui::TextUnformatted(text);
}

// zx_text_disabled draws a line greyed out, for a read-out with nothing to say yet.
void zx_text_disabled(ZxView* v, const char* text)
{
    ImGui::SetCurrentContext(v->Ctx);
    ImGui::TextDisabled("%s", text);
}

// zx_label_value draws a label and its value on one line, which is what every
// read-out in the tool windows is.
void zx_label_value(ZxView* v, const char* label, const char* value)
{
    ImGui::SetCurrentContext(v->Ctx);
    ImGui::TextDisabled("%s", label);
    ImGui::SameLine();
    ImGui::TextUnformatted(value);
}

// zx_button draws a button and reports whether it was pressed.
int zx_button(ZxView* v, const char* label, int enabled)
{
    ImGui::SetCurrentContext(v->Ctx);
    return ImGui::Button(label) && enabled ? 1 : 0;
}

// zx_button_enabled draws a button and reports both whether it was pressed and
// whether it was usable: a control that cannot act yet says so rather than being
// hidden, because a window that changes shape as the machine changes is harder to
// read than one with a greyed-out button in it.
int zx_button_state(ZxView* v, const char* label, int enabled)
{
    ImGui::SetCurrentContext(v->Ctx);
    if (!enabled)
    {
        ImGui::BeginDisabled();
        ImGui::Button(label);
        ImGui::EndDisabled();
        return 0;
    }
    return ImGui::Button(label) ? 1 : 0;
}

// zx_key draws one key of the on-screen keyboard: a button of a fixed size whose label
// carries the key's legends, drawn in the "active" colour while the key is held.
//
// It reports two things through the out-parameters: pressed for the click that happens
// while it is held, and held for the whole time it is. A keyboard wants the second:
// ImGui reports a button's click on release, and a ZX key goes down when the user's
// finger does. The caller compares held between frames to get the two edges.
//
// down is what makes a key the *machine* is holding look held as well as one the mouse
// is holding: the caller passes the state it knows from the matrix.
void zx_key(ZxView* v, const char* label, float w, float h, int down, int* held)
{
    ImGui::SetCurrentContext(v->Ctx);
    if (down)
        ImGui::PushStyleColor(ImGuiCol_Button, ImGui::GetStyleColorVec4(ImGuiCol_ButtonActive));
    ImGui::Button(label, ImVec2(w, h));
    if (down)
        ImGui::PopStyleColor();
    *held = ImGui::IsItemActive() ? 1 : 0;
}

// zx_radio draws one option of a mutually exclusive row. A radio button carries its own
// selected state, so unlike zx_choice there is nothing to colour: the caller hands over the
// truth and ImGui draws the dot from it.
int zx_radio(ZxView* v, const char* label, int selected)
{
    ImGui::SetCurrentContext(v->Ctx);
    return ImGui::RadioButton(label, selected != 0) ? 1 : 0;
}

// zx_checkbox draws a switch row's box. The state comes from the caller rather than being kept
// by the toolkit, so a click reports that it happened and the front end decides what it means -
// the same rule every other control here follows, and the reason the box is not left to flip
// itself: a setting that is waiting for a relaunch must show the file's value, not the
// toolkit's optimism.
int zx_checkbox(ZxView* v, const char* label, int checked)
{
    ImGui::SetCurrentContext(v->Ctx);
    bool value = checked != 0;
    return ImGui::Checkbox(label, &value) ? 1 : 0;
}

void zx_same_line(ZxView* v)
{
    ImGui::SetCurrentContext(v->Ctx);
    ImGui::SameLine();
}

void zx_separator(ZxView* v)
{
    ImGui::SetCurrentContext(v->Ctx);
    ImGui::Separator();
}

void zx_spacing(ZxView* v)
{
    ImGui::SetCurrentContext(v->Ctx);
    ImGui::Spacing();
}

// zx_heading draws a bold-ish section heading, which is what separates one read-out
// group from the next in a tool window.
void zx_heading(ZxView* v, const char* text)
{
    ImGui::SetCurrentContext(v->Ctx);
    ImGui::SeparatorText(text);
}

} // extern "C"
