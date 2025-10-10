package adapters

import (
	"net"
	"net/netip"
	"testing"
	"time"
	"tungo/application/network/connection"
)

// setupConns creates a server and client UDPConns and returns them plus a Transport.
func setupConns(t testing.TB) (serverConn *net.UDPConn, clientConn *net.UDPConn, clientAdapter connection.Transport) {
	t.Helper()

	serverConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("setup server: %v", err)
	}
	clientConn, err = net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		_ = serverConn.Close()
		t.Fatalf("setup client: %v", err)
	}

	addrPort, err := netip.ParseAddrPort(serverConn.LocalAddr().String())
	if err != nil {
		_ = serverConn.Close()
		_ = clientConn.Close()
		t.Fatalf("parse server addrport: %v", err)
	}

	clientAdapter = NewUdpAdapter(clientConn, addrPort)

	t.Cleanup(func() {
		_ = serverConn.Close()
		_ = clientConn.Close()
	})
	return serverConn, clientConn, clientAdapter
}

func TestUdpAdapter_Write(t *testing.T) {
	serverConn, clientConn, clientAdapter := setupConns(t)

	msg := []byte("ping")
	n, err := clientAdapter.Write(msg)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(msg) {
		t.Errorf("Write wrote %d, want %d", n, len(msg))
	}

	_ = serverConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 16)
	n2, _, _, _, err := serverConn.ReadMsgUDPAddrPort(buf, nil)
	if err != nil {
		t.Fatalf("server Read: %v", err)
	}
	if string(buf[:n2]) != string(msg) {
		t.Errorf("server got %q, want %q", buf[:n2], msg)
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
}

func TestUdpAdapter_Read(t *testing.T) {
	serverConn, clientConn, clientAdapter := setupConns(t)

	resp := []byte("pong")
	clientAddrPort, err := netip.ParseAddrPort(clientConn.LocalAddr().String())
	if err != nil {
		t.Fatalf("parse client addrport: %v", err)
	}
	if n, err := serverConn.WriteToUDPAddrPort(resp, clientAddrPort); err != nil || n != len(resp) {
		t.Fatalf("server WriteToUDPAddrPort: n=%d err=%v", n, err)
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 16)
	n2, err := clientAdapter.Read(buf)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if string(buf[:n2]) != string(resp) {
		t.Errorf("adapter.Read got %q, want %q", buf[:n2], resp)
	}
}

func TestUdpAdapter_Close_then_Read(t *testing.T) {
	_, clientConn, clientAdapter := setupConns(t)

	if err := clientAdapter.Close(); err != nil {
		t.Errorf("Close error: %v", err)
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 4)
	if _, err := clientAdapter.Read(buf); err == nil {
		t.Error("expected error after Close, got nil")
	}
}

func TestUdpAdapter_WriteAfterClose(t *testing.T) {
	_, _, clientAdapter := setupConns(t)

	if err := clientAdapter.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
	if n, err := clientAdapter.Write([]byte("x")); err == nil || n != 0 {
		t.Fatalf("expected write error after Close, got n=%d err=%v", n, err)
	}
}

func TestUdpAdapter_ReadShortBuffer(t *testing.T) {
	serverConn, clientConn, clientAdapter := setupConns(t)

	resp := []byte("too big")
	clientAddrPort, err := netip.ParseAddrPort(clientConn.LocalAddr().String())
	if err != nil {
		t.Fatalf("parse client addrport: %v", err)
	}
	if _, err := serverConn.WriteToUDPAddrPort(resp, clientAddrPort); err != nil {
		t.Fatalf("server WriteToUDPAddrPort: %v", err)
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 4)
	n, readErr := clientAdapter.Read(buf)
	if readErr != nil {
		t.Fatalf("unexpected error: %v", readErr)
	}
	if n != len(buf) {
		t.Errorf("expected n=%d, got %d", len(buf), n)
	}
	if string(buf[:n]) != string(resp[:len(buf)]) {
		t.Errorf("expected %q, got %q", resp[:len(buf)], buf[:n])
	}
}
