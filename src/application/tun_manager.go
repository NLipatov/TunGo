package application

import (
	"tungo/infrastructure/settings"
)

type ClientTunManager interface {
	CreateTunDevice() (TunDevice, error)
	DisposeTunDevices() error
}

type ServerTunManager interface {
	CreateTunDevice(settings settings.Settings) (TunDevice, error)
	DisposeTunDevices(settings settings.Settings) error
	DisableDevMasquerade() error
}
