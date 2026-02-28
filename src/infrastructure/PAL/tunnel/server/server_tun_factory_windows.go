package server

import (
	"errors"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/settings"
)

var errServerNotSupported = errors.New("server mode is not supported on this platform")

type TunFactory struct {
}

func NewTunFactory() tun.ServerManager {
	return &TunFactory{}
}

func (s TunFactory) CreateDevice(_ settings.Settings) (tun.Device, error) {
	return nil, errServerNotSupported
}

func (s TunFactory) DisposeDevices(_ settings.Settings) error {
	return nil
}
