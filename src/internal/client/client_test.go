package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	clientconfig "tungo/internal/config/client"
	"tungo/internal/config/settings"
	"tungo/internal/protocol/keys"
)

type clientTestTUN struct {
	closeCalls atomic.Int32
	closeErr   error
}

func TestNewClient(t *testing.T) {
	configurations := clientconfig.NewConfigurations(t.TempDir())
	client, err := New(configurations)
	if err != nil {
		t.Fatal(err)
	}
	if client.configurations != configurations || client.tunManager != nil || client.Ready() {
		t.Fatalf("unexpected new client state: %+v", client)
	}
}

func (*clientTestTUN) Open(netip.Addr) (io.ReadWriter, error) {
	return nil, nil
}

func (m *clientTestTUN) Close() error {
	m.closeCalls.Add(1)
	return m.closeErr
}

func TestRunTunnelLogsPreflightCleanupFailure(t *testing.T) {
	cleanupErr := errors.New("cleanup failed")
	tunnel := &clientTestTUN{closeErr: cleanupErr}
	client := newTestClient(t)
	client.tunManager = tunnel

	var logs bytes.Buffer
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(originalLogger) })

	for attempt := 1; attempt <= 2; attempt++ {
		if err := client.runTunnel(t.Context()); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("runTunnel() error = %v, want missing configuration", err)
		}
		if client.tunManager != tunnel {
			t.Fatal("failed cleanup discarded the manager before it could be retried")
		}
		if got := tunnel.closeCalls.Load(); got != int32(attempt) {
			t.Fatalf("Close() calls = %d, want %d", got, attempt)
		}
	}
	if !strings.Contains(logs.String(), "failed to clean stale tunnel state") ||
		!strings.Contains(logs.String(), cleanupErr.Error()) {
		t.Fatalf("preflight cleanup log = %q", logs.String())
	}
}

func TestRunTunnelClearsPreviousSessionState(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ready bool
	}{
		{name: "before first ready"},
		{name: "after first ready", ready: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tunnel := &clientTestTUN{}
			client := newTestClient(t)
			client.tunManager = tunnel
			if tc.ready {
				client.sessionReadyAt = time.Now().Add(-6 * time.Minute)
			}
			client.ready.Store(tc.ready)
			for range 2 {
				if err := client.runTunnel(t.Context()); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("runTunnel() error = %v, want missing configuration", err)
				}
				if client.tunManager != nil || !client.sessionReadyAt.IsZero() {
					t.Fatal("previous session state was retained")
				}
				if client.Ready() != tc.ready {
					t.Fatalf("Ready() = %v, want %v", client.Ready(), tc.ready)
				}
			}
			if got := tunnel.closeCalls.Load(); got != 1 {
				t.Fatalf("Close() calls = %d, want 1", got)
			}
		})
	}
}

type clientTestReconnectHandler struct {
	slog.Handler
	reconnecting chan struct{}
}

func (h clientTestReconnectHandler) Handle(ctx context.Context, record slog.Record) error {
	if record.Message == "session error, reconnecting" {
		select {
		case h.reconnecting <- struct{}{}:
		default:
		}
	}
	return h.Handler.Handle(ctx, record)
}

func TestClientStopsDuringReconnectDelay(t *testing.T) {
	tunnel := &clientTestTUN{}
	client := newTestClient(t)
	client.tunManager = tunnel
	reconnecting := make(chan struct{}, 1)
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(clientTestReconnectHandler{
		Handler:      slog.NewTextHandler(io.Discard, nil),
		reconnecting: reconnecting,
	}))
	t.Cleanup(func() { slog.SetDefault(originalLogger) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- client.Run(ctx) }()

	select {
	case <-reconnecting:
	case <-time.After(2 * time.Second):
		t.Fatal("client did not reach the reconnect delay")
	}
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(reconnectInterval / 2):
		t.Fatal("client did not stop after cancellation")
	}
	if got := tunnel.closeCalls.Load(); got != 1 {
		t.Fatalf("Close() calls = %d, want preflight cleanup", got)
	}
	// Exactly one session failed, so the next failure is still below the threshold.
	if rotated, name, err := client.failover.OnFail(); err != nil || rotated || name != "" {
		t.Fatalf("second failure = %v, %q, %v; want no rotation", rotated, name, err)
	}
	if rotated, name, err := client.failover.OnFail(); !errors.Is(err, ErrNoFailoverCandidates) || rotated || name != "" {
		t.Fatalf("third failure = %v, %q, %v; want ErrNoFailoverCandidates", rotated, name, err)
	}
}

type clientTestRemoteTransport struct {
	io.ReadWriteCloser
	remote netip.AddrPort
}

