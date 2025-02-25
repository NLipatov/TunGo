package udp_chacha20_test

import (
	"fmt"
	"net"
	"testing"
	"time"
	"tungo/client/routing/udp_chacha20"

	"tungo/settings"
)

func TestUDPConnection_Establish(t *testing.T) {
	serverAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serverConn, err := net.ListenUDP("udp", serverAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer func(serverConn *net.UDPConn) {
		_ = serverConn.Close()
	}(serverConn)

	actualAddr := serverConn.LocalAddr().(*net.UDPAddr)

	clientSettings := settings.ConnectionSettings{
		ConnectionIP: "127.0.0.1",
		Port:         fmt.Sprintf("%d", actualAddr.Port),
	}

	udpConn := udp_chacha20.NewConnection(clientSettings)

	clientConn, err := udpConn.Establish()
	if err != nil {
		t.Fatalf("failed to establish connection: %v", err)
	}
	defer func(clientConn *net.UDPConn) {
		_ = clientConn.Close()
	}(clientConn)

	testMsg := []byte("hello")
	n, err := clientConn.Write(testMsg)
	if err != nil {
		t.Fatalf("failed to write: %v", err)
	}
	if n != len(testMsg) {
		t.Errorf("expected to write %d bytes, wrote %d", len(testMsg), n)
	}

	_ = serverConn.SetReadDeadline(time.Now().Add(1 * time.Second))
	buf := make([]byte, 1024)
	n, addr, err := serverConn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("failed to read from UDP: %v", err)
	}

	if string(buf[:n]) != "hello" {
		t.Errorf("expected message 'hello', got '%s'", string(buf[:n]))
	}

	if addr.String() != clientConn.LocalAddr().String() {
		t.Errorf("unexpected sender address: expected %s, got %s", clientConn.LocalAddr().String(), addr.String())
	}
}
