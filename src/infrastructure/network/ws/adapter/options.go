package adapter

type Options struct {
	CtxFactory  CtxFactory
	Copier      Copier
	ErrorMapper ErrorMapper
}

func (o *Options) WithDefaults() *Options {
	if o == nil {
		o = &Options{}
	}
	if o.CtxFactory == nil {
		o.CtxFactory = DefaultCtxFactory{}
	}
	if o.Copier == nil {
		o.Copier = DefaultCopier{}
	}
	if o.ErrorMapper == nil {
		o.ErrorMapper = DefaultErrorMapper{}
	}
	return o
}
