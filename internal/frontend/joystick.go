package frontend

// JoyState is a host joystick read as the machine's Kempston port sees it.
//
// It is the front end's type rather than the backend's for the reason `Rect` and
// `Key` are: a backend reports what it saw in terms the front end owns, so a
// second backend (or the on-screen keyboard, which already plays this part for
// the matrix) does not need a second vocabulary. The five fields are the port's
// five bits and nothing else - a backend resolves a stick's axis, a D-pad and a
// host key down to the direction they mean before it gets here.
//
// Opposite directions can be true at once, exactly as they can on the port: a
// gamepad whose stick is pushed sideways diagonally is not the front end's to
// disallow, and the machine reads what it reads. The zero value is the released
// stick, which is what a disconnected pad reports.
type JoyState struct {
	Right bool
	Left  bool
	Down  bool
	Up    bool
	Fire  bool
}

// Any reports whether any direction or the fire button is held. It is what a
// caller asks before doing something visible about the joystick, so that an
// untouched pad costs nothing.
func (j JoyState) Any() bool {
	return j.Right || j.Left || j.Down || j.Up || j.Fire
}
