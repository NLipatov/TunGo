package network

import (
	"os"
)

type LinuxTunAdapter struct {
	TunFile *os.File
}

func (a *LinuxTunAdapter) Read(buffer []byte) (int, error) {
	n, err := a.TunFile.Read(buffer)
	return n, err
}

func (a *LinuxTunAdapter) Write(data []byte) (int, error) {
	n, err := a.TunFile.Write(data)
	return n, err
}

func (a *LinuxTunAdapter) Close() error {
	return a.TunFile.Close()
}
