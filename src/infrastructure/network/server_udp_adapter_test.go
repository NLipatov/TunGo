package network

import (
	"errors"
	"io"
	"net"
	"net/netip"
	"testing"
	"time"
	"tungo/application"
)

// setupConns creates a server and client UDPConns and returns them plus a ConnectionAdapter.
func setupConns(t *testing.T) (serverConn *net.UDPConn, clientConn *net.UDPConn, clientAdapter application.ConnectionAdapter) {
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
	return serverConn, clientConn, clientAdapter
}

func teardownConns(serverConn, clientConn *net.UDPConn) {
	_ = serverConn.Close()
	_ = clientConn.Close()
}

func TestUdpAdapter_Write(t *testing.T) {
	serverConn, clientConn, clientAdapter := setupConns(t)
	defer teardownConns(serverConn, clientConn)

	msg := []byte("ping")
	n, err := clientAdapter.Write(msg)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(msg) {
		t.Errorf("Write wrote %d, want %d", n, len(msg))
	}

	buf := make([]byte, 16)
	n2, _, _, _, err := serverConn.ReadMsgUDPAddrPort(buf, nil)
	if err != nil {
		t.Fatalf("server Read: %v", err)
	}
	if string(buf[:n2]) != string(msg) {
		t.Errorf("server got %q, want %q", buf[:n2], msg)
	}
}

func TestUdpAdapter_Read(t *testing.T) {
	serverConn, clientConn, clientAdapter := setupConns(t)
	defer teardownConns(serverConn, clientConn)

	resp := []byte("pong")
	clientAddrPort, err := netip.ParseAddrPort(clientConn.LocalAddr().String())
	if err != nil {
		t.Fatalf("parse client addrport: %v", err)
	}
	n, err := serverConn.WriteToUDPAddrPort(resp, clientAddrPort)
	if err != nil {
		t.Fatalf("server WriteToUDPAddrPort: %v", err)
	}
	if n != len(resp) {
		t.Errorf("server wrote %d, want %d", n, len(resp))
	}

	buf := make([]byte, 16)
	_ = clientConn.SetReadDeadline(time.Now().Add(time.Second))
	n2, err := clientAdapter.Read(buf)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if string(buf[:n2]) != string(resp) {
		t.Errorf("adapter.Read got %q, want %q", buf[:n2], resp)
	}
}

func TestUdpAdapter_Close(t *testing.T) {
	serverConn, clientConn, clientAdapter := setupConns(t)
	defer teardownConns(serverConn, clientConn)

	if err := clientAdapter.Close(); err != nil {
		t.Errorf("Close error: %v", err)
	}

	buf := make([]byte, 4)
	_, err := clientAdapter.Read(buf)
	if err == nil {
		t.Error("expected error after Close, got nil")
	}
}

func TestUdpAdapter_ReadShortBuffer(t *testing.T) {
	serverConn, clientConn, clientAdapter := setupConns(t)
	defer teardownConns(serverConn, clientConn)

	resp := []byte("too big")
	clientAddrPort, err := netip.ParseAddrPort(clientConn.LocalAddr().String())
	if err != nil {
		t.Fatalf("parse client addrport: %v", err)
	}
	if _, err := serverConn.WriteToUDPAddrPort(resp, clientAddrPort); err != nil {
		t.Fatalf("server WriteToUDPAddrPort: %v", err)
	}

	buf := make([]byte, 4)
	_ = clientConn.SetReadDeadline(time.Now().Add(time.Second))
	if n, err := clientAdapter.Read(buf); !errors.Is(err, io.ErrShortBuffer) || n != 0 {
		t.Fatalf("want io.ErrShortBuffer, got (%d,%v)", n, err)
	}
}
