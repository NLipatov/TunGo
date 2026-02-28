package platform

type linuxCaps struct{}

func (linuxCaps) ServerModeSupported() bool { return true }

// Capabilities returns the platform capabilities for linux.
func Capabilities() Caps { return linuxCaps{} }
