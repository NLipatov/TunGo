//go:build darwin

package ifconfig

import "tungo/infrastructure/PAL"

type Factory struct {
	commander PAL.Commander
}

func NewFactory(commander PAL.Commander) *Factory {
	return &Factory{commander: commander}
}

func (f *Factory) NewV4() Contract {
	return newV4(f.commander)
}

func (f *Factory) NewV6() Contract {
	return newV6(f.commander)
}
