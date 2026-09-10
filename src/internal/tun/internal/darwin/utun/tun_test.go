//go:build darwin

package utun

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"tungo/internal/tun/internal/iolifecycle"

	"golang.org/x/sys/unix"
)

func newSocketTun(t *testing.T) (*tun, int) {
	t.Helper()

	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_DGRAM, 0)
	if err != nil {
		t.Fatalf("create socket pair: %v", err)
	}
	socketTun := &tun{fd: fds[0], io: iolifecycle.New()}
	t.Cleanup(func() {
		_ = socketTun.Close()
		_ = unix.Close(fds[1])
	})
	return socketTun, fds[1]
}

func reuseDescriptor(t *testing.T, fd int) int {
	t.Helper()

	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_DGRAM, 0)
	if err != nil {
		t.Fatalf("create replacement socket pair: %v", err)
	}
	if err := unix.Dup2(fds[0], fd); err != nil {
		_ = unix.Close(fds[0])
		_ = unix.Close(fds[1])
		t.Fatalf("reuse descriptor %d: %v", fd, err)
	}
	t.Cleanup(func() {
		_ = unix.Close(fd)
		if fds[0] != fd {
			_ = unix.Close(fds[0])
		}
		_ = unix.Close(fds[1])
	})
	return fds[1]
}

func TestRead(t *testing.T) {
	tun, peer := newSocketTun(t)
	payload := []byte{0x45, 0x11, 0x22, 0x33}
	packet := append(make([]byte, headerLen), payload...)
	if _, err := unix.Write(peer, packet); err != nil {
		t.Fatalf("write packet: %v", err)
	}

	got := make([]byte, len(payload))
	n, err := tun.Read(got)
	if err != nil {
		t.Fatalf("Read returned unexpected error: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("Read returned length %d, want %d", n, len(payload))
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("Read payload = %v, want %v", got, payload)
	}
}

func TestReadRejectsEmptyDestination(t *testing.T) {
	tun := &tun{}
	if _, err := tun.Read(nil); err == nil || err.Error() != "destination slice too small" {
		t.Fatalf("Read error = %v, want destination slice too small", err)
	}
}

func TestReadRejectsMissingHeader(t *testing.T) {
	tun, peer := newSocketTun(t)
	if _, err := unix.Write(peer, []byte{1, 2, 3}); err != nil {
		t.Fatalf("write packet: %v", err)
	}

	if _, err := tun.Read(make([]byte, 1)); err == nil || err.Error() != "short read (no UTUN header)" {
		t.Fatalf("Read error = %v, want short UTUN header", err)
	}
}

func TestWrite(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		family  int
	}{
		{name: "IPv4", payload: []byte{0x45, 0xaa, 0xbb}, family: unix.AF_INET},
		{name: "IPv6", payload: []byte{0x60, 0xde, 0xad}, family: unix.AF_INET6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tun, peer := newSocketTun(t)
			n, err := tun.Write(tt.payload)
			if err != nil {
				t.Fatalf("Write returned unexpected error: %v", err)
			}
			if n != len(tt.payload) {
				t.Fatalf("Write returned length %d, want %d", n, len(tt.payload))
			}

			packet := make([]byte, headerLen+len(tt.payload))
			n, err = unix.Read(peer, packet)
			if err != nil {
				t.Fatalf("read packet: %v", err)
			}
			if n != len(packet) {
				t.Fatalf("packet length = %d, want %d", n, len(packet))
			}
			if family := int(binary.BigEndian.Uint32(packet[:headerLen])); family != tt.family {
				t.Fatalf("address family = %d, want %d", family, tt.family)
			}
			if !bytes.Equal(packet[headerLen:], tt.payload) {
				t.Fatalf("payload = %v, want %v", packet[headerLen:], tt.payload)
			}
		})
	}
}

func TestWriteRejectsEmptyPacket(t *testing.T) {
	tun := &tun{}
	if _, err := tun.Write(nil); err == nil || err.Error() != "empty packet" {
		t.Fatalf("Write error = %v, want empty packet", err)
	}
}

