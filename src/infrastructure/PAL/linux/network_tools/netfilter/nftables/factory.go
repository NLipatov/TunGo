package nftables

import (
	"tungo/application"
)

type Factory interface {
	New() (application.Netfilter, error)
}

type DefaultFactory struct{}

func (DefaultFactory) New() (application.Netfilter, error) {
	//base netfilter
	netfilter, nErr := NewNetfilter()
	if nErr != nil {
		return nil, nErr
	}

	// make base netfilter concurrent via wrapper
	concurrentNetfilter := NewSynchronizedNetfilter(netfilter)

	return concurrentNetfilter, nil
}
