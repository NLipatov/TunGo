package udp_chacha20

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
	"tungo/application"
	"tungo/settings"
)

// routerTestFakeTun implements the TunAdapter interface.
type routerTestFakeTun struct {
	readData    []byte
	readErr     error
	written     [][]byte
	closeCalled bool
	mu          sync.Mutex
}

func (f *routerTestFakeTun) Read(p []byte) (int, error) {
	if f.readErr != nil {
		return 0, f.readErr
	}
	// Return preset data once, then return an error to break the loop.
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.readData) == 0 {
		return 0, errors.New("no data")
	}
	n := copy(p, f.readData)
	// Clear data so that the next call results in an error.
	f.readData = nil
	return n, nil
}

func (f *routerTestFakeTun) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.written = append(f.written, p)
	return len(p), nil
}

func (f *routerTestFakeTun) Close() error {
	f.closeCalled = true
	return nil
}

// routerTestFakeTunConfigurator implements the TunDeviceConfigurator interface.
type routerTestFakeTunConfigurator struct {
	tun          application.TunDevice
	deconfigured bool
}

func (f *routerTestFakeTunConfigurator) Configure(_ settings.ConnectionSettings) (application.TunDevice, error) {
	return f.tun, nil
}

func (f *routerTestFakeTunConfigurator) Dispose(_ settings.ConnectionSettings) {
	f.deconfigured = true
}

// TestUDPRouter_RouteTraffic_ContextCancelled tests that RouteTraffic gracefully terminates when context is cancelled.
func TestUDPRouter_RouteTraffic_ContextCancelled(t *testing.T) {
	// Create fake TUN and configurator.
	ftun := &routerTestFakeTun{
		readData: []byte("test packet"),
	}
	ftc := &routerTestFakeTunConfigurator{
		tun: ftun,
	}
	// Use settings with a short timeout.
	sett := settings.ConnectionSettings{
		ConnectionIP:  "127.0.0.1",
		DialTimeoutMs: 10,
	}
	router := &UDPRouter{
		Settings:        sett,
		TunConfigurator: ftc,
	}

	// Start RouteTraffic with a cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := router.RouteTraffic(ctx)
	if err != nil {
		t.Errorf("expected no error on context cancellation, got %v", err)
	}
	if !ftc.deconfigured {
		t.Error("expected Tun configurator to be deconfigured")
	}
	if !ftun.closeCalled {
		t.Error("expected Tun to be closed")
	}
}

// TestUDPRouter_RouteTraffic_EstablishFailure tests that RouteTraffic terminates properly when secure connection establishment fails.
func TestUDPRouter_RouteTraffic_EstablishFailure(t *testing.T) {
	// Create fake TUN and configurator.
	ftun := &routerTestFakeTun{
		readData: []byte("dummy"),
	}
	ftc := &routerTestFakeTunConfigurator{
		tun: ftun,
	}
	// Use settings with a short DialTimeoutMs.
	sett := settings.ConnectionSettings{
		ConnectionIP:  "127.0.0.1",
		DialTimeoutMs: 10,
	}
	router := &UDPRouter{
		Settings:        sett,
		TunConfigurator: ftc,
	}

	// Instead of a timeout context, use a cancelable context and cancel manually.
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel the context after a short delay.
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err := router.RouteTraffic(ctx)
	if err != nil {
		t.Errorf("expected no error on context cancellation, got %v", err)
	}
	if !ftc.deconfigured {
		t.Error("expected Tun configurator to be deconfigured")
	}
	if !ftun.closeCalled {
		t.Error("expected Tun to be closed")
	}
}
