package elevation

import "os"

type ProcessElevationImpl struct {
}

func NewProcessElevation() ProcessElevation {
	return &ProcessElevationImpl{}
}

func (p *ProcessElevationImpl) IsElevated() bool {
	return os.Getuid() == 0
}
