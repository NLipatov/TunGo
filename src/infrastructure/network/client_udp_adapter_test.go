package network

import (
	"net"
	"testing"
	"time"
	"tungo/application"
)

func TestClientUdpAdapter(t *testing.T) {
	// setup server
	srvAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ResolveUDPAddr: %v", err)
	}
	srv, err := net.ListenUDP("udp", srvAddr)
	if err != nil {
		t.Fatalf("ListenUDP: %v", err)
	}
	defer func(srv *net.UDPConn) {
		_ = srv.Close()
	}(srv)

	// setup client adapter
	cliConn, err := net.DialUDP("udp", nil, srv.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatalf("DialUDP: %v", err)
	}
	adapter := NewClientUdpAdapter(cliConn)
	defer func(adapter application.ConnectionAdapter) {
		_ = adapter.Close()
	}(adapter)

	// Write
	msg := []byte("hello")
	if n, err := adapter.Write(msg); err != nil || n != len(msg) {
		t.Fatalf("Write: n=%d err=%v", n, err)
	}

	// server reads
	buf := make([]byte, 1024)
	_ = srv.SetReadDeadline(time.Now().Add(time.Second))
	n, addr, err := srv.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("ReadFromUDP: %v", err)
	}
	if string(buf[:n]) != string(msg) {
		t.Fatalf("got %q want %q", buf[:n], msg)
	}

	// server replies
	reply := []byte("world")
	_ = srv.SetWriteDeadline(time.Now().Add(time.Second))
	if _, err := srv.WriteToUDP(reply, addr); err != nil {
		t.Fatalf("WriteToUDP: %v", err)
	}

	// Read
	readBuf := make([]byte, 1024)
	_ = cliConn.SetReadDeadline(time.Now().Add(time.Second))
	n2, err := adapter.Read(readBuf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(readBuf[:n2]) != string(reply) {
		t.Fatalf("got %q want %q", readBuf[:n2], reply)
	}

	// Close
	if err := adapter.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := adapter.Write([]byte("x")); err == nil {
		t.Fatal("expected error after Close")
	}
}
