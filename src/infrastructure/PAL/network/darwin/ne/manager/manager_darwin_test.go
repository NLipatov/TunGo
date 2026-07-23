//go:build darwin

package manager

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestRegisterFileDescriptorRejectsInvalidDescriptor(t *testing.T) {
	if _, err := RegisterFileDescriptor(-1); err == nil {
		t.Fatal("expected invalid descriptor error")
	}
}

func TestRegisterFileDescriptorAllowsOneActiveRegistration(t *testing.T) {
	sockets, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_DGRAM, 0)
	if err != nil {
		t.Fatalf("Socketpair() error = %v", err)
	}
	defer unix.Close(sockets[0])
	defer unix.Close(sockets[1])

	release, err := RegisterFileDescriptor(sockets[0])
	if err != nil {
		t.Fatalf("first registration error = %v", err)
	}
	if _, err := RegisterFileDescriptor(sockets[0]); err == nil {
		t.Fatal("second active registration unexpectedly succeeded")
	}
	release()

	releaseAgain, err := RegisterFileDescriptor(sockets[0])
	if err != nil {
		t.Fatalf("registration after release error = %v", err)
	}
	releaseAgain()
}

func TestRegisterFileDescriptorReleaseIsScopedAndIdempotent(t *testing.T) {
	sockets, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_DGRAM, 0)
	if err != nil {
		t.Fatalf("Socketpair() error = %v", err)
	}
	defer unix.Close(sockets[0])
	defer unix.Close(sockets[1])

	firstRelease, err := RegisterFileDescriptor(sockets[0])
	if err != nil {
		t.Fatalf("first registration error = %v", err)
	}
	firstRelease()

	secondRelease, err := RegisterFileDescriptor(sockets[0])
	if err != nil {
		t.Fatalf("second registration error = %v", err)
	}
	defer secondRelease()
	firstRelease()

	if _, err := RegisterFileDescriptor(sockets[0]); err == nil {
		t.Fatal("stale release removed the active registration")
	}
}
