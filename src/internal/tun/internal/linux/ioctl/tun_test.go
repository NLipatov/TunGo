//go:build linux
// +build linux

package ioctl

import (
	"errors"
	"os"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// mockCommander implements Commander for testing.
type mockCommander struct {
	// override behavior per call
	IoctlFn    func(fd uintptr, request uintptr, ifr *IfReq) (uintptr, uintptr, unix.Errno)
	IoctlIntFn func(fd uintptr, request uintptr, value int) unix.Errno
}

func (m *mockCommander) Ioctl(fd uintptr, request uintptr, ifr *IfReq) (uintptr, uintptr, unix.Errno) {
	return m.IoctlFn(fd, request, ifr)
}

func (m *mockCommander) IoctlInt(fd uintptr, request uintptr, value int) unix.Errno {
	if m.IoctlIntFn == nil {
		return 0
	}
	return m.IoctlIntFn(fd, request, value)
}

func TestDetectTunNameFromFd_Success(t *testing.T) {
	const expected = "tunXYZ"
	mock := &mockCommander{
		IoctlFn: func(fd uintptr, request uintptr, ifr *IfReq) (uintptr, uintptr, unix.Errno) {
			copy(ifr.Name[:], expected)
			return 0, 0, 0
		},
	}
	w := New(mock, os.DevNull)

	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("failed to open %s: %v", os.DevNull, err)
	}
	defer func(f *os.File) {
		_ = f.Close()
	}(f)

	name, err := w.DetectTunNameFromFd(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != expected {
		t.Errorf("got %q, want %q", name, expected)
	}
}

func TestDetectTunNameFromFd_Error(t *testing.T) {
	mock := &mockCommander{
		IoctlFn: func(fd uintptr, request uintptr, ifr *IfReq) (uintptr, uintptr, unix.Errno) {
			return 0, 0, unix.EPERM
		},
	}
	w := New(mock, os.DevNull)

	f, _ := os.Open(os.DevNull)
	defer func(f *os.File) {
		_ = f.Close()
	}(f)

	_, err := w.DetectTunNameFromFd(f)
	if !errors.Is(err, unix.EPERM) {
		t.Fatalf("got error %v, want unix.EPERM", err)
	}
}

func TestCreateTunInterface_Success(t *testing.T) {
	persistenceDisabled := false
	mock := &mockCommander{
		IoctlFn: func(fd uintptr, request uintptr, ifr *IfReq) (uintptr, uintptr, unix.Errno) {
			// ensure name and flags were set on req
			name := string(ifr.Name[:len(ifr.Name)-1])
			if !strings.HasPrefix(name, "tunTest") {
				t.Errorf("expected ioctl to receive a Name starting 'tunTest', got %q", name)
			}
			return 0, 0, 0
		},
		IoctlIntFn: func(_ uintptr, request uintptr, value int) unix.Errno {
			if request != uintptr(unix.TUNSETPERSIST) || value != 0 {
				t.Errorf("IoctlInt() = request %#x, value %d; want TUNSETPERSIST, 0", request, value)
			}
			persistenceDisabled = true
			return 0
		},
	}
	w := New(mock, os.DevNull)

	f, err := w.CreateTunInterface("tunTest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f == nil {
		t.Fatal("expected non-nil *os.File")
	}
	if !persistenceDisabled {
		t.Fatal("CreateTunInterface() did not disable TUN persistence")
	}
	_ = f.Close()
}

func TestCreateTunInterface_OpenError(t *testing.T) {
	mock := &mockCommander{ // should not even be invoked
		IoctlFn: func(fd uintptr, request uintptr, ifr *IfReq) (uintptr, uintptr, unix.Errno) {
			t.Fatal("Ioctl should not be called when OpenFile fails")
			return 0, 0, 0
		},
	}
	w := New(mock, "/path/does/not/exist")

	_, err := w.CreateTunInterface("foo")
	if err == nil {
		t.Fatal("expected error opening tunPath")
	}
	if !strings.Contains(err.Error(), "failed to open") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCreateTunInterface_IoctlError(t *testing.T) {
	mock := &mockCommander{
		IoctlFn: func(fd uintptr, request uintptr, ifr *IfReq) (uintptr, uintptr, unix.Errno) {
			return 0, 0, unix.EPERM
		},
	}
	w := New(mock, os.DevNull)

	f, err := w.CreateTunInterface("tunError")
	if err == nil {
		t.Fatal("expected ioctl failure")
	}
	if !strings.Contains(err.Error(), "ioctl TUNSETIFF failed") {
		t.Errorf("error message %q does not mention TUNSETIFF", err.Error())
	}
	if f != nil {
		t.Errorf("expected returned file to be nil on error, got %v", f)
	}
}

func TestCreateTunInterface_PersistenceError(t *testing.T) {
	mock := &mockCommander{
		IoctlFn: func(fd uintptr, request uintptr, ifr *IfReq) (uintptr, uintptr, unix.Errno) {
			return 0, 0, 0
		},
		IoctlIntFn: func(fd uintptr, request uintptr, value int) unix.Errno {
			return unix.EPERM
		},
	}
	w := New(mock, os.DevNull)

	f, err := w.CreateTunInterface("tunPersistent")
	if err == nil {
		t.Fatal("expected TUNSETPERSIST failure")
	}
	if !strings.Contains(err.Error(), "ioctl TUNSETPERSIST failed") {
		t.Errorf("error message %q does not mention TUNSETPERSIST", err.Error())
	}
	if f != nil {
		t.Errorf("expected returned file to be nil on error, got %v", f)
	}
}