func TestReadRejectsReusedDescriptor(t *testing.T) {
	tun, _ := newSocketTun(t)
	fd := tun.fd
	if err := tun.Close(); err != nil {
		t.Fatalf("close tun: %v", err)
	}

	peer := reuseDescriptor(t, fd)
	packet := []byte{0, 0, 0, unix.AF_INET, 0x45}
	if _, err := unix.Write(peer, packet); err != nil {
		t.Fatalf("write replacement socket: %v", err)
	}

	if _, err := tun.Read(make([]byte, 1)); !errors.Is(err, unix.EBADF) {
		t.Fatalf("Read error = %v, want EBADF", err)
	}
}

func TestWriteRejectsReusedDescriptor(t *testing.T) {
	tun, _ := newSocketTun(t)
	fd := tun.fd
	if err := tun.Close(); err != nil {
		t.Fatalf("close tun: %v", err)
	}

	reuseDescriptor(t, fd)
	if _, err := tun.Write([]byte{0x45}); !errors.Is(err, unix.EBADF) {
		t.Fatalf("Write error = %v, want EBADF", err)
	}
}

func TestCloseUnblocksRead(t *testing.T) {
	tun, _ := newSocketTun(t)
	readDone := make(chan error, 1)
	readStarted := make(chan struct{})
	go func() {
		close(readStarted)
		_, err := tun.Read(make([]byte, 1))
		readDone <- err
	}()

	<-readStarted
	time.Sleep(10 * time.Millisecond)

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- tun.Close()
	}()

	select {
	case err := <-readDone:
		if err == nil {
			t.Fatal("Read error = nil after Close")
		}
	case <-time.After(time.Second):
		_ = unix.Close(tun.fd)
		t.Fatal("Close did not unblock Read")
	}
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not return after Read stopped")
	}
}

func TestCloseUnblocksWrite(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("create socket pair: %v", err)
	}
	tun := &tun{fd: fds[0], io: iolifecycle.New()}
	t.Cleanup(func() {
		_ = tun.Close()
		_ = unix.Close(fds[1])
	})

	if err := unix.SetNonblock(tun.fd, true); err != nil {
		t.Fatalf("make tun socket non-blocking: %v", err)
	}
	for {
		if _, err := unix.Write(tun.fd, []byte{0}); err != nil {
			if errors.Is(err, unix.EAGAIN) {
				break
			}
			t.Fatalf("fill tun socket send buffer: %v", err)
		}
	}
	if err := unix.SetNonblock(tun.fd, false); err != nil {
		t.Fatalf("make tun socket blocking: %v", err)
	}

	writeDone := make(chan error, 1)
	writeStarted := make(chan struct{})
	go func() {
		close(writeStarted)
		_, err := tun.Write([]byte{0x45})
		writeDone <- err
	}()

	<-writeStarted
	time.Sleep(10 * time.Millisecond)

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- tun.Close()
	}()

	select {
	case err := <-writeDone:
		if err == nil {
			t.Fatal("Write error = nil after Close")
		}
	case <-time.After(time.Second):
		_ = unix.Close(tun.fd)
		t.Fatal("Close did not unblock Write")
	}
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not return after Write stopped")
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	tun, _ := newSocketTun(t)
	if err := tun.Close(); err != nil {
		t.Fatalf("first Close error = %v, want nil", err)
	}
	if err := tun.Close(); err != nil {
		t.Fatalf("second Close error = %v, want nil", err)
	}
}

func TestCloseCachesError(t *testing.T) {
	tun := &tun{fd: -1}
	if err := tun.Close(); !errors.Is(err, unix.EBADF) {
		t.Fatalf("first Close error = %v, want EBADF", err)
	}
	if err := tun.Close(); !errors.Is(err, unix.EBADF) {
		t.Fatalf("second Close error = %v, want EBADF", err)
	}
}
