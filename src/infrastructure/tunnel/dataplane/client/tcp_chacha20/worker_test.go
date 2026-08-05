package tcp_chacha20

import (
	"errors"
	"testing"
)

/* ─── mocks ──────────────────────────────────────────────────────────────── */

// TcpTunWorkerTestMockTunHandler implements application.TunHandler
type TcpTunWorkerTestMockTunHandler struct {
	called bool
	err    error
}

func (m *TcpTunWorkerTestMockTunHandler) HandleTun() error {
	m.called = true
	return m.err
}

// TcpTunWorkerTestMockTransportHandler implements application.TransportHandler
type TcpTunWorkerTestMockTransportHandler struct {
	called bool
	err    error
}

func (m *TcpTunWorkerTestMockTransportHandler) HandleTransport() error {
	m.called = true
	return m.err
}

/* ─── tests ──────────────────────────────────────────────────────────────── */

func TestTcpTunWorker_DelegatesSuccessfully(t *testing.T) {
	tun := &TcpTunWorkerTestMockTunHandler{}
	transport := &TcpTunWorkerTestMockTransportHandler{}
	w := NewTcpTunWorker(tun, transport)

	if err := w.HandleTun(); err != nil {
		t.Fatalf("HandleTun returned unexpected error: %v", err)
	}
	if err := w.HandleTransport(); err != nil {
		t.Fatalf("HandleTransport returned unexpected error: %v", err)
	}

	if !tun.called || !transport.called {
		t.Fatalf("expected both handlers to be called (tun=%v, transport=%v)", tun.called, transport.called)
	}
}

func TestTcpTunWorker_ErrorPropagation(t *testing.T) {
	tunErr := errors.New("tun fail")
	trpErr := errors.New("transport fail")

	tun := &TcpTunWorkerTestMockTunHandler{err: tunErr}
	transport := &TcpTunWorkerTestMockTransportHandler{err: trpErr}
	w := NewTcpTunWorker(tun, transport)

	if err := w.HandleTun(); !errors.Is(err, tunErr) {
		t.Fatalf("expected %v, got %v", tunErr, err)
	}
	if err := w.HandleTransport(); !errors.Is(err, trpErr) {
		t.Fatalf("expected %v, got %v", trpErr, err)
	}
}
