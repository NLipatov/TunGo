//go:build darwin

package utun

import "tungo/internal/tun/internal/darwin/ifconfig"

func Create(ifConfig ifconfig.Contract, mtu int) (UTUN, error) {
	u, err := newRawUTUN()
	if err != nil {
		return nil, err
	}
	ifName := u.name

	if setMTUErr := ifConfig.SetMTU(ifName, mtu); setMTUErr != nil {
		_ = u.Close()
		return nil, setMTUErr
	}

	return u, nil
}
