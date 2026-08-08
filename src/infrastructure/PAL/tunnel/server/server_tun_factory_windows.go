package server

import (
	"errors"
	"io"
	"tungo/application/configuration/settings"
)

var errServerNotSupported = errors.New("server mode is not supported on this platform")

type TunFactory struct {
}

func NewTunFactory() *TunFactory {
	return &TunFactory{}
}

func (s TunFactory) CreateDevice(_ settings.Settings) (io.ReadWriteCloser, error) {
	return nil, errServerNotSupported
}

func (s TunFactory) DisposeDevices(_ settings.Settings) error {
	return nil
}
