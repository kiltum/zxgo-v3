// Package frontend holds the decisions every front end shares: how a matrix key
// or a host command reaches the machine, when the screen is presented, how a
// replay's media and a session's path are resolved, and how a screenshot is
// written. It draws nothing and owns no window.
//
// It sits where it does on purpose. Dependencies point inward (UI_DESIGN.md
// section 4): cmd -> internal/ui/* -> internal/frontend -> internal/emulator.
// This package therefore imports no UI backend, and the loop it drives talks to a
// port it declares itself (WindowBackend, backendloop.go) that a window backend
// satisfies structurally - no adapter, no cycle.
//
// Everything here reaches the machine through the emulator's own methods.
// Recording hangs off PressKey, ReleaseKey and the tape transport, so a second
// route to the matrix produces a replay file that is silently missing its input
// events (see internal/emulator/emulator.go:204 and ToggleTapePlayback).
//
// UI_DESIGN.md's stages name the work: M0 moved this out of cmd/zxgo, M1 adds the
// App, the run state, the action registry and the view models, M2 gives it the
// ImGui backend.
package frontend
