package ioctl

import (
	"os"
)

type Contract interface {
	DetectTunNameFromFd(fd *os.File) (string, error)
	CreateTunInterface(name string) (*os.File, error)
}
