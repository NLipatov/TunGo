package udp

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/netip"
	"testing"
	"time"
)

// ---------------------------
// Mocks
// ---------------------------

type mockQueue struct {
	readResults []struct {
		n   int
		err error
		buf []byte
	}
	readIdx  int
	enqueued [][]byte
	closed   bool
}

func (m *mockQueue) Enqueue(pkt []byte) {
	cp := make([]byte, len(pkt))
	copy(cp, pkt)
	m.enqueued = append(m.enqueued, cp)
}

func (m *mockQueue) ReadInto(dst []byte) (int, error) {
	if m.readIdx >= len(m.readResults) {
		return 0, io.EOF
	}
	r := m.readResults[m.readIdx]
	m.readIdx++

	if len(dst) < r.n {
		return 0, io.ErrShortBuffer
	}
	copy(dst, r.buf[:r.n])
	return r.n, r.err
}

func (m *mockQueue) Close() {
	m.closed = true
}

func listenRegistrationUDP(t *testing.T) *net.UDPConn {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	return conn
}

// ---------------------------
// Tests
// ---------------------------

// Write should forward data to listener.WriteMsgUDPAddrPort.
func TestRegistrationConn_Write_OK(t *testing.T) {
	q := &mockQueue{}
	listener := listenRegistrationUDP(t)
	client := listenRegistrationUDP(t)
	addr := client.LocalAddr().(*net.UDPAddr).AddrPort()
	server := listener.LocalAddr().(*net.UDPAddr).AddrPort()

	adapter := NewRegistrationConn(listener, addr, server.Addr(), q)

	data := []byte{1, 2, 3, 4}

	n, err := adapter.Write(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(data) {
		t.Fatalf("expected %d bytes written, got %d", len(data), n)
	}

	buf := make([]byte, 16)
	n, from, err := client.ReadFromUDPAddrPort(buf)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf[:n], data) || from != server {
		t.Fatalf("received %x from %s; want %x from %s", buf[:n], from, data, server)
	}
}

// Write should forward listener errors.
func TestRegistrationConn_Write_Error(t *testing.T) {
	q := &mockQueue{}
	listener := listenRegistrationUDP(t)
	addr := listener.LocalAddr().(*net.UDPAddr).AddrPort()

	adapter := NewRegistrationConn(listener, addr, addr.Addr(), q)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	n, err := adapter.Write([]byte{9, 9})
	if n != 0 || !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Write() = %d, %v; want 0, net.ErrClosed", n, err)
	}
}

// Read should call queue.ReadInto.
func TestRegistrationConn_Read_OK(t *testing.T) {
	q := &mockQueue{
		readResults: []struct {
			n   int
			err error
			buf []byte
		}{
			{n: 3, buf: []byte{0xaa, 0xbb, 0xcc}},
		},
	}

	addr := netip.MustParseAddrPort("10.0.0.1:8888")

	adapter := NewRegistrationConn(nil, addr, netip.MustParseAddr("192.0.2.1"), q)

	dst := make([]byte, 10)
	n, err := adapter.Read(dst)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 3 {
		t.Fatalf("expected read 3 bytes, got %d", n)
	}
	if !bytes.Equal(dst[:3], []byte{0xaa, 0xbb, 0xcc}) {
		t.Fatalf("data mismatch: %x", dst[:3])
	}
}

// Read should forward errors from queue.ReadInto.
func TestRegistrationConn_Read_Error(t *testing.T) {
	q := &mockQueue{
		readResults: []struct {
			n   int
			err error
			buf []byte
		}{
			{n: 0, err: io.EOF},
		},
	}
	addr := netip.MustParseAddrPort("10.0.0.1:7777")

	adapter := NewRegistrationConn(nil, addr, netip.MustParseAddr("192.0.2.1"), q)

	_, err := adapter.Read(make([]byte, 10))
	if err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
}

// RemoteAddrPort should return the address passed at construction time.
func TestRegistrationConn_RemoteAddrPort(t *testing.T) {
	q := &mockQueue{}
	addr := netip.MustParseAddrPort("10.0.0.1:5555")

	adapter := NewRegistrationConn(nil, addr, netip.MustParseAddr("192.0.2.1"), q)

	type addrPorter interface {
		RemoteAddrPort() netip.AddrPort
	}
	ra, ok := adapter.(addrPorter)
	if !ok {
		t.Fatal("adapter does not implement RemoteAddrPort()")
	}
	if ra.RemoteAddrPort() != addr {
		t.Fatalf("expected %v, got %v", addr, ra.RemoteAddrPort())
	}
}

// Close should do nothing (no panic, no close of UDP socket).
func TestRegistrationConn_Close_NoEffect(t *testing.T) {
	q := &mockQueue{}
	listener := listenRegistrationUDP(t)
	addr := netip.MustParseAddrPort("10.0.0.1:6666")

	adapter := NewRegistrationConn(listener, addr, netip.MustParseAddr("192.0.2.1"), q)

	err := adapter.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := listener.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("shared UDP socket was closed by adapter: %v", err)
	}
}

func TestRegistrationConn_SetAddrPort(t *testing.T) {
	q := &mockQueue{}
	original := netip.MustParseAddrPort("10.0.0.1:9999")
	updated := netip.MustParseAddrPort("10.0.0.2:8888")
	adapter := NewRegistrationConn(nil, original, netip.MustParseAddr("192.0.2.1"), q).(*registrationConn)

	adapter.SetAddrPort(updated)

	if got := adapter.RemoteAddrPort(); got != updated {
		t.Fatalf("RemoteAddrPort() = %v, want %v", got, updated)
	}
}
