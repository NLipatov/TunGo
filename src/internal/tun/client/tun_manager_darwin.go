//go:build darwin

package client

import (
	"io"
	"net/netip"
	"tungo/internal/config/client"
	"tungo/internal/tun/internal/darwin/manager"
)

type clientManager interface {
	OpenTunnel(serverAddr netip.Addr) (io.ReadWriteCloser, error)
	CloseTunnel() error
}

func New(conf *client.Configuration) (clientManager, error) {
	settings, err := conf.ActiveSettings()
	if err != nil {
		return nil, err
	}
	return manager.New(settings)
}
