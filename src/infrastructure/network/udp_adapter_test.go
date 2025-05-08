package network

import (
	"bytes"
	"net"
	"net/netip"
	"testing"
	"time"
)

func TestUdpAdapterWrite(t *testing.T) {
	serverAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ResolveUDPAddr error: %v", err)
	}
	serverConn, err := net.ListenUDP("udp", serverAddr)
	if err != nil {
		t.Fatalf("ListenUDP error: %v", err)
	}
	defer serverConn.Close()

	clientAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ResolveUDPAddr error: %v", err)
	}
	clientConn, err := net.ListenUDP("udp", clientAddr)
	if err != nil {
		t.Fatalf("ListenUDP error: %v", err)
	}
	defer clientConn.Close()

	serverUDPAddr := serverConn.LocalAddr().(*net.UDPAddr)
	addrPort := netip.MustParseAddrPort(serverUDPAddr.String())

	adapter := UdpAdapter{
		UdpConn:  clientConn,
		AddrPort: addrPort,
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
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ResolveUDPAddr error: %v", err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("ListenUDP error: %v", err)
	}
	defer conn.Close()

	initData := []byte("initial")
	adapter := UdpAdapter{
		UdpConn:     conn,
		InitialData: initData,
	}

	buf := make([]byte, 1024)
	n, err := adapter.Read(buf)
	if err != nil {
		t.Fatalf("UdpAdapter Read error: %v", err)
	}
	if !bytes.Equal(buf[:n], initData) {
		t.Errorf("Expected %q, got %q", initData, buf[:n])
	}

	_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, err = adapter.Read(buf)
	if err == nil {
		t.Errorf("Expected timeout error due to no data after InitialData was consumed")
	}
}

func TestUdpAdapterRead_Normal(t *testing.T) {
	clientAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ResolveUDPAddr error: %v", err)
	}
	clientConn, err := net.ListenUDP("udp", clientAddr)
	if err != nil {
		t.Fatalf("ListenUDP error: %v", err)
	}
	defer clientConn.Close()

	adapter := UdpAdapter{
		UdpConn: clientConn,
	}

	clientUDPAddr := clientConn.LocalAddr().(*net.UDPAddr)
	serverConn, err := net.DialUDP("udp", nil, clientUDPAddr)
	if err != nil {
		t.Fatalf("DialUDP error: %v", err)
	}
	defer serverConn.Close()

	data := []byte("normal read")
	n, err := serverConn.Write(data)
	if err != nil {
		t.Fatalf("Server Write error: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Write: expected %d bytes, got %d", len(data), n)
	}

	buf := make([]byte, 1024)
	_ = clientConn.SetReadDeadline(time.Now().Add(1 * time.Second))
	n, err = adapter.Read(buf)
	if err != nil {
		t.Fatalf("UdpAdapter Read error: %v", err)
	}
	if !bytes.Equal(buf[:n], data) {
		t.Errorf("Adapter Read: got %q, expected %q", buf[:n], data)
	}
}
