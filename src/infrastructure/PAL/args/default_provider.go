package args

import "os"

type DefaultProvider struct {
}

func NewDefaultProvider() *DefaultProvider {
	return &DefaultProvider{}
}

func (d *DefaultProvider) Args() []string {
	// skip binary name(e.g. tungo), which is first argument
	return os.Args[1:]
}
