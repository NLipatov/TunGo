//go:build linux

package epoll

import (
	"io"
	"os"
)

type Wrapper struct {
}

func NewWrapper() *Wrapper {
	return &Wrapper{}
}

func (e *Wrapper) Wrap(f *os.File) (io.ReadWriteCloser, error) {
	return newTUN(f)
}
