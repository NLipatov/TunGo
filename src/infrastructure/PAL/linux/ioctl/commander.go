package ioctl

import "golang.org/x/sys/unix"

type Commander interface {
	Ioctl(fd uintptr, request uintptr, arg uintptr) (uintptr, uintptr, unix.Errno)
}

type LinuxIoctlCommander struct {
}

func NewLinuxIoctlCommander() Commander {
	return &LinuxIoctlCommander{}
}

func (d LinuxIoctlCommander) Ioctl(fd uintptr, request uintptr, arg uintptr) (uintptr, uintptr, unix.Errno) {
	return unix.Syscall(unix.SYS_IOCTL, fd, request, arg)
}
