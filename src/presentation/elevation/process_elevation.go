package elevation

// ProcessElevation is a simple interface to check if the application is running with elevated privileges.
type ProcessElevation interface {
	IsElevated() bool
}
