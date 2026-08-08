//go:build darwin

package utun

import "tungo/infrastructure/PAL/network/darwin/ifconfig"

type Factory struct {
	ifConfig ifconfig.Contract
}

func NewFactory(ifConfig ifconfig.Contract) *Factory {
	return &Factory{
		ifConfig: ifConfig,
	}
}

// CreateTUN mimics the API of wireguard/tun.CreateTUN on darwin.
func (d *Factory) CreateTUN(mtu int) (UTUN, error) {
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
