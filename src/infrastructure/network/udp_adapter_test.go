package network

import (
	"bytes"
	"net"
	"testing"
	"time"
)

func TestUdpAdapterWrite(t *testing.T) {
	// Create a UDP server listening on a random port.
	serverAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ResolveUDPAddr error: %v", err)
	}
	serverConn, err := net.ListenUDP("udp", serverAddr)
	if err != nil {
		t.Fatalf("ListenUDP error: %v", err)
	}
	defer func(serverConn *net.UDPConn) {
		_ = serverConn.Close()
	}(serverConn)

	// Create a UDP client for sending data.
	clientAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ResolveUDPAddr error: %v", err)
	}
	clientConn, err := net.ListenUDP("udp", clientAddr)
	if err != nil {
		t.Fatalf("ListenUDP error: %v", err)
	}
	defer func(clientConn *net.UDPConn) {
		_ = clientConn.Close()
	}(clientConn)

	// Use the server's address for sending.
	serverUDPAddr := serverConn.LocalAddr().(*net.UDPAddr)

	adapter := UdpAdapter{
		Conn: *clientConn,    // Convert *net.UDPConn to net.UDPConn
		Addr: *serverUDPAddr, // Server address
	}

	data := []byte("udp test")
	n, err := adapter.Write(data)
	if err != nil {
		t.Fatalf("UdpAdapter Write error: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Write: expected %d bytes, got %d", len(data), n)
	}

	buf := make([]byte, 1024)
	// Set a short deadline to prevent blocking.
	_ = serverConn.SetReadDeadline(time.Now().Add(1 * time.Second))
	n, _, err = serverConn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("ReadFromUDP error on server: %v", err)
	}
	if !bytes.Equal(buf[:n], data) {
		t.Errorf("Server received %q, expected %q", buf[:n], data)
	}
}

func TestUdpAdapterRead_InitialData(t *testing.T) {
	// Create a dummy UDP connection to fill the Conn field.
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ResolveUDPAddr error: %v", err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("ListenUDP error: %v", err)
	}
	defer func(conn *net.UDPConn) {
		_ = conn.Close()
	}(conn)

	initData := []byte("initial")
	adapter := UdpAdapter{
		Conn:        *conn,
		InitialData: initData,
	}

	buf := make([]byte, 1024)
	// First read should return the InitialData.
	n, err := adapter.Read(buf)
	if err != nil {
		t.Fatalf("UdpAdapter Read error: %v", err)
	}
	if !bytes.Equal(buf[:n], initData) {
		t.Errorf("Expected %q, got %q", initData, buf[:n])
	}

	// Subsequent call should attempt to read from the socket.
	_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, err = adapter.Read(buf)
	// Expect a timeout error or zero bytes read.
	if err == nil {
		t.Errorf("Expected an error due to no data after InitialData was consumed")
	}
}

func TestUdpAdapterRead_Normal(t *testing.T) {
	// Create a UDP client to read data through the adapter.
	clientAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ResolveUDPAddr error: %v", err)
	}
	clientConn, err := net.ListenUDP("udp", clientAddr)
	if err != nil {
		t.Fatalf("ListenUDP error: %v", err)
	}
	defer func(clientConn *net.UDPConn) {
		_ = clientConn.Close()
	}(clientConn)

	adapter := UdpAdapter{
		Conn: *clientConn,
		// No InitialData provided.
	}

	// Simulate a server sending data to the client.
	clientUDPAddr := clientConn.LocalAddr().(*net.UDPAddr)
	serverConn, err := net.DialUDP("udp", nil, clientUDPAddr)
	if err != nil {
		t.Fatalf("DialUDP error: %v", err)
	}
	defer func(serverConn *net.UDPConn) {
		_ = serverConn.Close()
	}(serverConn)

	data := []byte("normal read")
	n, err := serverConn.Write(data)
	if err != nil {
		t.Fatalf("Server Write error: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Write: expected %d bytes, got %d", len(data), n)
	}

	buf := make([]byte, 1024)
	// Set a deadline to prevent blocking.
	_ = clientConn.SetReadDeadline(time.Now().Add(1 * time.Second))
	n, err = adapter.Read(buf)
	if err != nil {
		t.Fatalf("UdpAdapter Read error: %v", err)
	}
	if !bytes.Equal(buf[:n], data) {
		t.Errorf("Adapter Read: got %q, expected %q", buf[:n], data)
	}
}
