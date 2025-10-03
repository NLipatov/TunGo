//go:build darwin

package tun_adapters

import "tungo/infrastructure/PAL/darwin/network_tools/ifconfig"

type DefaultFactory struct {
	ifConfig ifconfig.Contract
}

func NewDefaultFactory(ifConfig ifconfig.Contract) *DefaultFactory {
	return &DefaultFactory{
		ifConfig: ifConfig,
	}
}

// CreateTUN mimics the API of wireguard/tun.CreateTUN on darwin.
func (d *DefaultFactory) CreateTUN(mtu int) (Adapter, error) {
	u, err := newUTun()
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
