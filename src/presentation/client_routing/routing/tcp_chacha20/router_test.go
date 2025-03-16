package tcp_chacha20

import (
	"context"
	"math"
	"math/rand"
	"os"
	"path"
	"strconv"
	"testing"
	"time"
	"tungo/settings"
)

// For testing secure connection, we simulate failure by letting the secure connection
// establishment (which is based on a short DialTimeoutMs) fail.
func TestTCPRouter_RouteTraffic_ContextCancelled(t *testing.T) {
	tun, tunErr := os.Create(path.Join(t.TempDir(), strconv.Itoa(rand.Intn(math.MaxInt32))))
	if tunErr != nil {
		t.Fatal(tunErr)
	}
	defer func(tun *os.File) {
		_ = tun.Close()
	}(tun)

	// Use settings with a short DialTimeoutMs.
	sett := settings.ConnectionSettings{
		ConnectionIP:  "127.0.0.1",
		DialTimeoutMs: 10,
	}
	router := &TCPRouter{
		Settings: sett,
	}

	// Start RouteTraffic with a cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := router.RouteTraffic(ctx)
	if err != nil {
		t.Errorf("expected no error on context cancellation, got %v", err)
	}
}

// TestTCPRouter_RouteTraffic_EstablishFailure tests that the TCPRouter cleans up properly when secure
// connection establishment fails. Instead of using a timeout context, we cancel the context manually.
func TestTCPRouter_RouteTraffic_EstablishFailure(t *testing.T) {
	tun, tunErr := os.Create(path.Join(t.TempDir(), strconv.Itoa(rand.Intn(math.MaxInt32))))
	if tunErr != nil {
		t.Fatal(tunErr)
	}
	defer func(tun *os.File) {
		_ = tun.Close()
	}(tun)

	// Use settings with a short DialTimeoutMs so that secure connection quickly fails.
	sett := settings.ConnectionSettings{
		ConnectionIP:  "127.0.0.1",
		DialTimeoutMs: 10,
	}
	router := &TCPRouter{
		Settings: sett,
		Tun:      tun,
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
}
