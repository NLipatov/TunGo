//go:build windows

package signal

import "os"

type DefaultProvider struct {
}

func NewDefaultProvider() Provider {
	return &DefaultProvider{}
}

func (p *DefaultProvider) ShutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