func (t clientTestRemoteTransport) RemoteAddrPort() netip.AddrPort {
	return t.remote
}

type clientTestNoRemoteTransport struct{}

func (clientTestNoRemoteTransport) Read([]byte) (int, error)    { return 0, io.EOF }
func (clientTestNoRemoteTransport) Write(p []byte) (int, error) { return len(p), nil }
func (clientTestNoRemoteTransport) Close() error                { return nil }

func TestTransportServerAddr(t *testing.T) {
	transport := clientTestRemoteTransport{remote: netip.MustParseAddrPort("[::ffff:192.0.2.10]:443")}

	got, err := transportServerAddr(transport)
	if err != nil {
		t.Fatalf("transportServerAddr() error = %v", err)
	}
	if want := netip.MustParseAddr("192.0.2.10"); got != want {
		t.Fatalf("transportServerAddr() = %s, want %s", got, want)
	}
}

func TestTransportServerAddrRejectsMissingOrInvalidRemoteAddress(t *testing.T) {
	tests := []struct {
		name      string
		transport io.ReadWriteCloser
	}{
		{name: "missing provider", transport: clientTestNoRemoteTransport{}},
		{name: "invalid address", transport: clientTestRemoteTransport{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := transportServerAddr(tt.transport); err == nil {
				t.Fatal("transportServerAddr() error = nil")
			}
		})
	}
}

func TestAllowedSources(t *testing.T) {
	tests := []struct {
		name string
		s    settings.Settings
		want []netip.Addr
	}{
		{name: "empty"},
		{
			name: "dual stack with mapped IPv4",
			s: settings.Settings{Network: settings.Network{
				IPv4: netip.MustParseAddr("::ffff:192.0.2.1"),
				IPv6: netip.MustParseAddr("2001:db8::1"),
			}},
			want: []netip.Addr{
				netip.MustParseAddr("192.0.2.1"),
				netip.MustParseAddr("2001:db8::1"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := allowedSources(tt.s)
			if len(got) != len(tt.want) {
				t.Fatalf("allowedSources() length = %d, want %d: %v", len(got), len(tt.want), got)
			}
			for _, addr := range tt.want {
				if _, ok := got[addr]; !ok {
					t.Errorf("allowedSources() does not contain %s: %v", addr, got)
				}
			}
		})
	}
}

func TestClientWithCanceledContextDoesNotStartSession(t *testing.T) {
	tunnel := &clientTestTUN{}
	client := &Client{tunManager: tunnel}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := client.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := tunnel.closeCalls.Load(); got != 0 {
		t.Fatalf("Close() calls = %d, want no session cleanup", got)
	}
}

func TestRunSessionReturnsConfigurationError(t *testing.T) {
	tunnel := &clientTestTUN{}
	client := newTestClient(t)
	client.tunManager = tunnel

	err := client.runSession(t.Context())
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runSession() error = %v, want missing configuration", err)
	}
	if got := tunnel.closeCalls.Load(); got != 1 {
		t.Fatalf("Close() calls = %d, want 1", got)
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

	client := newTestClient(t)
	configuration := clientTestConfiguration()
	configuration.TCPSettings.Port = listener.Addr().(*net.TCPAddr).Port
	configuration.ClientPublicKey = clientPublicKey
	configuration.ClientPrivateKey = clientPrivateKey[:]
	configuration.X25519PublicKey = serverPublicKey
	importClientConfiguration(t, client.configurations, "server", configuration)
	if err := client.configurations.Activate("server"); err != nil {
		t.Fatal(err)
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
	if client.Ready() || !client.sessionReadyAt.IsZero() || client.tunManager != nil {
		t.Fatal("an interrupted handshake marked the session ready or created a TUN")
	}
}

func newTestClient(t *testing.T) *Client {
	t.Helper()
	client, err := New(clientconfig.NewConfigurations(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func clientTestConfiguration() clientconfig.Configuration {
	return clientconfig.Configuration{
		ClientID: 1,
		Protocol: settings.TCP,
		TCPSettings: settings.Settings{
			Network: settings.Network{
				TunName:    "tun0",
				IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
				Server:     settings.Host{IPv4: "127.0.0.1"},
				Port:       8080,
			},
			MTU:      settings.DefaultMTU,
			Protocol: settings.TCP,
		},
		X25519PublicKey:  make([]byte, 32),
		ClientPublicKey:  make([]byte, 32),
		ClientPrivateKey: make([]byte, 32),
	}
}

func importClientConfiguration(t *testing.T, configurations *clientconfig.Configurations, name string, configuration clientconfig.Configuration) {
	t.Helper()
	data, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	if err := configurations.Import(name, string(data)); err != nil {
		t.Fatal(err)
	}
}
