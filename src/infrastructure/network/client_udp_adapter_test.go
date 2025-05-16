package network

import (
	"io"
	"net"
	"testing"
	"time"
)

// helper: returns adapter + server side of the pair
func newAdapterPair(tb testing.TB) (*ClientUDPAdapter, *net.UDPConn) {
	tb.Helper()

	server, err := net.ListenUDP("udp", nil)
	if err != nil {
		tb.Fatalf("listen: %v", err)
	}

	client, err := net.DialUDP("udp", nil, server.LocalAddr().(*net.UDPAddr))
	if err != nil {
		tb.Fatalf("dial: %v", err)
	}

	ad := NewClientUDPAdapter(client, time.Second, time.Second).(*ClientUDPAdapter)
	return ad, server
}

func TestWriteReadHappyPath(t *testing.T) {
	ad, server := newAdapterPair(t)
	defer func(ad *ClientUDPAdapter) {
		_ = ad.Close()
	}(ad)
	defer func(server *net.UDPConn) {
		_ = server.Close()
	}(server)

	payload := []byte("hello-udp")

	// Write
	if n, err := ad.Write(payload); err != nil || n != len(payload) {
		t.Fatalf("Write() = (%d, %v), want (%d, nil)", n, err, len(payload))
	}

	// Server receives
	recv := make([]byte, len(payload))
	_ = server.SetReadDeadline(time.Now().Add(time.Second))
	if n, _, err := server.ReadFromUDP(recv); err != nil || string(recv[:n]) != string(payload) {
		t.Fatalf("server got (%q, %v), want %q", recv[:n], err, payload)
	}

	// Read
	go func() {
		_, _ = server.WriteToUDP(payload, ad.conn.LocalAddr().(*net.UDPAddr))
	}()

	buf := make([]byte, len(payload))
	if n, err := ad.Read(buf); err != nil || string(buf[:n]) != string(payload) {
		t.Fatalf("Read() = (%q, %v), want %q", buf[:n], err, payload)
	}
}

func TestReadShortBuffer(t *testing.T) {
	ad, server := newAdapterPair(t)
	defer func(ad *ClientUDPAdapter) {
		_ = ad.Close()
	}(ad)
	defer func(server *net.UDPConn) {
		_ = server.Close()
	}(server)

	msg := []byte("oversize")
	go func() {
		_, _ = server.WriteToUDP(msg, ad.conn.LocalAddr().(*net.UDPAddr))
	}()

	small := make([]byte, 1)
	if n, err := ad.Read(small); err != io.ErrShortBuffer || n != 0 {
		t.Fatalf("Read() = (%d, %v), want (0, io.ErrShortBuffer)", n, err)
	}
}

func TestWriteAfterClose(t *testing.T) {
	ad, _ := newAdapterPair(t)
	_ = ad.Close()

	if _, err := ad.Write([]byte("x")); err == nil {
		t.Fatalf("expected error after Close")
	}
}

func TestReadTimeout(t *testing.T) {
	ad, _ := newAdapterPair(t)
	defer func(ad *ClientUDPAdapter) {
		_ = ad.Close()
	}(ad)

	ad.readDeadline = 10 * time.Millisecond
	start := time.Now()
	if _, err := ad.Read(make([]byte, 1)); err == nil {
		t.Fatalf("expected timeout error")
	}
	if time.Since(start) < ad.readDeadline {
		t.Fatalf("deadline not enforced")
	}
}
