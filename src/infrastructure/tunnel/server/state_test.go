package server

import (
	"net/netip"
	"testing"

	appConfiguration "tungo/application/configuration"
	"tungo/infrastructure/cryptography/noise"
	"tungo/infrastructure/tunnel/server/internal/session"
)

func TestServerUpdateReplacesAllowedPeers(t *testing.T) {
	publicKey := []byte("peer")
	r := Server{
		allowedPeers: noise.NewAllowedPeersLookup(nil),
	}
	r.Update([]appConfiguration.ServerPeer{{PublicKey: publicKey, ClientID: 7, Enabled: true}})

	clientID, enabled, found := r.allowedPeers.Lookup(publicKey)
	if !found || !enabled || clientID != 7 {
		t.Fatalf("Lookup() = (%d, %v, %v), want (7, true, true)", clientID, enabled, found)
	}
}

func TestRuntimeRegistersRepositories(t *testing.T) {
	r := Server{}
	r.register(session.NewRepository())

	if len(r.repositories) != 1 {
		t.Fatalf("repositories = %d, want 1", len(r.repositories))
	}
}

func TestServerRevokesSessionsAcrossProtocols(t *testing.T) {
	key := []byte("client-key")
	first := session.NewRepository()
	second := session.NewRepository()
	first.Add(session.NewPeerWithAuth(
		nil, nil, netip.MustParseAddr("10.0.0.2"), netip.MustParseAddrPort("192.0.2.1:1"), key, nil, nil,
	))
	second.Add(session.NewPeerWithAuth(
		nil, nil, netip.MustParseAddr("10.0.0.3"), netip.MustParseAddrPort("192.0.2.2:2"), key, nil, nil,
	))
	server := Server{}
	server.register(first)
	server.register(second)

	if revoked := server.RevokeByPubKey(key); revoked != 2 {
		t.Fatalf("RevokeByPubKey() = %d, want 2", revoked)
	}
}
