package session

import (
	"net/netip"
	"sync"
	"time"

	"tungo/application/network/connection"
)

type Repository interface {
	// Add adds peer to the repository
	Add(peer *Peer)
	// Delete deletes peer from the repository and zeroes key material
	Delete(peer *Peer)
	// GetByInternalAddrPort tries to retrieve peer by internal(in vpn) ip
	GetByInternalAddrPort(addr netip.Addr) (*Peer, error)
	// GetByExternalAddrPort tries to retrieve peer by external(outside of vpn) ip and port combination
	GetByExternalAddrPort(addrPort netip.AddrPort) (*Peer, error)
	// GetByRouteID tries to retrieve peer by stable per-session UDP route identifier.
	GetByRouteID(routeID uint64) (*Peer, error)
	// FindByDestinationIP finds the peer that should receive packets destined for addr.
	// Checks both internal IP (exact match) and AllowedIPs (prefix match).
	// Used for egress routing (TUN → client).
	FindByDestinationIP(addr netip.Addr) (*Peer, error)
	// AllPeers returns a snapshot slice of all peers in the repository.
	AllPeers() []*Peer
	// UpdateExternalAddr atomically re-indexes the peer under a new external address.
	// Used when a client roams to a different NAT endpoint.
	UpdateExternalAddr(peer *Peer, newAddr netip.AddrPort)
}

// RepositoryWithRevocation extends Repository with session revocation capability.
// Used when AllowedPeers configuration changes require terminating existing sessions.
type RepositoryWithRevocation interface {
	Repository
	// TerminateByPubKey finds and terminates all sessions for the given public key.
	// Returns the number of sessions terminated.
	// SECURITY: Must be called after AllowedPeers config changes to prevent
	// stale AllowedIPs snapshots from persisting.
	TerminateByPubKey(pubKey []byte) int
}

// IdleReaper is implemented by repositories that support idle session cleanup.
type IdleReaper interface {
	// ReapIdle deletes all sessions whose last activity is older than timeout.
	// Returns the number of sessions reaped.
	ReapIdle(timeout time.Duration) int
}

// DefaultRepository is a thread-safe session repository.
//
// CONCURRENCY INVARIANT: All map operations are protected by RWMutex.
// - Read operations (Get*, Find*) acquire RLock for concurrent reads
// - Write operations (Add, Delete, TerminateByPubKey) acquire Lock for exclusive access
//
// LIFECYCLE INVARIANT: Delete zeroes key material AFTER removing from maps.
// This ensures no new lookups can return the peer while zeroing is in progress.
type DefaultRepository struct {
	mu                sync.RWMutex
	internalIpToPeer  map[netip.Addr]*Peer
	externalIPToPeer  map[netip.AddrPort]*Peer
	routeIDToPeer     map[uint64]*Peer
	allowedAddrToPeer map[netip.Addr]*Peer // host-route (/32, /128) from AllowedIPs for O(1) lookup
	// pubKeyToPeers tracks sessions by client public key for revocation support.
	// Multiple sessions may exist for the same pubkey (e.g., TCP + UDP).
	pubKeyToPeers map[string][]*Peer
}

func NewDefaultRepository() Repository {
	return &DefaultRepository{
		internalIpToPeer:  make(map[netip.Addr]*Peer),
		externalIPToPeer:  make(map[netip.AddrPort]*Peer),
		routeIDToPeer:     make(map[uint64]*Peer),
		allowedAddrToPeer: make(map[netip.Addr]*Peer),
		pubKeyToPeers:     make(map[string][]*Peer),
	}
}

func (s *DefaultRepository) Add(peer *Peer) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.internalIpToPeer[peer.InternalAddr().Unmap()] = peer
	s.externalIPToPeer[s.canonicalAP(peer.ExternalAddrPort())] = peer
	if routeID, ok := peerRouteID(peer); ok {
		s.routeIDToPeer[routeID] = peer
	}

	// Index allowed addresses for O(1) peer lookup (e.g. IPv6 address)
	if sess, ok := peer.Session.(*Session); ok {
		for addr := range sess.AllowedAddrs() {
			s.allowedAddrToPeer[addr] = peer
		}
	}

	// Track by public key for revocation support
	if identity, ok := peer.Session.(connection.SessionIdentity); ok {
		if pubKey := identity.ClientPubKey(); len(pubKey) > 0 {
			key := string(pubKey)
			s.pubKeyToPeers[key] = append(s.pubKeyToPeers[key], peer)
		}
	}
}

