package platform

// Caps describes what the current platform supports.
type Caps interface {
	ServerModeSupported() bool
}

