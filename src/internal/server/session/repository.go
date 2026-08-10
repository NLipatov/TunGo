package session

import (
	"net/netip"
	"sync"
	"time"
)

// Repository is a thread-safe session repository.
//
// CONCURRENCY INVARIANT: All map operations are protected by RWMutex.
// - Read operations (Get*, Find*) acquire RLock for concurrent reads
// - Write operations (Add, Delete, TerminateByPubKey) acquire Lock for exclusive access
//
// LIFECYCLE INVARIANT: Delete zeroes key material AFTER removing from maps.
// This ensures no new lookups can return the peer while zeroing is in progress.
type Repository struct {
	mu                sync.RWMutex
	internalIpToPeer  map[netip.Addr]*Peer
	routeIDToPeer     map[uint64]*Peer
	allowedAddrToPeer map[netip.Addr]*Peer // host-route (/32, /128) from AllowedIPs for O(1) lookup
	// pubKeyToPeers tracks sessions by client public key for revocation support.
	// Multiple sessions may exist for the same pubkey (e.g., TCP + UDP).
	pubKeyToPeers map[string][]*Peer
}

func NewRepository() *Repository {
	return &Repository{
		internalIpToPeer:  make(map[netip.Addr]*Peer),
		routeIDToPeer:     make(map[uint64]*Peer),
		allowedAddrToPeer: make(map[netip.Addr]*Peer),
		pubKeyToPeers:     make(map[string][]*Peer),
	}
}

func (s *Repository) Add(peer *Peer) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.internalIpToPeer[peer.InternalAddr().Unmap()] = peer
	if routeID, ok := peerRouteID(peer); ok {
		s.routeIDToPeer[routeID] = peer
	}

	// Index allowed addresses for O(1) peer lookup (e.g. IPv6 address)
	for addr := range peer.allowedAddrs {
		s.allowedAddrToPeer[addr] = peer
	}

	// Track by public key for revocation support
	if len(peer.clientPubKey) > 0 {
		key := string(peer.clientPubKey)
		s.pubKeyToPeers[key] = append(s.pubKeyToPeers[key], peer)
	}
}

// Delete removes peer from repository and zeroes key material.
//
// LIFECYCLE INVARIANT: mark closed, close egress, remove routes, then wait for
// in-flight Peer.Send/Decrypt calls before zeroing key material.
func (s *Repository) Delete(peer *Peer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteLocked(peer)
}

func (s *Repository) GetByInternalAddrPort(addr netip.Addr) (*Peer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	value, found := s.internalIpToPeer[addr.Unmap()]
	if !found {
		return nil, ErrNotFound
	}
	return value, nil
}

func (s *Repository) GetByRouteID(routeID uint64) (*Peer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	value, found := s.routeIDToPeer[routeID]
	if !found {
		return nil, ErrNotFound
	}
	return value, nil
}

func (s *Repository) UpdateExternalAddr(peer *Peer, newAddr netip.AddrPort) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Guard against re-inserting a peer that was concurrently deleted.
	if peer.IsClosed() {
		return
	}

	peer.SetExternalAddrPort(newAddr)
	peer.updateEgressAddr(newAddr)
}

// FindByDestinationIP finds the peer that should receive packets destined for addr.
// Fast path: O(1) lookup by internal IP, then O(1) by AllowedIPs host-routes.
// Slow path: O(n) scan through all peers checking non-host AllowedIPs prefixes.
func (s *Repository) FindByDestinationIP(addr netip.Addr) (*Peer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	normalized := addr.Unmap()

	// Fast path: exact match on internal IP (IPv4)
	if peer, found := s.internalIpToPeer[normalized]; found {
		return peer, nil
	}

	// Fast path: exact match on AllowedIPs host-route (IPv6 /128, etc.)
	if peer, found := s.allowedAddrToPeer[normalized]; found {
		return peer, nil
	}

	// Slow path: check non-host AllowedIPs prefixes for each peer
	for _, peer := range s.internalIpToPeer {
		if peer.IsSourceAllowed(normalized) {
			return peer, nil
		}
	}

	return nil, ErrNotFound
}

