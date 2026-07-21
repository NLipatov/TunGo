//go:build darwin

package client

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"testing"

	"golang.org/x/sys/unix"
	appConfiguration "tungo/application/configuration"
)

func TestManager_DeviceUsesDuplicateOfNetworkExtensionDescriptor(t *testing.T) {
	sockets, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_DGRAM, 0)
	if err != nil {
		t.Fatalf("Socketpair() error = %v", err)
	}
	defer unix.Close(sockets[0])
	defer unix.Close(sockets[1])

	release, err := RegisterNetworkExtensionFileDescriptor(sockets[0])
	if err != nil {
		t.Fatalf("RegisterNetworkExtensionFileDescriptor() error = %v", err)
	}
	defer release()
	manager, err := NewPlatformTunManager(appConfiguration.ClientRuntimeConfiguration{})
	if err != nil {
		t.Fatalf("NewPlatformTunManager() error = %v", err)
	}
	device, err := manager.CreateDevice()
	if err != nil {
		t.Fatalf("CreateDevice() error = %v", err)
	}

	payload := []byte{0x45, 0, 0, 20}
	frame := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(frame[:4], uint32(unix.AF_INET))
	copy(frame[4:], payload)
	if _, err := unix.Write(sockets[1], frame); err != nil {
		t.Fatalf("write framed packet: %v", err)
	}

	got := make([]byte, 64)
	n, err := device.Read(got)
	if err != nil {
		t.Fatalf("device.Read() error = %v", err)
	}
	if !bytes.Equal(got[:n], payload) {
		t.Fatalf("device payload = %v, want %v", got[:n], payload)
	}

	if _, err := device.Write(payload); err != nil {
		t.Fatalf("device.Write() error = %v", err)
	}
	written := make([]byte, 64)
	n, err = unix.Read(sockets[1], written)
	if err != nil {
		t.Fatalf("read framed output: %v", err)
	}
	if !bytes.Equal(written[:n], frame) {
		t.Fatalf("framed output = %v, want %v", written[:n], frame)
	}

	if err := device.Close(); err != nil {
		t.Fatalf("device.Close() error = %v", err)
	}

	sourceStillOpen := []byte("source descriptor remains open")
	if _, err := unix.Write(sockets[1], sourceStillOpen); err != nil {
		t.Fatalf("write peer socket after device close: %v", err)
	}
	got = make([]byte, len(sourceStillOpen))
	if _, err := unix.Read(sockets[0], got); err != nil {
		t.Fatalf("read source socket after device close: %v", err)
	}
	if !bytes.Equal(got, sourceStillOpen) {
		t.Fatalf("source descriptor payload = %q, want %q", got, sourceStillOpen)
	}

	if err := manager.DisposeDevices(); err != nil {
		t.Fatalf("DisposeDevices() error = %v", err)
	}
}

func TestManager_CloseUnblocksDeviceRead(t *testing.T) {
	sockets, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_DGRAM, 0)
	if err != nil {
		t.Fatalf("Socketpair() error = %v", err)
	}
	defer unix.Close(sockets[0])
	defer unix.Close(sockets[1])

	release, err := RegisterNetworkExtensionFileDescriptor(sockets[0])
	if err != nil {
		t.Fatalf("RegisterNetworkExtensionFileDescriptor() error = %v", err)
	}
	defer release()
	manager, err := NewPlatformTunManager(appConfiguration.ClientRuntimeConfiguration{})
	if err != nil {
		t.Fatalf("NewPlatformTunManager() error = %v", err)
	}
	device, err := manager.CreateDevice()
	if err != nil {
		t.Fatalf("CreateDevice() error = %v", err)
	}

	readDone := make(chan error, 1)
	go func() {
		_, err := device.Read(make([]byte, 64))
		readDone <- err
	}()

	if err := device.Close(); err != nil {
		t.Fatalf("device.Close() error = %v", err)
	}
	if err := <-readDone; !errors.Is(err, os.ErrClosed) {
		t.Fatalf("blocked Read() error = %v, want os.ErrClosed", err)
	}
}

func TestRegisterNetworkExtensionFileDescriptorRejectsInvalidDescriptor(t *testing.T) {
	if _, err := RegisterNetworkExtensionFileDescriptor(-1); err == nil {
		t.Fatal("expected invalid descriptor error")
	}
}

func TestRegisterNetworkExtensionFileDescriptorAllowsOneActiveRegistration(t *testing.T) {
	sockets, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_DGRAM, 0)
	if err != nil {
		t.Fatalf("Socketpair() error = %v", err)
	}
	defer unix.Close(sockets[0])
	defer unix.Close(sockets[1])

	release, err := RegisterNetworkExtensionFileDescriptor(sockets[0])
	if err != nil {
		t.Fatalf("first registration error = %v", err)
	}
	if _, err := RegisterNetworkExtensionFileDescriptor(sockets[0]); err == nil {
		t.Fatal("second active registration unexpectedly succeeded")
	}
	release()

	releaseAgain, err := RegisterNetworkExtensionFileDescriptor(sockets[0])
	if err != nil {
		t.Fatalf("registration after release error = %v", err)
	}
	releaseAgain()
}
