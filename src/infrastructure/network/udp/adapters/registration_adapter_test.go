package adapters

import (
	"bytes"
	"errors"
	"io"
	"net/netip"
	"testing"
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

type mockUdpListener struct {
	writes []struct {
		data []byte
		addr netip.AddrPort
	}
	writeErr error

	closed           bool
	setReadBufCalls  int
	setWriteBufCalls int
}

func (m *mockUdpListener) Close() error {
	m.closed = true
	return nil
}

func (m *mockUdpListener) ReadMsgUDPAddrPort(_, _ []byte) (int, int, int, netip.AddrPort, error) {
	return 0, 0, 0, netip.AddrPort{}, errors.New("should not be called in adapter tests")
}

func (m *mockUdpListener) SetReadBuffer(_ int) error {
	m.setReadBufCalls++
	return nil
}

func (m *mockUdpListener) SetWriteBuffer(_ int) error {
	m.setWriteBufCalls++
	return nil
}

func (m *mockUdpListener) WriteToUDPAddrPort(data []byte, addr netip.AddrPort) (int, error) {
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	m.writes = append(m.writes, struct {
		data []byte
		addr netip.AddrPort
	}{cp, addr})
	return len(data), nil
}

// ---------------------------
// Tests
// ---------------------------

// Write should forward data to listener.WriteToUDPAddrPort.
func TestRegistrationAdapter_Write_OK(t *testing.T) {
	q := &mockQueue{}
	ul := &mockUdpListener{}
	addr := netip.MustParseAddrPort("10.0.0.1:9999")

	adapter := NewRegistrationTransport(ul, addr, q)

	data := []byte{1, 2, 3, 4}

	n, err := adapter.Write(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(data) {
		t.Fatalf("expected %d bytes written, got %d", len(data), n)
	}

	if len(ul.writes) != 1 {
		t.Fatalf("expected 1 write, got %d", len(ul.writes))
	}

	if !bytes.Equal(ul.writes[0].data, data) {
		t.Fatalf("written data mismatch")
	}

	if ul.writes[0].addr != addr {
		t.Fatalf("expected write to %v, got %v", addr, ul.writes[0].addr)
	}
}

// Write should forward listener errors.
func TestRegistrationAdapter_Write_Error(t *testing.T) {
	q := &mockQueue{}
	ul := &mockUdpListener{writeErr: errors.New("write fail")}
	addr := netip.MustParseAddrPort("10.0.0.1:9999")

	adapter := NewRegistrationTransport(ul, addr, q)

	_, err := adapter.Write([]byte{9, 9})
	if err == nil || err.Error() != "write fail" {
		t.Fatalf("expected write fail, got %v", err)
	}
}

// Read should call queue.ReadInto.
func TestRegistrationAdapter_Read_OK(t *testing.T) {
	q := &mockQueue{
		readResults: []struct {
			n   int
			err error
			buf []byte
		}{
			{n: 3, buf: []byte{0xaa, 0xbb, 0xcc}},
		},
	}

	ul := &mockUdpListener{}
	addr := netip.MustParseAddrPort("10.0.0.1:8888")

	adapter := NewRegistrationTransport(ul, addr, q)

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
func TestRegistrationAdapter_Read_Error(t *testing.T) {
	q := &mockQueue{
		readResults: []struct {
			n   int
			err error
			buf []byte
		}{
			{n: 0, err: io.EOF},
		},
	}
	ul := &mockUdpListener{}
	addr := netip.MustParseAddrPort("10.0.0.1:7777")

	adapter := NewRegistrationTransport(ul, addr, q)

	_, err := adapter.Read(make([]byte, 10))
	if err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
}

// Close should do nothing (no panic, no close of UDP socket).
func TestRegistrationAdapter_Close_NoEffect(t *testing.T) {
	q := &mockQueue{}
	ul := &mockUdpListener{}
	addr := netip.MustParseAddrPort("10.0.0.1:6666")

	adapter := NewRegistrationTransport(ul, addr, q)

	err := adapter.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ul.closed {
		t.Fatalf("Close() on adapter must NOT close UDP listener")
	}
}
