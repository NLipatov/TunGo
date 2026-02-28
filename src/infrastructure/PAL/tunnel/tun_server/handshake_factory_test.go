package tun_server

import (
	"errors"
	"io"
	"testing"

	"tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/cryptography/noise"
)

type handshakeFactoryReadOnlyTransport struct{}

func (handshakeFactoryReadOnlyTransport) Write(_ []byte) (int, error) { return 0, nil }
func (handshakeFactoryReadOnlyTransport) Read(_ []byte) (int, error)  { return 0, io.EOF }
func (handshakeFactoryReadOnlyTransport) Close() error                { return nil }

func TestHandshakeFactory_NewHandshake_MissingServerKey(t *testing.T) {
	cookieManager, err := noise.NewCookieManager()
	if err != nil {
		t.Fatalf("failed to create cookie manager: %v", err)
	}

	f := NewHandshakeFactory(
		server.Configuration{},
		noise.NewAllowedPeersLookup(nil),
		cookieManager,
		noise.NewLoadMonitor(1),
	)
	hs := f.NewHandshake()
	if hs == nil {
		t.Fatal("expected non-nil handshake")
	}

	_, err = hs.ServerSideHandshake(handshakeFactoryReadOnlyTransport{})
	if !errors.Is(err, noise.ErrMissingServerKey) {
		t.Fatalf("expected ErrMissingServerKey, got: %v", err)
	}
}

func TestHandshakeFactory_NewHandshake_MissingAllowedPeers(t *testing.T) {
	cfg := server.Configuration{
		X25519PublicKey:  make([]byte, 32),
		X25519PrivateKey: make([]byte, 32),
	}
	f := NewHandshakeFactory(cfg, nil, nil, nil)

	_, err := f.NewHandshake().ServerSideHandshake(handshakeFactoryReadOnlyTransport{})
	if !errors.Is(err, noise.ErrMissingAllowedPeers) {
		t.Fatalf("expected ErrMissingAllowedPeers, got: %v", err)
	}
}
