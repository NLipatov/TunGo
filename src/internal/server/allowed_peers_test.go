package server

import (
	"testing"

	serverconfig "tungo/internal/config/server"
)

func TestAllowedPeersUpdateReplacesLookup(t *testing.T) {
	oldKey := []byte("old")
	newKey := []byte("new")
	peers := newAllowedPeers([]serverconfig.AllowedPeer{{
		PublicKey: oldKey,
		ClientID:  1,
		Enabled:   true,
	}})

	peers.Update([]serverconfig.AllowedPeer{{
		PublicKey: newKey,
		ClientID:  2,
		Enabled:   false,
	}})

	if _, _, found := peers.Lookup(oldKey); found {
		t.Fatal("old peer remains after replacement")
	}
	clientID, enabled, found := peers.Lookup(newKey)
	if !found || enabled || clientID != 2 {
		t.Fatalf("Lookup() = (%d, %v, %v), want (2, false, true)", clientID, enabled, found)
	}
}
