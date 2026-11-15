package screen

// Input injects pointer and keyboard events into the interactive desktop.
//
// It is deliberately narrow: normalised coordinates (0..1 over the virtual
// desktop) rather than pixels, so the browser can send what it rendered without
// knowing the host's resolution, and a fixed key-code vocabulary (Windows
// virtual-key codes) rather than text, so no keystroke synthesis happens here.
type Input interface {
	// MoveTo moves the pointer to the given fraction of the virtual desktop.
	MoveTo(x, y float64) error
	// Button presses or releases a mouse button (1 left, 2 right, 3 middle).
	Button(button int, down bool) error
	// Wheel scrolls; positive is away from the user.
	Wheel(delta int) error
	// Key presses or releases a virtual-key code.
	Key(vk uint16, down bool) error
}

// NewInput returns the platform injector.
func NewInput() Input { return newInput() }
