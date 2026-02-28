//go:build linux

package ioctl

import (
	"golang.org/x/sys/unix"
	"unsafe"
)

type Commander interface {
	Ioctl(fd uintptr, request uintptr, ifr *IfReq) (uintptr, uintptr, unix.Errno)
}

type LinuxIoctlCommander struct {
}

func NewLinuxIoctlCommander() Commander {
	return &LinuxIoctlCommander{}
}

func (d LinuxIoctlCommander) Ioctl(fd uintptr, request uintptr, ifr *IfReq) (uintptr, uintptr, unix.Errno) {
	return unix.Syscall(unix.SYS_IOCTL, fd, request, uintptr(unsafe.Pointer(ifr)))
}
