package ioctl

import (
	"os"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

// mockCommander implements Commander for testing
type mockWrapperCommander struct {
	ioctlFunc func(fd uintptr, request uintptr, arg uintptr) (uintptr, uintptr, unix.Errno)
}

func (m *mockWrapperCommander) Ioctl(fd uintptr, request uintptr, arg uintptr) (uintptr, uintptr, unix.Errno) {
	return m.ioctlFunc(fd, request, arg)
}

func TestDetectTunNameFromFd_Success(t *testing.T) {
	expectedName := "tun0"
	mock := &mockWrapperCommander{
		ioctlFunc: func(fd, req, arg uintptr) (uintptr, uintptr, unix.Errno) {
			ifr := (*IfReq)(unsafe.Pointer(arg))
			copy(ifr.Name[:], expectedName)
			return 0, 0, 0
		},
	}
	wrapper := NewWrapper(mock, "/dev/null")

	f, err := os.Open("/dev/null")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	name, err := wrapper.DetectTunNameFromFd(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != expectedName {
		t.Errorf("expected name %q, got %q", expectedName, name)
	}
}

func TestDetectTunNameFromFd_Error(t *testing.T) {
	mock := &mockWrapperCommander{
		ioctlFunc: func(fd, req, arg uintptr) (uintptr, uintptr, unix.Errno) {
			return 0, 0, unix.EINVAL
		},
	}
	wrapper := NewWrapper(mock, "/dev/null")

	f, err := os.Open("/dev/null")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	_, err = wrapper.DetectTunNameFromFd(f)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCreateTunInterface_Success(t *testing.T) {
	mock := &mockWrapperCommander{
		ioctlFunc: func(fd, req, arg uintptr) (uintptr, uintptr, unix.Errno) {
			return 0, 0, 0
		},
	}
	wrapper := NewWrapper(mock, "/dev/null")

	tun, err := wrapper.CreateTunInterface("tun-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tun == nil {
		t.Fatal("expected valid tun file, got nil")
	}
	_ = tun.Close()
}

func TestCreateTunInterface_OpenError(t *testing.T) {
	mock := &mockWrapperCommander{
		ioctlFunc: func(fd, req, arg uintptr) (uintptr, uintptr, unix.Errno) {
			return 0, 0, 0
		},
	}
	wrapper := NewWrapper(mock, "/dev/this-does-not-exist")

	tun, err := wrapper.CreateTunInterface("tun-test")
	if err == nil || !strings.Contains(err.Error(), "failed to open") {
		t.Fatalf("expected open error, got: %v", err)
	}
	if tun != nil {
		t.Fatal("expected nil tun file on open error")
	}
}

func TestCreateTunInterface_IoctlError(t *testing.T) {
	mock := &mockWrapperCommander{
		ioctlFunc: func(fd, req, arg uintptr) (uintptr, uintptr, unix.Errno) {
			return 0, 0, unix.EPERM
		},
	}
	wrapper := NewWrapper(mock, "/dev/null")

	tun, err := wrapper.CreateTunInterface("tun-test")
	if err == nil || !strings.Contains(err.Error(), "ioctl TUNSETIFF failed") {
		t.Fatalf("expected ioctl error, got: %v", err)
	}
	if tun != nil {
		t.Fatal("expected nil tun file on ioctl error")
	}
}
