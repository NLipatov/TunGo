package elevation

import "os"

// ProcessElevationImpl implements ProcessElevation on macOS/Linux.
type ProcessElevationImpl struct{}

func NewProcessElevation() ProcessElevation {
	return &ProcessElevationImpl{}
}

// IsElevated returns true if we're running as root.
func (p *ProcessElevationImpl) IsElevated() bool {
	return os.Geteuid() == 0
}
