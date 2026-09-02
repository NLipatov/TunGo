//go:build linux

package ioctl

import (
	"errors"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func TestLinuxIoctlCommander(t *testing.T) {
	file, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { _ = file.Close() })

	commander := NewLinuxIoctlCommander()
	var req IfReq
	if _, _, errno := commander.Ioctl(file.Fd(), uintptr(unix.TUNGETIFF), &req); !errors.Is(errno, unix.ENOTTY) {
		t.Fatalf("Ioctl() error = %v, want %v", errno, unix.ENOTTY)
	}
	if errno := commander.IoctlInt(file.Fd(), uintptr(unix.TUNSETPERSIST), 0); !errors.Is(errno, unix.ENOTTY) {
		t.Fatalf("IoctlInt() error = %v, want %v", errno, unix.ENOTTY)
	}
}
