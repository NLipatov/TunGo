//go:build !windows

package signal

import (
	"os"
	"syscall"
)

type DefaultProvider struct {
}

func NewDefaultProvider() *DefaultProvider {
	return &DefaultProvider{}
}

func (p *DefaultProvider) ShutdownSignals() []os.Signal {
	return []os.Signal{
		os.Interrupt,    // SIGINT (Ctrl-C)
		syscall.SIGTERM, // systemd/docker stop
		syscall.SIGHUP,  // terminal closed / reload
	}
}
