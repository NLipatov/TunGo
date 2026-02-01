package udp_chacha20

import (
	"errors"
	"testing"
)

// fakeTun implements application.TunHandler
// and allows us to simulate behavior in tests.
type workerTestFakeTun struct {
	called bool
	err    error
}

func (f *workerTestFakeTun) HandleTun() error {
	f.called = true
	return f.err
}

// fakeTransport implements application.TransportHandler
// and allows us to simulate behavior in tests.
type workerTestFakeTransport struct {
	called bool
	err    error
}

func (f *workerTestFakeTransport) HandleTransport() error {
	f.called = true
	return f.err
}

func TestHandleTun_Success(t *testing.T) {
	ft := &workerTestFakeTun{}
	w := NewUdpWorker(nil, ft)
	if err := w.HandleTun(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !ft.called {
		t.Error("expected TunHandler.HandleTun to be called")
	}
}

func TestHandleTun_Error(t *testing.T) {
	want := errors.New("tun error")
	ft := &workerTestFakeTun{err: want}
	w := NewUdpWorker(nil, ft)
	if err := w.HandleTun(); !errors.Is(err, want) {
		t.Errorf("expected error %v, got %v", want, err)
	}
	if !ft.called {
		t.Error("expected TunHandler.HandleTun to be called even on error")
	}
}

func TestHandleTransport_Success(t *testing.T) {
	ft := &workerTestFakeTransport{}
	w := NewUdpWorker(ft, nil)
	if err := w.HandleTransport(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !ft.called {
		t.Error("expected TransportHandler.HandleTransport to be called")
	}
}

func TestHandleTransport_Error(t *testing.T) {
	want := errors.New("transport error")
	ft := &workerTestFakeTransport{err: want}
	w := NewUdpWorker(ft, nil)
	if err := w.HandleTransport(); !errors.Is(err, want) {
		t.Errorf("expected error %v, got %v", want, err)
	}
	if !ft.called {
		t.Error("expected TransportHandler.HandleTransport to be called even on error")
	}
}
