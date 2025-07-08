package server_configuration

import (
	"testing"
	"time"
)

type mockReader struct {
	count int
}

func (m *mockReader) read() (*Configuration, error) {
	m.count++
	return &Configuration{}, nil
}

func TestTTLReader_Caching(t *testing.T) {
	mr := &mockReader{}
	r := NewTTLReader(mr, 50*time.Millisecond)

	if _, err := r.read(); err != nil {
		t.Fatalf("first read error: %v", err)
	}
	if mr.count != 1 {
		t.Fatalf("expected 1 underlying read, got %d", mr.count)
	}

	if _, err := r.read(); err != nil {
		t.Fatalf("second read error: %v", err)
	}
	if mr.count != 1 {
		t.Fatalf("expected cached read without underlying call, got %d", mr.count)
	}

	time.Sleep(60 * time.Millisecond)
	if _, err := r.read(); err != nil {
		t.Fatalf("third read error: %v", err)
	}
	if mr.count != 2 {
		t.Fatalf("expected underlying read after TTL expire, got %d", mr.count)
	}
}
