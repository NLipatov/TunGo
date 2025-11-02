package ipcfg

type Factory struct {
}

func NewFactory() Factory {
	return Factory{}
}

func (f *Factory) NewV4() Contract {
	return newV4()
}

func (f *Factory) NewV6() Contract {
	return newV6()
}
