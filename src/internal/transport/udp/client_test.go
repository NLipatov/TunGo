package udp

import (
	"errors"
	"io"
	"net"
	"os"
	"testing"
	"time"
)

// helper: returns client-side adapter and matching UDP server socket
func newPair(tb testing.TB) (*clientConn, *net.UDPConn) {
	tb.Helper()

	server, err := net.ListenUDP("udp", nil)
	if err != nil {
		tb.Fatalf("listen: %v", err)
	}

	client, err := net.DialUDP("udp", nil, server.LocalAddr().(*net.UDPAddr))
	if err != nil {
		tb.Fatalf("dial: %v", err)
	}

	// Keep writes bounded; reads are interrupted by closing the connection.
	ad := NewClientConn(client, time.Second)
	return ad.(*clientConn), server
}

func TestWriteReadHappy(t *testing.T) {
	ad, srv := newPair(t)
	defer func(ad *clientConn) {
		_ = ad.Close()
	}(ad)
	defer func(srv *net.UDPConn) {
		_ = srv.Close()
	}(srv)

	msg := []byte("hello")

	// write to server
	if n, err := ad.Write(msg); err != nil || n != len(msg) {
		t.Fatalf("Write = (%d,%v), want (%d,nil)", n, err, len(msg))
	}

	// server receives the packet
	buf := make([]byte, 10)
	_ = srv.SetReadDeadline(time.Now().Add(time.Second))
	if n, _, err := srv.ReadFromUDP(buf); err != nil || string(buf[:n]) != "hello" {
		t.Fatalf("server got (%q,%v)", buf[:n], err)
	}

	// echo back to client
	go func() {
		_, _ = srv.WriteToUDP(msg, ad.conn.LocalAddr().(*net.UDPAddr))
	}()

	readBuf := make([]byte, 10)
	if n, err := ad.Read(readBuf); err != nil || string(readBuf[:n]) != "hello" {
		t.Fatalf("Read = (%q,%v)", readBuf[:n], err)
	}
}

func TestReadShortBuffer(t *testing.T) {
	ad, srv := newPair(t)
	defer func(ad *clientConn) {
		_ = ad.Close()
	}(ad)
	defer func(srv *net.UDPConn) {
		_ = srv.Close()
	}(srv)

	_, _ = srv.WriteToUDP([]byte("oversize"), ad.conn.LocalAddr().(*net.UDPAddr))

	tiny := make([]byte, 1)
	if n, err := ad.Read(tiny); !errors.Is(err, io.ErrShortBuffer) || n != 0 {
		t.Fatalf("want io.ErrShortBuffer, got (%d,%v)", n, err)
	}
}

func TestWriteAfterClose(t *testing.T) {
	ad, _ := newPair(t)
	_ = ad.Close()

	if _, err := ad.Write([]byte("x")); err == nil {
		t.Fatalf("expected error after Close")
	}
}

func TestReadFastPath(t *testing.T) {
	ad, srv := newPair(t)
	defer func() { _ = ad.Close() }()
	defer func() { _ = srv.Close() }()

	msg := []byte("fast path")
	if _, err := srv.WriteToUDP(msg, ad.conn.LocalAddr().(*net.UDPAddr)); err != nil {
		t.Fatalf("server write: %v", err)
	}

	buffer := make([]byte, len(ad.buf))
	if n, err := ad.Read(buffer); err != nil || string(buffer[:n]) != string(msg) {
		t.Fatalf("Read = (%q, %v), want (%q, nil)", buffer[:n], err, msg)
	}
}

func TestReadAfterClose(t *testing.T) {
	ad, srv := newPair(t)
	defer func() { _ = srv.Close() }()
	if err := ad.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if _, err := ad.Read(make([]byte, 1)); err == nil {
		t.Fatal("expected read error after close")
	}
}

func TestMain(m *testing.M) {
	// small pause to avoid flakiness on slow CI runners
	time.Sleep(10 * time.Millisecond)
	os.Exit(m.Run())
}
