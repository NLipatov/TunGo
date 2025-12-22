//go:build darwin

package route

import (
	"tungo/infrastructure/PAL/exec_commander"
)

type Factory struct {
	commander exec_commander.Commander
}

func NewFactory(commander exec_commander.Commander) *Factory {
	return &Factory{
		commander: commander,
	}
}

func (f *Factory) NewV4() Contract {
	return newV4(f.commander)
}

func (f *Factory) NewV6() Contract {
	return newV6(f.commander)
}
