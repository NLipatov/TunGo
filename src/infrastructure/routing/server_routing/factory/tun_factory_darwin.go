package factory

import (
	"tungo/application"
	"tungo/settings"
)

type ServerTunFactory struct {
}

func NewServerTunFactory() application.ServerTunManager {
	return &ServerTunFactory{}
}

func (s ServerTunFactory) CreateTunDevice(_ settings.ConnectionSettings) (application.TunDevice, error) {
	panic("not implemented")
}

func (s ServerTunFactory) DisposeTunDevices(_ settings.ConnectionSettings) error {
	panic("not implemented")
}
