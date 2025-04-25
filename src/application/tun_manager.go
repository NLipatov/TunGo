package application

import "tungo/settings"

type ClientTunManager interface {
	CreateTunDevice() (TunDevice, error)
	DisposeTunDevices() error
}

type ServerTunManager interface {
	CreateTunDevice(settings settings.ConnectionSettings) (TunDevice, error)
	DisposeTunDevices(settings settings.ConnectionSettings) error
}
