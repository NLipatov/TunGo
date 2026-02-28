package server

import (
	"testing"

	"tungo/infrastructure/cryptography/noise"
	"tungo/infrastructure/tunnel/session"
)

func TestRuntime_AllowedPeersUpdater_NonNil(t *testing.T) {
	r := Runtime{
		allowedPeers: noise.NewAllowedPeersLookup(nil),
	}

	if r.AllowedPeersUpdater() == nil {
		t.Fatal("expected non-nil AllowedPeersUpdater when allowedPeers is set")
	}
}

func TestRuntime_AllowedPeersUpdater_Nil(t *testing.T) {
	r := Runtime{
		allowedPeers: nil,
	}

	if r.AllowedPeersUpdater() != nil {
		t.Fatal("expected nil AllowedPeersUpdater when allowedPeers is nil")
	}
}

func TestRuntime_SessionRevoker(t *testing.T) {
	revoker := session.NewCompositeSessionRevoker()
	r := Runtime{
		sessionRevoker: revoker,
	}

	if r.SessionRevoker() != revoker {
		t.Fatal("expected SessionRevoker() to return the same revoker instance")
	}
}
