package network

import (
	"os"
)

type (
	//TunAdapter provides a single and trivial API for any supported tun devices
	TunAdapter interface {
		Read(data []byte) (int, error)
		Write(data []byte) (int, error)
		Close() error
	}
	LinuxTunAdapter struct {
		TunFile *os.File
	}
)

func (a *LinuxTunAdapter) Read(buffer []byte) (int, error) {
	n, err := a.TunFile.Read(buffer)
	return n, err
}

func (a *LinuxTunAdapter) Write(data []byte) (int, error) {
	n, err := a.TunFile.Write(data)
	return n, err
}

func (a *LinuxTunAdapter) Close() error {
	return a.Close()
}
