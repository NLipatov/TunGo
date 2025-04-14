package client_routing

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// mockRouterTestTunWorker implements the application.TunWorker interface for testing.
type mockRouterTestTunWorker struct {
	errTun          error
	errTransport    error
	tunCalled       bool
	transportCalled bool
	// delay simulates work duration; if > 0, the method will wait for the delay or context cancellation.
	delay time.Duration
}

func (m *mockRouterTestTunWorker) HandleTun(ctx context.Context) error {
	m.tunCalled = true
	if m.delay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(m.delay):
			// continue after delay
		}
	}
	return m.errTun
}

func (m *mockRouterTestTunWorker) HandleTransport(ctx context.Context) error {
	m.transportCalled = true
	if m.delay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(m.delay):
			// continue after delay
		}
	}
	return m.errTransport
}

func TestRouteTraffic_AllSucceed(t *testing.T) {
	worker := &mockRouterTestTunWorker{}
	router := NewRouter(worker)

	err := router.RouteTraffic(context.Background())
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if !worker.tunCalled {
		t.Errorf("expected HandleTun to be called")
	}
	if !worker.transportCalled {
		t.Errorf("expected HandleTransport to be called")
	}
}

func TestRouteTraffic_HandleTunError(t *testing.T) {
	testErr := errors.New("tun error")
	worker := &mockRouterTestTunWorker{
		errTun: testErr,
	}
	router := NewRouter(worker)

	err := router.RouteTraffic(context.Background())
	if err == nil {
		t.Errorf("expected error, got nil")
		return
	}
	if !strings.Contains(err.Error(), testErr.Error()) {
		t.Errorf("expected error message to contain '%v', got: %v", testErr, err)
	}
	if !worker.tunCalled {
		t.Errorf("expected HandleTun to be called")
	}
}

func TestRouteTraffic_HandleTransportError(t *testing.T) {
	testErr := errors.New("transport error")
	worker := &mockRouterTestTunWorker{
		errTransport: testErr,
	}
	router := NewRouter(worker)

	err := router.RouteTraffic(context.Background())
	if err == nil {
		t.Errorf("expected error, got nil")
		return
	}
	if !strings.Contains(err.Error(), testErr.Error()) {
		t.Errorf("expected error message to contain '%v', got: %v", testErr, err)
	}
	if !worker.transportCalled {
		t.Errorf("expected HandleTransport to be called")
	}
}

func TestRouteTraffic_BothErrors(t *testing.T) {
	tunErr := errors.New("tun error")
	transportErr := errors.New("transport error")
	worker := &mockRouterTestTunWorker{
		errTun:       tunErr,
		errTransport: transportErr,
	}
	router := NewRouter(worker)

	err := router.RouteTraffic(context.Background())
	if err == nil {
		t.Errorf("expected error, got nil")
		return
	}
	// Check that the error message contains either of the errors.
	if !strings.Contains(err.Error(), tunErr.Error()) && !strings.Contains(err.Error(), transportErr.Error()) {
		t.Errorf("expected error message to contain either '%v' or '%v', got: %v", tunErr, transportErr, err)
	}
}

func TestRouteTraffic_ExternalContextCancel(t *testing.T) {
	// Simulate external cancellation with delayed worker methods.
	worker := &mockRouterTestTunWorker{
		delay: 100 * time.Millisecond,
	}
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel the context after a short delay.
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	router := NewRouter(worker)
	err := router.RouteTraffic(ctx)
	if err == nil {
		t.Errorf("expected error due to external context cancellation, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got: %v", err)
	}
}
