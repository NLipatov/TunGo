package ioctl

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"strings"
	"unsafe"
)

type Wrapper struct {
	commander Commander
	tunPath   string
}

func NewWrapper(commander Commander, tunPath string) Contract {
	return &Wrapper{
		commander: commander,
		tunPath:   tunPath,
	}
}

func (w *Wrapper) DetectTunNameFromFd(fd *os.File) (string, error) {
	var ifr IfReq

	_, _, errno := w.commander.Ioctl(
		fd.Fd(),
		uintptr(unix.TUNGETIFF),
		uintptr(unsafe.Pointer(&ifr)),
	)
	if errno != 0 {
		return "", errno
	}

	name := strings.TrimRight(string(ifr.Name[:]), "\x00")
	return name, nil
}

func (w *Wrapper) CreateTunInterface(name string) (*os.File, error) {
	tun, err := os.OpenFile(w.tunPath, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open %v: %v", w.tunPath, err)
	}

	shouldCloseTun := true
	defer func() {
		if shouldCloseTun {
			_ = tun.Close()
		}
	}()

	var req IfReq
	copy(req.Name[:], name)
	req.Flags = iffTun | IffNoPi

	_, _, errno := w.commander.Ioctl(tun.Fd(), uintptr(tunSetIff), uintptr(unsafe.Pointer(&req)))
	if errno != 0 {
		return nil, fmt.Errorf("ioctl TUNSETIFF failed for %s: %v", name, errno)
	}

	shouldCloseTun = false
	return tun, nil
}
