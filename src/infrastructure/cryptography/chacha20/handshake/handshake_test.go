package handshake

import (
	"bytes"
	"errors"
	"net"
	"testing"
	"time"
	"tungo/infrastructure/network/ip"
	"tungo/infrastructure/settings"
)

// --- fake connections ---

// fakeAdapter implements application.Transport
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
	h := NewHandshake(make([]byte, 0), make([]byte, 0))
	adapter := &fakeAdapter{readErr: errors.New("read-fail")}
	_, err := h.ServerSideHandshake(adapter)
	if err == nil {
		t.Fatal("expected error reading ClientHello, got nil")
	}
}

func TestServerHandshakeReceiveClientHello_LegacyClient(t *testing.T) {
	buf, orig := buildValidHello(t, ip.V4, "10.0.0.50")
	legacyBuf := buf[:len(buf)-mtuFieldLength]

	adapter := &fakeAdapter{in: bytes.NewBuffer(legacyBuf)}
	server := NewServerHandshake(adapter)

	hello, err := server.ReceiveClientHello()
	if err != nil {
		t.Fatalf("ReceiveClientHello failed for legacy payload: %v", err)
	}

	if !hello.ipAddress.Equal(orig.ipAddress) {
		t.Fatalf("ipAddress mismatch: got %v want %v", hello.ipAddress, orig.ipAddress)
	}

	if _, ok := hello.MTU(); ok {
		t.Fatal("expected legacy ClientHello to omit MTU extension")
	}
}

func TestDefaultHandshake_PeerMTU_Default(t *testing.T) {
	h := NewHandshake(make([]byte, 0), make([]byte, 0))
	if _, ok := h.PeerMTU(); ok {
		t.Fatal("expected no peer MTU before handshake")
	}
}

// --- tests for ClientSideHandshake ---

func TestClientSideHandshake_WriteHelloError(t *testing.T) {
	h := NewHandshake(make([]byte, 0), make([]byte, 0))
	// a net.conn whose Write always fails
	bad := &badNetConn{&fakeAdapter{writeErr: errors.New("boom")}}
	err := h.ClientSideHandshake(bad, settings.Settings{
		InterfaceAddress: "10.0.0.2", ConnectionIP: "127.0.0.1", Port: "9999",
	})
	if err == nil {
		t.Fatal("expected error on WriteClientHello, got nil")
	}
}

func TestClientSideHandshake_ReadServerHelloError(t *testing.T) {
	h := NewHandshake(make([]byte, 0), make([]byte, 0))
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
