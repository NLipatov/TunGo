package elevation

import "os"

// ProcessElevationImpl implements ProcessElevation on GNU/Linux
type ProcessElevationImpl struct {
}

func NewProcessElevation() ProcessElevation {
	return &ProcessElevationImpl{}
}

func (p *ProcessElevationImpl) IsElevated() bool {
	return os.Getuid() == 0
}
