package adapters

import (
	"errors"
	"io"
	"net"
	"os"
	"testing"
	"time"
	"tungo/infrastructure/network"
	"tungo/infrastructure/settings"
)

// helper: returns client-side adapter and matching UDP server socket
func newPair(tb testing.TB) (*ClientUDPAdapter, *net.UDPConn) {
	tb.Helper()

	server, err := net.ListenUDP("udp", nil)
	if err != nil {
		tb.Fatalf("listen: %v", err)
	}

	client, err := net.DialUDP("udp", nil, server.LocalAddr().(*net.UDPAddr))
	if err != nil {
		tb.Fatalf("dial: %v", err)
	}

	// 1-second deadlines for tests
	ad := NewClientUDPAdapter(client, network.Timeout(time.Second), network.Timeout(time.Second), settings.DefaultEthernetMTU)
	return ad.(*ClientUDPAdapter), server
}

func TestWriteReadHappy(t *testing.T) {
	ad, srv := newPair(t)
	defer func(ad *ClientUDPAdapter) {
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
	defer func(ad *ClientUDPAdapter) {
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

func TestReadTimeout(t *testing.T) {
	ad, _ := newPair(t)
	defer func(ad *ClientUDPAdapter) {
		_ = ad.Close()
	}(ad)

	ad.readDeadline = network.Timeout(5 * time.Millisecond)

	start := time.Now()
	if _, err := ad.Read(make([]byte, 1)); err == nil {
		t.Fatalf("expected timeout")
	}
	if time.Since(start) < 5*time.Millisecond {
		t.Fatalf("deadline not respected")
	}
}

func TestMain(m *testing.M) {
	// small pause to avoid flakiness on slow CI runners
	time.Sleep(10 * time.Millisecond)
	os.Exit(m.Run())
}
