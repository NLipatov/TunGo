package syscall

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"strings"
	"unsafe"
)

func DetectTunNameFromFd(fd *os.File) (string, error) {
	var ifr IfReq

	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
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

func CreateTunInterface(name string) (*os.File, error) {
	tun, err := os.OpenFile(tunPath, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open %v: %v", tunPath, err)
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

	_, _, errno := unix.Syscall(unix.SYS_IOCTL, tun.Fd(), uintptr(tunSetIff), uintptr(unsafe.Pointer(&req)))
	if errno != 0 {
		return nil, fmt.Errorf("ioctl TUNSETIFF failed for %s: %v", name, errno)
	}

	shouldCloseTun = false
	return tun, nil
}
