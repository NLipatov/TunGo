//go:build darwin

package manager

import (
	"tungo/infrastructure/PAL/network/darwin/raw/ifconfig"
	"tungo/infrastructure/PAL/network/darwin/utun"
)

func createTUN(ifConfig ifconfig.Contract, mtu int) (utun.UTUN, error) {
	device, err := utun.Open()
	if err != nil {
		return nil, err
	}
	name, err := device.Name()
	if err != nil {
		_ = device.Close()
		return nil, err
	}
	if err := ifConfig.SetMTU(name, mtu); err != nil {
		_ = device.Close()
		return nil, err
	}
	return device, nil
}
