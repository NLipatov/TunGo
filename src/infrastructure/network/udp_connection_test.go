package network

import (
	"errors"
	"net"
	"testing"
	"time"
	"tungo/application"
)

// udpConnectionTestSocketMock implements application.Socket for error and success scenarios.
type udpConnectionTestSocketMock struct {
	addr *net.UDPAddr
	err  error
}

func (m *udpConnectionTestSocketMock) UdpAddr() (*net.UDPAddr, error) {
	return m.addr, m.err
}

func (m *udpConnectionTestSocketMock) StringAddr() string {
	return m.addr.String()
}

// TestEstablish_ErrorFromSocket ensures Establish returns the socket’s UdpAddr error.
func TestEstablish_ErrorFromSocket(t *testing.T) {
	wantErr := errors.New("socket failure")
	conn := NewUdpConnection(&udpConnectionTestSocketMock{err: wantErr})
	_, err := conn.Establish()
	if !errors.Is(err, wantErr) {
		t.Fatalf("Establish() error = %v; want %v", err, wantErr)
	}
}

// TestEstablish_Success verifies that Establish opens a UDP connection and data flows.
func TestEstablish_Success(t *testing.T) {
	// Start a UDP listener on a random port.
	listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer func(listener *net.UDPConn) {
		_ = listener.Close()
	}(listener)

	// Prepare udpConnectionTestSocketMock to return the listener's address.
	serverAddr := listener.LocalAddr().(*net.UDPAddr)
	mock := &udpConnectionTestSocketMock{addr: serverAddr}

	// Establish a connection to the listener.
	conn := NewUdpConnection(mock)
	clientConn, err := conn.Establish()
	if err != nil {
		t.Fatalf("Establish() error: %v", err)
	}
	defer func(clientConn application.ConnectionAdapter) {
		_ = clientConn.Close()
	}(clientConn)

	// Send a single byte.
	sent := []byte{0xAB}
	if _, err := clientConn.Write(sent); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Read it back on the listener side.
	buf := make([]byte, 1)
	_ = listener.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	n, _, err := listener.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("ReadFromUDP failed: %v", err)
	}
	if n != 1 || buf[0] != sent[0] {
		t.Errorf("listener got %v; want %v", buf[:n], sent)
	}
}
