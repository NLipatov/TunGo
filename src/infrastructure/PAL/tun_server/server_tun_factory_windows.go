package tun_server

import (
	"tungo/application/network/tun"
	"tungo/infrastructure/settings"
)

type ServerTunFactory struct {
}

func NewServerTunFactory() tun.ServerManager {
	return &ServerTunFactory{}
}

func (s ServerTunFactory) CreateTunDevice(_ settings.Settings) (tun.Device, error) {
	panic("not implemented")
}

func (s ServerTunFactory) DisposeTunDevices(_ settings.Settings) error {
	panic("not implemented")
}
