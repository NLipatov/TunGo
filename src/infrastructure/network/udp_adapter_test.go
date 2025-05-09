package network

import (
	"net"
	"net/netip"
	"testing"
	"time"
)

// Test that UdpAdapter implements ConnectionAdapter and performs I/O correctly.
func TestUdpAdapter_ReadWriteClose(t *testing.T) {
	// prepare server socket
	serverConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("setup server: %v", err)
	}
	defer func(serverConn *net.UDPConn) {
		_ = serverConn.Close()
	}(serverConn)

	// prepare client socket
	clientConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("setup client: %v", err)
	}
	defer func(clientConn *net.UDPConn) {
		_ = clientConn.Close()
	}(clientConn)

	// wrap client socket in UdpAdapter targeting server address
	serverAddrPort, err := netip.ParseAddrPort(serverConn.LocalAddr().String())
	if err != nil {
		t.Fatalf("parse server addrport: %v", err)
	}
	clientAdapter := NewUdpAdapter(clientConn, serverAddrPort)

	// Write should send to serverConn
	msg := []byte("ping")
	n, err := clientAdapter.Write(msg)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(msg) {
		t.Errorf("Write wrote %d, want %d", n, len(msg))
	}

	// server receives via ReadMsgUDPAddrPort
	buf := make([]byte, 16)
	n2, _, _, _, err := serverConn.ReadMsgUDPAddrPort(buf, nil)
	if err != nil {
		t.Fatalf("server ReadMsgUDPAddrPort: %v", err)
	}
	if string(buf[:n2]) != string(msg) {
		t.Errorf("server got %q, want %q", buf[:n2], msg)
	}

	// server sends back response to client
	resp := []byte("pong")
	clientAddrPort, err := netip.ParseAddrPort(clientConn.LocalAddr().String())
	if err != nil {
		t.Fatalf("parse client addrport: %v", err)
	}
	n3, err := serverConn.WriteToUDPAddrPort(resp, clientAddrPort)
	if err != nil {
		t.Fatalf("server WriteToUDPAddrPort: %v", err)
	}
	if n3 != len(resp) {
		t.Errorf("server wrote %d, want %d", n3, len(resp))
	}

	// Read should get response
	buf2 := make([]byte, 16)
	_ = clientConn.SetReadDeadline(time.Now().Add(time.Second))
	n4, err := clientAdapter.Read(buf2)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if string(buf2[:n4]) != string(resp) {
		t.Errorf("adapter.Read got %q, want %q", buf2[:n4], resp)
	}

	// Close underlying and ensure adapter.Close propagates
	if err := clientAdapter.Close(); err != nil {
		t.Errorf("Close error: %v", err)
	}

	// After close, Read should error
	_, err = clientAdapter.Read(buf2)
	if err == nil {
		t.Error("expected error after Close, got nil")
	}
}
