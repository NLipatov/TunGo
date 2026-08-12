package server

import (
	"sync/atomic"

	serverconfig "tungo/internal/config/server"
)

type allowedPeers struct {
	peers atomic.Pointer[map[string]allowedPeer]
}

type allowedPeer struct {
	enabled  bool
	clientID int
}

func newAllowedPeers(peers []serverconfig.AllowedPeer) *allowedPeers {
	lookup := &allowedPeers{}
	lookup.Update(peers)
	return lookup
}

func (a *allowedPeers) Lookup(publicKey []byte) (int, bool, bool) {
	peers := a.peers.Load()
	if peers == nil {
		return 0, false, false
	}
	peer, ok := (*peers)[string(publicKey)]
	if !ok {
		return 0, false, false
	}
	return peer.clientID, peer.enabled, true
}

func (a *allowedPeers) Update(peers []serverconfig.AllowedPeer) {
	byPublicKey := make(map[string]allowedPeer, len(peers))
	for _, peer := range peers {
		byPublicKey[string(peer.PublicKey)] = allowedPeer{
			enabled:  peer.Enabled,
			clientID: peer.ClientID,
		}
	}
	a.peers.Store(&byPublicKey)
}
