//go:build windows

package signal

import (
	"os"
	"syscall"
)

type DefaultProvider struct {
}

func NewDefaultProvider() Provider {
	return &DefaultProvider{}
}

func (p *DefaultProvider) ShutdownSignals() []os.Signal {
	return []os.Signal{
		os.Interrupt,    // Ctrl-C
		syscall.SIGTERM, // console close / Task Manager stop
	}
}
