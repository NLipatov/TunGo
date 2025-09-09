package application

import (
	"errors"
	"testing"
	"time"
)

type mockProber struct {
	threshold int
	lastSize  int
	sendErr   error
}

func (m *mockProber) SendProbe(size int) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.lastSize = size
	return nil
}

func (m *mockProber) AwaitAck(timeout time.Duration) (bool, time.Duration, error) {
	if m.lastSize <= m.threshold {
		return true, time.Millisecond, nil
	}
	return false, time.Millisecond, nil
}

func TestDiscoverMTU(t *testing.T) {
	m := &mockProber{threshold: 1450}
	mtu, err := DiscoverMTU(m, 1200, 1500, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mtu != 1450 {
		t.Fatalf("expected 1450, got %d", mtu)
	}
}

func TestDiscoverMTUSendError(t *testing.T) {
	m := &mockProber{threshold: 1450, sendErr: errors.New("fail")}
	mtu, err := DiscoverMTU(m, 1200, 1500, time.Millisecond)
	if err == nil {
		t.Fatal("expected error")
	}
	if mtu != 1200 {
		t.Fatalf("expected fallback to min, got %d", mtu)
	}
}
