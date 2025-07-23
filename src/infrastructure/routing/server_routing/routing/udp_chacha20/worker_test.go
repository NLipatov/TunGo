package udp_chacha20

import (
	"errors"
	"testing"
)

type mockTunHandler struct {
	called bool
	err    error
}

func (m *mockTunHandler) HandleTun() error {
	m.called = true
	return m.err
}

type mockTransportHandler struct {
	called bool
	err    error
}

func (m *mockTransportHandler) HandleTransport() error {
	m.called = true
	return m.err
}

func TestUdpTunWorker_HandleTun(t *testing.T) {
	tun := &mockTunHandler{err: errors.New("tun")}
	trans := &mockTransportHandler{}
	worker := NewUdpTunWorker(tun, trans)

	err := worker.HandleTun()
	if !tun.called {
		t.Fatalf("HandleTun was not called on tunHandler")
	}
	if err == nil || err.Error() != "tun" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUdpTunWorker_HandleTransport(t *testing.T) {
	tun := &mockTunHandler{}
	trans := &mockTransportHandler{err: errors.New("transport")}
	worker := NewUdpTunWorker(tun, trans)

	err := worker.HandleTransport()
	if !trans.called {
		t.Fatalf("HandleTransport was not called on transportHandler")
	}
	if err == nil || err.Error() != "transport" {
		t.Fatalf("unexpected error: %v", err)
	}
}
