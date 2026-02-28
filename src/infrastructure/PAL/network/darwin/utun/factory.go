//go:build darwin

package utun

import "tungo/infrastructure/PAL/network/darwin/ifconfig"

type Factory interface {
	CreateTUN(mtu int) (UTUN, error)
}

type DefaultFactory struct {
	ifConfig ifconfig.Contract
}

func NewDefaultFactory(ifConfig ifconfig.Contract) *DefaultFactory {
	return &DefaultFactory{
		ifConfig: ifConfig,
	}
}

// CreateTUN mimics the API of wireguard/tun.CreateTUN on darwin.
func (d *DefaultFactory) CreateTUN(mtu int) (UTUN, error) {
	u, err := newRawUTUN()
	if err != nil {
		return nil, err
	}
	ifName := u.name

	if setMTUErr := d.ifConfig.SetMTU(ifName, mtu); setMTUErr != nil {
		_ = u.Close()
		return nil, setMTUErr
	}

	return u, nil
}
