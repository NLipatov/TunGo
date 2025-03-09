package tcp_chacha20

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"tungo/network"
	"tungo/settings"
)

// tcpRouterTestFakeTun implements the TunAdapter interface for TCP tests.
type tcpRouterTestFakeTun struct {
	readData    []byte
	readErr     error
	written     [][]byte
	closeCalled bool
	mu          sync.Mutex
}

func (f *tcpRouterTestFakeTun) Read(p []byte) (int, error) {
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

func (f *tcpRouterTestFakeTun) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.written = append(f.written, p)
	return len(p), nil
}

func (f *tcpRouterTestFakeTun) Close() error {
	f.closeCalled = true
	return nil
}

// tcpRouterTestFakeTunConfigurator implements the TunConfigurator interface.
type tcpRouterTestFakeTunConfigurator struct {
	tun          network.TunAdapter
	deconfigured bool
}

func (f *tcpRouterTestFakeTunConfigurator) Configure(_ settings.ConnectionSettings) (network.TunAdapter, error) {
	return f.tun, nil
}

func (f *tcpRouterTestFakeTunConfigurator) Deconfigure(_ settings.ConnectionSettings) {
	f.deconfigured = true
}

// For testing secure connection, we simulate failure by letting the secure connection
// establishment (which is based on a short DialTimeoutMs) fail.
func TestTCPRouter_RouteTraffic_ContextCancelled(t *testing.T) {
	// Create fake TUN and configurator.
	ftun := &tcpRouterTestFakeTun{
		readData: []byte("test packet"),
	}
	ftc := &tcpRouterTestFakeTunConfigurator{
		tun: ftun,
	}
	// Use settings with a short DialTimeoutMs.
	sett := settings.ConnectionSettings{
		ConnectionIP:  "127.0.0.1",
		DialTimeoutMs: 10,
	}
	router := &TCPRouter{
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
		t.Error("expected tun configurator to be deconfigured")
	}
	if !ftun.closeCalled {
		t.Error("expected tun to be closed")
	}
}

// TestTCPRouter_RouteTraffic_EstablishFailure tests that the TCPRouter cleans up properly when secure
// connection establishment fails. Instead of using a timeout context, we cancel the context manually.
func TestTCPRouter_RouteTraffic_EstablishFailure(t *testing.T) {
	// Create fake TUN and configurator.
	ftun := &tcpRouterTestFakeTun{
		readData: []byte("dummy"),
	}
	ftc := &tcpRouterTestFakeTunConfigurator{
		tun: ftun,
	}
	// Use settings with a short DialTimeoutMs so that secure connection quickly fails.
	sett := settings.ConnectionSettings{
		ConnectionIP:  "127.0.0.1",
		DialTimeoutMs: 10,
	}
	router := &TCPRouter{
		Settings:        sett,
		TunConfigurator: ftc,
	}

	// Use a context that we cancel manually after a short delay.
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err := router.RouteTraffic(ctx)
	if err != nil {
		t.Errorf("expected no error on context cancellation, got %v", err)
	}
	if !ftc.deconfigured {
		t.Error("expected tun configurator to be deconfigured")
	}
	if !ftun.closeCalled {
		t.Error("expected tun to be closed")
	}
}
