package handshake

import (
	"bytes"
	"errors"
	"net"
	"testing"
	"time"
	"tungo/infrastructure/settings"
)

// --- fake connections ---

// fakeAdapter implements application.ConnectionAdapter
type fakeAdapter struct {
	in       *bytes.Buffer
	out      bytes.Buffer
	readErr  error
	writeErr error
}

func (f *fakeAdapter) Read(p []byte) (int, error) {
	if f.readErr != nil {
		return 0, f.readErr
	}
	return f.in.Read(p)
}

func (f *fakeAdapter) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return f.out.Write(p)
}

func (f *fakeAdapter) Close() error { return nil }

// badNetConn wraps fakeAdapter to satisfy net.Conn
type badNetConn struct{ *fakeAdapter }

func (b *badNetConn) LocalAddr() net.Addr                { return nil }
func (b *badNetConn) RemoteAddr() net.Addr               { return nil }
func (b *badNetConn) SetDeadline(_ time.Time) error      { return nil }
func (b *badNetConn) SetReadDeadline(_ time.Time) error  { return nil }
func (b *badNetConn) SetWriteDeadline(_ time.Time) error { return nil }

// --- tests for ServerSideHandshake ---

func TestServerSideHandshake_ReadClientHelloError(t *testing.T) {
	h := NewHandshake()
	adapter := &fakeAdapter{readErr: errors.New("read-fail")}
	_, err := h.ServerSideHandshake(adapter)
	if err == nil {
		t.Fatal("expected error reading ClientHello, got nil")
	}
}

// --- tests for ClientSideHandshake ---

func TestClientSideHandshake_WriteHelloError(t *testing.T) {
	h := NewHandshake()
	// a net.Conn whose Write always fails
	bad := &badNetConn{&fakeAdapter{writeErr: errors.New("boom")}}
	err := h.ClientSideHandshake(bad, settings.Settings{
		InterfaceAddress: "10.0.0.2", ConnectionIP: "127.0.0.1", Port: "9999",
	})
	if err == nil {
		t.Fatal("expected error on WriteClientHello, got nil")
	}
}

func TestClientSideHandshake_ReadServerHelloError(t *testing.T) {
	h := NewHandshake()
	// first WriteClientHello will succeed (no writeErr),
	// then ReadServerHello (Read) will fail
	buf := bytes.Repeat([]byte{0}, MaxClientHelloSizeBytes)
	conn := &badNetConn{&fakeAdapter{in: bytes.NewBuffer(buf), readErr: errors.New("recv-fail")}}
	err := h.ClientSideHandshake(conn, settings.Settings{
		InterfaceAddress: "10.0.0.2", ConnectionIP: "127.0.0.1", Port: "9999",
	})
	if err == nil {
		t.Fatal("expected error on ReadServerHello, got nil")
	}
}
