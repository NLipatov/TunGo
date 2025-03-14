package network

import (
	"bytes"
	"os"
	"testing"
)

func TestLinuxTunAdapter_ReadWrite(t *testing.T) {
	// Create a pipe to simulate a tun device.
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("Error creating pipe: %v", err)
	}
	defer func(pr *os.File) {
		_ = pr.Close()
	}(pr)
	defer func(pw *os.File) {
		_ = pw.Close()
	}(pw)

	// Use the write end for the adapter.
	adapter := LinuxTunAdapter{TunFile: pw}
	data := []byte("tun test")

	// Test Write.
	n, err := adapter.Write(data)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(data) {
		t.Errorf("Write: expected %d bytes, got %d", len(data), n)
	}

	// Read from the read end.
	buf := make([]byte, len(data))
	n, err = pr.Read(buf)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if !bytes.Equal(buf[:n], data) {
		t.Errorf("Read: expected %q, got %q", data, buf[:n])
	}
}

func TestLinuxTunAdapter_Close(t *testing.T) {
	// Create a pipe to simulate a tun device.
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("Error creating pipe: %v", err)
	}
	// Do not defer close for pw, since we want to test adapter.Close.
	adapter := LinuxTunAdapter{TunFile: pw}

	// Close the adapter.
	if err := adapter.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	// Attempt to write after closing.
	data := []byte("data")
	_, err = adapter.Write(data)
	if err == nil {
		t.Fatalf("Expected an error when writing to a closed file, got nil")
	}

	// Clean up read end.
	_ = pr.Close()
}
