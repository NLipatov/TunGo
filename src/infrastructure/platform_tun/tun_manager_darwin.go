package platform_tun

import (
	"tungo/application"
	"tungo/settings/client_configuration"
)

// PlatformTunManager Linux-specific TunDevice manager
type PlatformTunManager struct {
	conf client_configuration.Configuration
}

func NewPlatformTunManager(conf client_configuration.Configuration) (application.TunManager, error) {
	return &PlatformTunManager{
		conf: conf,
	}, nil
}

func (t *PlatformTunManager) CreateTunDevice() (application.TunDevice, error) {
	panic("not implemented")
}

func (t *PlatformTunManager) DisposeTunDevices() error {
	panic("not implemented")
}
