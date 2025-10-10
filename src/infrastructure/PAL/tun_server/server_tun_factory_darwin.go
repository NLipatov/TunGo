package tun_server

import (
	tun "tungo/application/network/routing/tun"
	"tungo/infrastructure/settings"
)

type ServerTunFactory struct {
}

func NewServerTunFactory() tun.ServerManager {
	return &ServerTunFactory{}
}

func (s ServerTunFactory) CreateDevice(_ settings.Settings) (tun.Device, error) {
	panic("not implemented")
}

func (s ServerTunFactory) DisposeDevices(_ settings.Settings) error {
	panic("not implemented")
}
