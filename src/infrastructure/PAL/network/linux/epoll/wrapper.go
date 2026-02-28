//go:build linux

package epoll

import (
	"os"
	application "tungo/application/network/routing/tun"
)

type Wrapper struct {
}

func NewWrapper() application.Wrapper {
	return &Wrapper{}
}

func (e *Wrapper) Wrap(f *os.File) (application.Device, error) {
	return newTUN(f)
}
