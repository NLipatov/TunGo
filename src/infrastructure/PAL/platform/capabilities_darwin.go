package platform

type darwinCaps struct{}

func (darwinCaps) ServerModeSupported() bool { return false }

// Capabilities returns the platform capabilities for darwin.
func Capabilities() Caps { return darwinCaps{} }
