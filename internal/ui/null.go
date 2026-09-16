package ui

// nullUI is the headless UI backend: it renders nothing and reports no input.
// The MCP worker uses it so the emulator runs without a window and without
// SDL3.
type nullUI struct{}

// NewNull returns a UI that discards all rendering and input.
func NewNull() UI { return &nullUI{} }

func (n *nullUI) Init() error { return nil }

func (n *nullUI) ProcessEvents(onKey func(row, col int, pressed bool), onCommand func(command string, pressed bool)) bool {
	return true // never reports a quit request
}

func (n *nullUI) RenderFrame(screen []uint32) {}

func (n *nullUI) SetTitle(title string) {}

func (n *nullUI) Destroy() {}
