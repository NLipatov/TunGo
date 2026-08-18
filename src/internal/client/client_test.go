package client

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	clientconfig "tungo/internal/config/client"
	"tungo/internal/config/settings"
	"tungo/internal/protocol/keys"
)

type clientTestTunManager struct {
	disposeCalls atomic.Int32
}

func (*clientTestTunManager) OpenTunnel() (io.ReadWriteCloser, error) {
	return nil, nil
}

func (m *clientTestTunManager) CloseTunnel() error {
	m.disposeCalls.Add(1)
	return nil
}

func (*clientTestTunManager) SetRouteEndpoint(netip.AddrPort) {}

func TestClientStopsDuringReconnectDelay(t *testing.T) {
	manager := &clientTestTunManager{}
	client := &Client{
		configuration: &clientconfig.Configuration{Protocol: settings.UNKNOWN},
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
		t.Fatalf("CloseTunnel() calls = %d, want before attempt and on exit", got)
	}
}

func TestClientWithCanceledContextOnlyCleansUp(t *testing.T) {
	manager := &clientTestTunManager{}
	client := &Client{
		configuration: &clientconfig.Configuration{Protocol: settings.UNKNOWN},
		tunManager:    manager,
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := client.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := manager.disposeCalls.Load(); got != 1 {
		t.Fatalf("CloseTunnel() calls = %d, want final cleanup", got)
	}
}

func TestRunSessionReturnsForwardError(t *testing.T) {
	manager := &clientTestTunManager{}
	client := &Client{
		configuration: &clientconfig.Configuration{Protocol: settings.UNKNOWN},
		tunManager:    manager,
	}

	err := client.runSession(t.Context())
	if err == nil || !strings.Contains(err.Error(), "unsupported protocol") {
		t.Fatalf("runSession() error = %v, want unsupported protocol", err)
	}
	if got := manager.disposeCalls.Load(); got != 1 {
		t.Fatalf("CloseTunnel() calls = %d, want 1", got)
	}
}

func TestRunSessionCancellationStopsForward(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	type acceptResult struct {
		conn net.Conn
		err  error
	}
	accepted := make(chan acceptResult, 1)
	go func() {
		conn, err := listener.Accept()
		accepted <- acceptResult{conn: conn, err: err}
	}()

	deriver := &keys.DefaultKeyDeriver{}
	serverPublicKey, _, err := deriver.GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}
	clientPublicKey, clientPrivateKey, err := deriver.GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}

	manager := &clientTestTunManager{}
	client := &Client{
		configuration: &clientconfig.Configuration{
			ClientID:         1,
			Protocol:         settings.TCP,
			TCPSettings:      mkTCPSettings(listener.Addr().(*net.TCPAddr).Port),
			ClientPublicKey:  clientPublicKey,
			ClientPrivateKey: clientPrivateKey[:],
			X25519PublicKey:  serverPublicKey,
		},
		tunManager: manager,
	}

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- client.runSession(ctx) }()

	var serverConn net.Conn
	select {
	case result := <-accepted:
		if result.err != nil {
			t.Fatalf("accept: %v", result.err)
		}
		serverConn = result.conn
		t.Cleanup(func() { _ = serverConn.Close() })
	case err := <-done:
		t.Fatalf("runSession() returned before connecting: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("runSession() did not connect")
	}

	connectionClosed := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, serverConn)
		close(connectionClosed)
	}()

	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("runSession() error = %v, want context cancellation", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runSession() did not stop after cancellation")
	}

	select {
	case <-connectionClosed:
	case <-time.After(time.Second):
		t.Fatal("connection remained open after cancellation")
	}
}
