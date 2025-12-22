//go:build darwin

package scutil

type Factory struct {
}

func NewFactory() *Factory {
	return &Factory{}
}

func (f *Factory) NewV4() Contract {
	return NewV4()
}

func (f *Factory) NewV6() Contract {
	return NewV6()
}
