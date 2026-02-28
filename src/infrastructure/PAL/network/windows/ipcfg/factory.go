//go:build windows

package ipcfg

import (
	"tungo/infrastructure/PAL/network/windows/ipcfg/network_interface/resolver"
)

type Factory struct {
}

func NewFactory() Factory {
	return Factory{}
}

func (f *Factory) NewV4() Contract {
	return newV4(
		resolver.NewResolver(),
	)
}

func (f *Factory) NewV6() Contract {
	return newV6(
		resolver.NewResolver(),
	)
}
