//go:build darwin

package utun

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

func newSocketTun(t *testing.T) (*tun, int) {
	t.Helper()

	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_DGRAM, 0)
	if err != nil {
		t.Fatalf("create socket pair: %v", err)
	}
	socketTun := &tun{fd: fds[0]}
	t.Cleanup(func() {
		if socketTun.fd >= 0 {
			_ = unix.Close(socketTun.fd)
		}
		_ = unix.Close(fds[1])
	})
	return socketTun, fds[1]
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

func TestReadReturnsSocketError(t *testing.T) {
	tun, _ := newSocketTun(t)
	if err := tun.Close(); err != nil {
		t.Fatalf("close tun: %v", err)
	}

	if _, err := tun.Read(make([]byte, 1)); !errors.Is(err, unix.EBADF) {
		t.Fatalf("Read error = %v, want EBADF", err)
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

func TestWriteReturnsSocketError(t *testing.T) {
	tun, _ := newSocketTun(t)
	if err := tun.Close(); err != nil {
		t.Fatalf("close tun: %v", err)
	}

	if _, err := tun.Write([]byte{0x45}); !errors.Is(err, unix.EBADF) {
		t.Fatalf("Write error = %v, want EBADF", err)
	}
}

func TestClose(t *testing.T) {
	tun, _ := newSocketTun(t)
	if err := tun.Close(); err != nil {
		t.Fatalf("Close error = %v, want nil", err)
	}
}

func TestCloseReturnsSocketError(t *testing.T) {
	tun, _ := newSocketTun(t)
	if err := tun.Close(); err != nil {
		t.Fatalf("first Close error = %v", err)
	}
	if err := tun.Close(); !errors.Is(err, unix.EBADF) {
		t.Fatalf("second Close error = %v, want EBADF", err)
	}
}
