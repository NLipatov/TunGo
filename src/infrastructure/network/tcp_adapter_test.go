package network

import (
	"bytes"
	"net"
	"testing"
)

func TestTcpAdapterWrite(t *testing.T) {
	c1, c2 := net.Pipe()
	defer func(c1 net.Conn) {
		_ = c1.Close()
	}(c1)
	defer func(c2 net.Conn) {
		_ = c2.Close()
	}(c2)

	adapter := TcpAdapter{Conn: c1}
	data := []byte("hello")

	// Write data to adapter and read from other side
	done := make(chan bool)
	go func() {
		n, err := adapter.Write(data)
		if err != nil {
			t.Errorf("Write error: %v", err)
		}
		if n != len(data) {
			t.Errorf("Write: expected %d bytes, got %d", len(data), n)
		}
		done <- true
	}()

	buf := make([]byte, len(data))
	n, err := c2.Read(buf)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if n != len(data) || !bytes.Equal(buf, data) {
		t.Errorf("Read: expected %q, got %q", data, buf)
	}

	<-done
}

func TestTcpAdapterRead(t *testing.T) {
	c1, c2 := net.Pipe()
	defer func(c1 net.Conn) {
		_ = c1.Close()
	}(c1)
	defer func(c2 net.Conn) {
		_ = c2.Close()
	}(c2)

	adapter := TcpAdapter{Conn: c1}
	data := []byte("world")

	// Write data to adapter and read from other side
	done := make(chan bool)
	go func() {
		n, err := c2.Write(data)
		if err != nil {
			t.Errorf("Pipe Write error: %v", err)
		}
		if n != len(data) {
			t.Errorf("Pipe Write: expected %d bytes, got %d", len(data), n)
		}
		done <- true
	}()

	buf := make([]byte, len(data))
	n, err := adapter.Read(buf)
	if err != nil {
		t.Fatalf("Adapter Read error: %v", err)
	}
	if n != len(data) || !bytes.Equal(buf, data) {
		t.Errorf("Adapter Read: expected %q, got %q", data, buf)
	}

	<-done
}

func TestTcpAdapterClose(t *testing.T) {
	c1, c2 := net.Pipe()
	defer func(c2 net.Conn) {
		_ = c2.Close()
	}(c2)
	adapter := TcpAdapter{Conn: c1}

	// Close the adapter
	if err := adapter.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	// Subsequent Write should fail
	if n, err := adapter.Write([]byte("x")); err == nil {
		t.Errorf("expected error on Write after Close, got n=%d, err=nil", n)
	}

	// Subsequent Read should fail
	buf := make([]byte, 1)
	if n, err := adapter.Read(buf); err == nil {
		t.Errorf("expected error on Read after Close, got n=%d, err=nil", n)
	}
}
