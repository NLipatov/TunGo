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

func TestNewClient(t *testing.T) {
	if client, err := New(nil); err == nil || client != nil {
		t.Fatalf("New(nil) = %v, %v; want nil and error", client, err)
	}

	configuration := &clientconfig.Configuration{
		ClientID: 1,
		Protocol: settings.TCP,
		TCPSettings: settings.Settings{
			Network: settings.Network{
				TunName:    "tun0",
				IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
				Server:     settings.Host{IPv4: "192.0.2.1"},
				Port:       8080,
			},
			MTU:      settings.DefaultMTU,
			Protocol: settings.TCP,
		},
		X25519PublicKey:  make([]byte, 32),
		ClientPublicKey:  make([]byte, 32),
		ClientPrivateKey: make([]byte, 32),
	}
	client, err := New(configuration)
	if err != nil {
		t.Fatal(err)
	}
	if client.configuration != configuration || client.Ready() {
		t.Fatalf("unexpected new client state: %+v", client)
	}

	configuration.Protocol = settings.UNKNOWN
	if client, err := New(configuration); err == nil || client != nil {
		t.Fatalf("New(invalid configuration) = %v, %v; want nil and error", client, err)
	}
}

func (*clientTestTunManager) OpenTunnel(netip.Addr) (io.ReadWriter, error) {
	return nil, nil
}

func (m *clientTestTunManager) CloseTunnel() error {
	m.disposeCalls.Add(1)
	return nil
}

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
	if got := manager.disposeCalls.Load(); got != 1 {
		t.Fatalf("CloseTunnel() calls = %d, want preflight cleanup", got)
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
	if got := manager.disposeCalls.Load(); got != 0 {
		t.Fatalf("CloseTunnel() calls = %d, want no session cleanup", got)
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
