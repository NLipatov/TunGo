package tun_server

import (
	"tungo/application"
	"tungo/infrastructure/settings"
)

type ServerTunFactory struct {
}

func NewServerTunFactory() application.ServerTunManager {
	return &ServerTunFactory{}
}

func (s ServerTunFactory) CreateTunDevice(_ settings.Settings) (application.TunDevice, error) {
	panic("not implemented")
}

func (s ServerTunFactory) DisposeTunDevices(_ settings.Settings) error {
	panic("not implemented")
}
