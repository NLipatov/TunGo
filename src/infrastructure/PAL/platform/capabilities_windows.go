package platform

type windowsCaps struct{}

func (windowsCaps) ServerModeSupported() bool { return false }

// Capabilities returns the platform capabilities for windows.
func Capabilities() Caps { return windowsCaps{} }
