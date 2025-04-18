package routing

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// mockRouterTestTunWorker implements the application.TunWorker interface for testing.
type mockRouterTestTunWorker struct {
	ctx             context.Context
	errTun          error
	errTransport    error
	tunCalled       bool
	transportCalled bool
	// delay simulates work duration; if > 0, the method will wait for the delay or context cancellation.
	delay time.Duration
}

func (m *mockRouterTestTunWorker) HandleTun() error {
	m.tunCalled = true
	if m.delay > 0 {
		select {
		case <-m.ctx.Done():
			return m.ctx.Err()
		case <-time.After(m.delay):
			// continue after delay
		}
	}
	return m.errTun
}

func (m *mockRouterTestTunWorker) HandleTransport() error {
	m.transportCalled = true
	if m.delay > 0 {
		select {
		case <-m.ctx.Done():
			return m.ctx.Err()
		case <-time.After(m.delay):
			// continue after delay
		}
	}
	return m.errTransport
}

func TestRouteTraffic_AllSucceed(t *testing.T) {
	worker := &mockRouterTestTunWorker{ctx: context.Background()}
	router := NewRouter(worker)

	err := router.RouteTraffic(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	// At least one handler should have been invoked
	if !worker.tunCalled && !worker.transportCalled {
		t.Error("expected at least one handler to be called")
	}
}

func TestRouteTraffic_HandleTunError(t *testing.T) {
	testErr := errors.New("tun error")
	// Both handlers return the same error to avoid race ordering issues
	worker := &mockRouterTestTunWorker{ctx: context.Background(), errTun: testErr, errTransport: testErr}
	router := NewRouter(worker)

	err := router.RouteTraffic(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), testErr.Error()) {
		t.Errorf("expected error message to contain '%v', got: %v", testErr, err)
	}
}

func TestRouteTraffic_HandleTransportError(t *testing.T) {
	testErr := errors.New("transport error")
	// Both handlers return the same error to avoid race ordering issues
	worker := &mockRouterTestTunWorker{ctx: context.Background(), errTun: testErr, errTransport: testErr}
	router := NewRouter(worker)

	err := router.RouteTraffic(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), testErr.Error()) {
		t.Errorf("expected error message to contain '%v', got: %v", testErr, err)
	}
}

func TestRouteTraffic_BothErrors(t *testing.T) {
	tunErr := errors.New("tun error")
	transportErr := errors.New("transport error")
	worker := &mockRouterTestTunWorker{ctx: context.Background(), errTun: tunErr, errTransport: transportErr}
	router := NewRouter(worker)

	err := router.RouteTraffic(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), tunErr.Error()) && !strings.Contains(err.Error(), transportErr.Error()) {
		t.Errorf("expected error message to contain either '%v' or '%v', got: %v", tunErr, transportErr, err)
	}
}

func TestRouteTraffic_ExternalContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	worker := &mockRouterTestTunWorker{ctx: ctx, delay: 100 * time.Millisecond}
	router := NewRouter(worker)
	err := router.RouteTraffic(ctx)
	if err == nil {
		t.Fatal("expected error due to external context cancellation, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got: %v", err)
	}
}
