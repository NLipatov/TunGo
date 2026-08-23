package server

import (
	"errors"
	"io"
	"tungo/internal/config/settings"
)

var errServerNotSupported = errors.New("server mode is not supported on this platform")

type Manager struct {
}

func NewManager() *Manager {
	return &Manager{}
}

func (s Manager) OpenTunnel(_ settings.Settings) (io.ReadWriteCloser, error) {
	return nil, errServerNotSupported
}

func (s Manager) CloseTunnel(_ settings.Settings) error {
	return nil
}
