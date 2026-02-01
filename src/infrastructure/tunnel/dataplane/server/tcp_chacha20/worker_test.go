package tcp_chacha20

import (
	"errors"
	"testing"
)

type workerMockTunHandler struct{ err error }

func (m *workerMockTunHandler) HandleTun() error { return m.err }

type workerMockTransportHandler struct{ err error }

func (m *workerMockTransportHandler) HandleTransport() error { return m.err }

func TestNewTcpTunWorker(t *testing.T) {
	w := NewTcpTunWorker(&workerMockTunHandler{}, &workerMockTransportHandler{})
	if w == nil {
		t.Fatal("expected non-nil worker")
	}
}

func TestTcpTunWorker_HandleTun_DelegatesError(t *testing.T) {
	tunErr := errors.New("tun error")
	w := NewTcpTunWorker(&workerMockTunHandler{err: tunErr}, &workerMockTransportHandler{})
	if err := w.HandleTun(); !errors.Is(err, tunErr) {
		t.Fatalf("expected tun error, got %v", err)
	}
}

func TestTcpTunWorker_HandleTransport_DelegatesError(t *testing.T) {
	trErr := errors.New("transport error")
	w := NewTcpTunWorker(&workerMockTunHandler{}, &workerMockTransportHandler{err: trErr})
	if err := w.HandleTransport(); !errors.Is(err, trErr) {
		t.Fatalf("expected transport error, got %v", err)
	}
}

func TestTcpTunWorker_HandleTun_NilError(t *testing.T) {
	w := NewTcpTunWorker(&workerMockTunHandler{}, &workerMockTransportHandler{})
	if err := w.HandleTun(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestTcpTunWorker_HandleTransport_NilError(t *testing.T) {
	w := NewTcpTunWorker(&workerMockTunHandler{}, &workerMockTransportHandler{})
	if err := w.HandleTransport(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}
