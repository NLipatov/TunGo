package nftables

import (
	"tungo/application"
)

type Factory interface {
	New() (application.Netfilter, error)
}

type DefaultFactory struct{}

func (DefaultFactory) New() (application.Netfilter, error) {
	return NewBackend()
}
