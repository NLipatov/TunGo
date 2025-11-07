//go:build windows

package ipcfg

import "tungo/infrastructure/PAL/windows/ipcfg/nif"

type Factory struct {
}

func NewFactory() Factory {
	return Factory{}
}

func (f *Factory) NewV4() Contract {
	return newV4(
		nif.NewResolver(),
	)
}

func (f *Factory) NewV6() Contract {
	return newV6(
		nif.NewResolver(),
	)
}
