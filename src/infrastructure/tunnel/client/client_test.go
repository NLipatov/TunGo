package client

import (
	"context"
	"io"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	clientConfiguration "tungo/application/configuration/client"
	"tungo/application/configuration/settings"
)

type clientTestTunManager struct {
	disposeCalls atomic.Int32
}

func (*clientTestTunManager) CreateDevice() (io.ReadWriteCloser, error) {
	return nil, nil
}

func (m *clientTestTunManager) DisposeDevices() error {
	m.disposeCalls.Add(1)
	return nil
}

func (*clientTestTunManager) SetRouteEndpoint(netip.AddrPort) {}

func TestClientStopsDuringReconnectDelay(t *testing.T) {
	manager := &clientTestTunManager{}
	client := &Client{
		configuration: &clientConfiguration.Configuration{Protocol: settings.UNKNOWN},
		tunManager:    manager,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- client.Run(ctx) }()

	deadline := time.After(2 * time.Second)
	for manager.disposeCalls.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("client did not start a session")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("client did not stop after cancellation")
	}
	if got := manager.disposeCalls.Load(); got != 2 {
		t.Fatalf("DisposeDevices() calls = %d, want before attempt and on exit", got)
	}
}

func TestClientWithCanceledContextOnlyCleansUp(t *testing.T) {
	manager := &clientTestTunManager{}
	client := &Client{
		configuration: &clientConfiguration.Configuration{Protocol: settings.UNKNOWN},
		tunManager:    manager,
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := client.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := manager.disposeCalls.Load(); got != 1 {
		t.Fatalf("DisposeDevices() calls = %d, want final cleanup", got)
	}
}
