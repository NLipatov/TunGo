package server

import (
	"errors"
	"io"
	"testing"

	appConfiguration "tungo/application/configuration"
	"tungo/infrastructure/cryptography/noise"
)

type handshakeFactoryReadOnlyTransport struct{}

func (handshakeFactoryReadOnlyTransport) Write(_ []byte) (int, error) { return 0, nil }
func (handshakeFactoryReadOnlyTransport) Read(_ []byte) (int, error)  { return 0, io.EOF }
func (handshakeFactoryReadOnlyTransport) Close() error                { return nil }

func TestNewHandshake_MissingServerKey(t *testing.T) {
	cookieManager, err := noise.NewCookieManager()
	if err != nil {
		t.Fatalf("failed to create cookie manager: %v", err)
	}

	server := &Server{
		configuration: appConfiguration.ServerRuntimeConfiguration{},
		allowedPeers:  noise.NewAllowedPeersLookup(nil),
		cookieManager: cookieManager,
		loadMonitor:   noise.NewLoadMonitor(1),
	}
	hs := server.newHandshake()
	if hs == nil {
		t.Fatal("expected non-nil handshake")
	}

	_, err = hs.ServerSideHandshake(handshakeFactoryReadOnlyTransport{})
	if !errors.Is(err, noise.ErrMissingServerKey) {
		t.Fatalf("expected ErrMissingServerKey, got: %v", err)
	}
}

func TestNewHandshake_MissingAllowedPeers(t *testing.T) {
	cfg := appConfiguration.ServerRuntimeConfiguration{
		X25519PublicKey:  make([]byte, 32),
		X25519PrivateKey: make([]byte, 32),
	}
	server := &Server{configuration: cfg}

	_, err := server.newHandshake().ServerSideHandshake(handshakeFactoryReadOnlyTransport{})
	if !errors.Is(err, noise.ErrMissingAllowedPeers) {
		t.Fatalf("expected ErrMissingAllowedPeers, got: %v", err)
	}
}