// TerminateByPubKey finds and terminates all sessions for the given public key.
// Returns the number of sessions terminated.
//
// SECURITY: Must be called after AllowedPeers config changes to prevent
// stale AllowedIPs snapshots from persisting.
//
// LIFECYCLE: First closes all egress paths (signals workers to exit),
// then removes from maps, then zeroes keys. This ordering prevents use-after-free.
func (s *Repository) TerminateByPubKey(pubKey []byte) int {
	if len(pubKey) == 0 {
		return 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	peers := s.pubKeyToPeers[string(pubKey)]
	if len(peers) == 0 {
		return 0
	}

	// Copy slice since deleteLocked modifies the index
	toDelete := make([]*Peer, len(peers))
	copy(toDelete, peers)

	for _, peer := range toDelete {
		s.deleteLocked(peer)
	}
	return len(toDelete)
}

// deleteLocked removes peer from repository. Caller MUST hold s.mu.Lock().
// This is the internal implementation used by both Delete and TerminateByPubKey.
//
// LIFECYCLE ORDER (prevents use-after-free):
// 1. Mark peer as closed (atomic flag - callers will see this immediately)
// 2. Close egress (TCP workers will exit, UDP writes will fail)
// 3. Remove from maps (no new lookups can find this peer)
// 4. Zero key material (safe - no active users possible)
func (s *Repository) deleteLocked(peer *Peer) {
	if peer.IsClosed() {
		return
	}
	// Step 1: Mark closed FIRST - this is checked by packet handlers
	// to abort before using crypto. Atomic operation visible immediately.
	peer.markClosed()

	// Step 2: Close egress to signal session termination
	if peer.sender != nil {
		_ = peer.sender.Close()
	}

	// Step 3: Remove from all maps
	delete(s.internalIpToPeer, peer.InternalAddr().Unmap())
	if routeID, ok := peerRouteID(peer); ok {
		if indexed := s.routeIDToPeer[routeID]; indexed == peer {
			delete(s.routeIDToPeer, routeID)
		}
	}

	// Remove allowed address entries from index
	for addr := range peer.allowedAddrs {
		if s.allowedAddrToPeer[addr] == peer {
			delete(s.allowedAddrToPeer, addr)
		}
	}

	// Remove from pubkey index
	if len(peer.clientPubKey) > 0 {
		key := string(peer.clientPubKey)
		peers := s.pubKeyToPeers[key]
		for i, p := range peers {
			if p == peer {
				peers[i] = peers[len(peers)-1]
				s.pubKeyToPeers[key] = peers[:len(peers)-1]
				break
			}
		}
		if len(s.pubKeyToPeers[key]) == 0 {
			delete(s.pubKeyToPeers, key)
		}
	}

	// Step 4: Zero key material after all in-flight Send/Decrypt calls finish.
	peer.cryptoMu.Lock()
	if crypto := peer.crypto; crypto != nil {
		if zeroizer, ok := crypto.(interface{ Zeroize() }); ok {
			zeroizer.Zeroize()
		}
	}
	peer.cryptoMu.Unlock()
}

func peerRouteID(peer *Peer) (uint64, bool) {
	type routeIDProvider interface {
		RouteID() uint64
	}

	crypto := peer.crypto
	if crypto == nil {
		return 0, false
	}
	provider, ok := crypto.(routeIDProvider)
	if !ok {
		return 0, false
	}
	return provider.RouteID(), true
}

// ReapIdle deletes all sessions whose last activity is older than timeout.
// Safe to call concurrently; acquires write lock internally.
// Deleting from a map during range iteration is safe in Go.
func (s *Repository) ReapIdle(timeout time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-timeout)
	var count int
	for _, peer := range s.internalIpToPeer {
		if peer.LastActivity().Before(cutoff) {
			s.deleteLocked(peer)
			count++
		}
	}
	return count
}
