package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
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
	client := newTestClientWithUnavailableServer(t)
	client.tunManager = tunnel

	var logs bytes.Buffer
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(originalLogger) })

	if err := client.runTunnel(t.Context()); err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("runTunnel() error = %v, want connection attempt despite cleanup failure", err)
	}
	if client.tunManager != nil {
		t.Fatal("new tunnel manager was not cleaned up after connection failure")
	}
	if got := tunnel.closeCalls.Load(); got != 1 {
		t.Fatalf("Close() calls = %d, want one best-effort preflight cleanup", got)
	}
	if !strings.Contains(logs.String(), "failed to clean stale tunnel state") ||
		!strings.Contains(logs.String(), cleanupErr.Error()) {
		t.Fatalf("preflight cleanup log = %q", logs.String())
	}
}

func TestRunTunnelClearsPreviousSessionState(t *testing.T) {
	for _, tc := range []struct {
		name    string
		readyAt time.Time
	}{
		{name: "before first session"},
		{name: "after stable session", readyAt: time.Now().Add(-6 * time.Minute)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tunnel := &clientTestTUN{}
			client := newTestClientWithUnavailableServer(t)
			client.tunManager = tunnel
			client.sessionReadyAt = tc.readyAt
			for range 2 {
				if err := client.runTunnel(t.Context()); err == nil || !strings.Contains(err.Error(), "503") {
					t.Fatalf("runTunnel() error = %v, want unavailable server", err)
				}
				if client.tunManager != nil || !client.sessionReadyAt.IsZero() {
					t.Fatal("previous session state was retained")
				}
				if client.Ready() {
					t.Fatal("failed connection marked the session ready")
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
	client := newTestClientWithUnavailableServer(t)
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
	case err := <-done:
		t.Fatalf("Run() returned before reconnecting: %v", err)
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

func TestClientRunReturnsConfigurationError(t *testing.T) {
	for _, tc := range []struct {
		name, content string
	}{
		{name: "missing"},
		{name: "invalid JSON", content: "{"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			if tc.content != "" {
				path := filepath.Join(directory, "client_configuration.json")
				if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			client, err := New(clientconfig.NewConfigurations(directory))
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			err = client.Run(ctx)
			if ctx.Err() != nil {
				t.Fatalf("Run() waited for cancellation instead of returning the configuration error: %v", err)
			}
			if !errors.Is(err, ErrInvalidConfiguration) {
				t.Fatalf("Run() error = %v, want ErrInvalidConfiguration", err)
			}
			if tc.content == "" {
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("Run() error = %v, want missing configuration cause", err)
				}
			} else {
				var syntaxErr *json.SyntaxError
				if !errors.As(err, &syntaxErr) {
					t.Fatalf("Run() error = %v, want JSON syntax cause", err)
				}
			}
		})
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
	client := newTestClient(t)

	err := client.runSession(t.Context())
	if !errors.Is(err, ErrInvalidConfiguration) || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runSession() error = %v, want missing configuration", err)
	}
	if client.tunManager != nil || client.Ready() {
		t.Fatal("invalid configuration started a tunnel session")
	}
}

func TestRunSessionCancellationStopsForward(t *testing.T) {
	stubClientNetworkCommands(t)
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
		t.Fatal("an interrupted handshake marked the session ready or retained the tunnel manager")
	}
}

func stubClientNetworkCommands(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("TUN cleanup uses native Windows APIs that cannot be isolated through PATH")
	}
	directory := t.TempDir()
	// Use only these stubs so cleanup cannot change the host's DNS, routes or firewall.
	t.Setenv("PATH", directory)
	for name, script := range map[string]string{
		"scutil":     "#!/bin/sh\nwhile IFS= read -r line; do :; done\nprintf 'No such key\\n'\n",
		"resolvconf": "#!/bin/sh\nexit 0\n",
		"ip":         "#!/bin/sh\nexit 0\n",
		"iptables":   "#!/bin/sh\nexit 0\n",
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
}

func newTestClientWithUnavailableServer(t *testing.T) *Client {
	t.Helper()
	stubClientNetworkCommands(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)
	client := newTestClient(t)
	configuration := clientTestConfiguration()
	configuration.Protocol = settings.WS
	configuration.WSSettings = configuration.TCPSettings
	configuration.WSSettings.Protocol = settings.WS
	configuration.WSSettings.Port = server.Listener.Addr().(*net.TCPAddr).Port
	importClientConfiguration(t, client.configurations, "server", configuration)
	if err := client.configurations.Activate("server"); err != nil {
		t.Fatal(err)
	}
	return client
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