// Delete removes peer from repository and zeroes key material.
//
// LIFECYCLE INVARIANT: Operations happen in this order to prevent use-after-free:
// 1. Close egress (signals TCP workers to exit, prevents new writes)
// 2. Remove from maps (prevents new lookups returning this peer)
// 3. Zero key material (safe because no new operations can start)
//
// For UDP: mutex serializes Delete with packet processing
// For TCP: closing egress causes worker's Read to fail and exit
func (s *DefaultRepository) Delete(peer *Peer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteLocked(peer)
}

func (s *DefaultRepository) GetByInternalAddrPort(addr netip.Addr) (*Peer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	value, found := s.internalIpToPeer[addr.Unmap()]
	if !found {
		return nil, ErrNotFound
	}
	return value, nil
}

func (s *DefaultRepository) GetByExternalAddrPort(addr netip.AddrPort) (*Peer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	value, found := s.externalIPToPeer[s.canonicalAP(addr)]
	if !found {
		return nil, ErrNotFound
	}
	return value, nil
}

func (s *DefaultRepository) GetByRouteID(routeID uint64) (*Peer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	value, found := s.routeIDToPeer[routeID]
	if !found {
		return nil, ErrNotFound
	}
	return value, nil
}

func (s *DefaultRepository) canonicalAP(ap netip.AddrPort) netip.AddrPort {
	ip := ap.Addr().Unmap()
	return netip.AddrPortFrom(ip, ap.Port())
}

func (s *DefaultRepository) AllPeers() []*Peer {
	s.mu.RLock()
	defer s.mu.RUnlock()

	peers := make([]*Peer, 0, len(s.internalIpToPeer))
	for _, p := range s.internalIpToPeer {
		peers = append(peers, p)
	}
	return peers
}

func (s *DefaultRepository) UpdateExternalAddr(peer *Peer, newAddr netip.AddrPort) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Guard against re-inserting a peer that was concurrently deleted.
	// Without this check, a zombie entry would persist in externalIPToPeer.
	if peer.IsClosed() {
		return
	}

	// Remove old external address index entry
	delete(s.externalIPToPeer, s.canonicalAP(peer.ExternalAddrPort()))
	// Update the peer's external address
	peer.SetExternalAddrPort(newAddr)
	// Update the egress writer's destination so replies go to the new address.
	peer.updateEgressAddr(newAddr)
	// Re-index under the new address
	s.externalIPToPeer[s.canonicalAP(newAddr)] = peer
}

// FindByDestinationIP finds the peer that should receive packets destined for addr.
// Fast path: O(1) lookup by internal IP, then O(1) by AllowedIPs host-routes.
// Slow path: O(n) scan through all peers checking non-host AllowedIPs prefixes.
func (s *DefaultRepository) FindByDestinationIP(addr netip.Addr) (*Peer, error) {
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
func (s *DefaultRepository) TerminateByPubKey(pubKey []byte) int {
	if len(pubKey) == 0 {
		return 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := string(pubKey)
	peers := s.pubKeyToPeers[key]
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
func (s *DefaultRepository) deleteLocked(peer *Peer) {
	// Step 1: Mark closed FIRST - this is checked by packet handlers
	// to abort before using crypto. Atomic operation visible immediately.
	peer.markClosed()

	// Step 2: Close egress to signal session termination
	if peer.egress != nil {
		_ = peer.egress.Close()
	}

	// Step 3: Remove from all maps
	delete(s.internalIpToPeer, peer.InternalAddr().Unmap())
	delete(s.externalIPToPeer, s.canonicalAP(peer.ExternalAddrPort()))
	if routeID, ok := peerRouteID(peer); ok {
		if indexed := s.routeIDToPeer[routeID]; indexed == peer {
			delete(s.routeIDToPeer, routeID)
		}
	}

	// Remove allowed address entries from index
	if sess, ok := peer.Session.(*Session); ok {
		for addr := range sess.AllowedAddrs() {
			if s.allowedAddrToPeer[addr] == peer {
				delete(s.allowedAddrToPeer, addr)
			}
		}
	}

	// Remove from pubkey index
	if identity, ok := peer.Session.(connection.SessionIdentity); ok {
		if pubKey := identity.ClientPubKey(); len(pubKey) > 0 {
			key := string(pubKey)
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
	}

	// Step 4: Zero key material — wait for active decrypts to finish.
	// cryptoMu.Lock blocks until all in-flight CryptoRLock holders release.
	peer.cryptoMu.Lock()
	if crypto := peer.Crypto(); crypto != nil {
		if zeroizer, ok := crypto.(connection.CryptoZeroizer); ok {
			zeroizer.Zeroize()
		}
	}
	peer.cryptoMu.Unlock()
}

func peerRouteID(peer *Peer) (uint64, bool) {
	type routeIDProvider interface {
		RouteID() uint64
	}

	crypto := peer.Crypto()
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
func (s *DefaultRepository) ReapIdle(timeout time.Duration) int {
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
