package tcp_chacha20

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

func TestTcpTunWorker_HandleTun(t *testing.T) {
	tun := &mockTunHandler{err: errors.New("tun error")}
	trans := &mockTransportHandler{}
	worker := NewTcpTunWorker(tun, trans)

	err := worker.HandleTun()
	if !tun.called {
		t.Fatalf("HandleTun was not called on tunHandler")
	}
	if err == nil || err.Error() != "tun error" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTcpTunWorker_HandleTransport(t *testing.T) {
	tun := &mockTunHandler{}
	trans := &mockTransportHandler{err: errors.New("transport error")}
	worker := NewTcpTunWorker(tun, trans)

	err := worker.HandleTransport()
	if !trans.called {
		t.Fatalf("HandleTransport was not called on transportHandler")
	}
	if err == nil || err.Error() != "transport error" {
		t.Fatalf("unexpected error: %v", err)
	}
}
