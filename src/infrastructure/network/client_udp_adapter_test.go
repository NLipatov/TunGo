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

// ensure adapter.Write incurs no heap allocations
func TestClientUdpAdapter_Write_NoAllocs(t *testing.T) {
	// set up a dummy UDP server
	srvAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	srv, _ := net.ListenUDP("udp", srvAddr)
	defer func(srv *net.UDPConn) {
		_ = srv.Close()
	}(srv)

	// dial it
	cli, _ := net.DialUDP("udp", nil, srv.LocalAddr().(*net.UDPAddr))
	adapter := NewClientUdpAdapter(cli)
	defer func(adapter application.ConnectionAdapter) {
		_ = adapter.Close()
	}(adapter)

	// run once to warm up any runtime paths
	_, _ = adapter.Write([]byte("warmup"))

	// measure allocations per Write
	const runs = 50
	allocs := testing.AllocsPerRun(runs, func() {
		_, _ = adapter.Write([]byte("ping"))
	})
	if allocs > 0 {
		t.Errorf("Write allocated %v objects; want 0", allocs)
	}
}

// ensure adapter.Read incurs no heap allocations
func TestClientUdpAdapter_Read_NoAllocs(t *testing.T) {
	// set up server that will reply immediately
	srvAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	srv, _ := net.ListenUDP("udp", srvAddr)
	defer func(srv *net.UDPConn) {
		_ = srv.Close()
	}(srv)

	cli, _ := net.DialUDP("udp", nil, srv.LocalAddr().(*net.UDPAddr))
	adapter := NewClientUdpAdapter(cli)
	defer func(adapter application.ConnectionAdapter) {
		_ = adapter.Close()
	}(adapter)

	// server goroutine to echo back one packet
	go func() {
		buf := make([]byte, 64)
		_ = srv.SetReadDeadline(time.Now().Add(time.Second))
		n, addr, _ := srv.ReadFromUDP(buf)
		_, _ = srv.WriteToUDP(buf[:n], addr)
	}()

	// warm up
	_, _ = adapter.Write([]byte("warmup"))
	tmp := make([]byte, 64)
	_ = cli.SetReadDeadline(time.Now().Add(time.Second))
	_, _ = adapter.Read(tmp)

	// measure allocations per Read
	const runs = 50
	allocs := testing.AllocsPerRun(runs, func() {
		_ = cli.SetReadDeadline(time.Now().Add(time.Second))
		_, _ = adapter.Read(tmp)
	})
	if allocs > 0 {
		t.Errorf("Read allocated %v objects; want 0", allocs)
	}
}
