package noise

type testPeer struct {
	PublicKey []byte
	Enabled   bool
	ClientID  int
}

type testPeers map[string]testPeer

func newTestAllowedPeers(peers []testPeer) AllowedPeersLookup {
	lookup := make(testPeers, len(peers))
	for _, peer := range peers {
		lookup[string(peer.PublicKey)] = peer
	}
	return lookup
}

func (a testPeers) Lookup(publicKey []byte) (int, bool, bool) {
	peer, ok := a[string(publicKey)]
	return peer.ClientID, peer.Enabled, ok
}
