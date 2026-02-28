package tun_server

import (
	"errors"
	tun "tungo/application/network/routing/tun"
	"tungo/infrastructure/settings"
)

var errServerNotSupported = errors.New("server mode is not supported on this platform")

type ServerTunFactory struct {
}

func NewServerTunFactory() tun.ServerManager {
	return &ServerTunFactory{}
}

func (s ServerTunFactory) CreateDevice(_ settings.Settings) (tun.Device, error) {
	return nil, errServerNotSupported
}

func (s ServerTunFactory) DisposeDevices(_ settings.Settings) error {
	return nil
}
