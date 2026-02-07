package session

import (
	"net/netip"
	"sync"

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
	// FindByDestinationIP finds the peer that should receive packets destined for addr.
	// Checks both internal IP (exact match) and AllowedIPs (prefix match).
	// Used for egress routing (TUN → client).
	FindByDestinationIP(addr netip.Addr) (*Peer, error)
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

// DefaultRepository is a thread-safe session repository.
//
// CONCURRENCY INVARIANT: All map operations are protected by RWMutex.
// - Read operations (Get*, Find*) acquire RLock for concurrent reads
// - Write operations (Add, Delete, TerminateByPubKey) acquire Lock for exclusive access
//
// LIFECYCLE INVARIANT: Delete zeroes key material AFTER removing from maps.
// This ensures no new lookups can return the peer while zeroing is in progress.
type DefaultRepository struct {
	mu               sync.RWMutex
	internalIpToPeer map[netip.Addr]*Peer
	externalIPToPeer map[netip.AddrPort]*Peer
	// pubKeyToPeers tracks sessions by client public key for revocation support.
	// Multiple sessions may exist for the same pubkey (e.g., TCP + UDP).
	pubKeyToPeers map[string][]*Peer
}

func NewDefaultRepository() Repository {
	return &DefaultRepository{
		internalIpToPeer: make(map[netip.Addr]*Peer),
		externalIPToPeer: make(map[netip.AddrPort]*Peer),
		pubKeyToPeers:    make(map[string][]*Peer),
	}
}

func (s *DefaultRepository) Add(peer *Peer) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.internalIpToPeer[peer.InternalAddr().Unmap()] = peer
	s.externalIPToPeer[s.canonicalAP(peer.ExternalAddrPort())] = peer

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

func (s *DefaultRepository) canonicalAP(ap netip.AddrPort) netip.AddrPort {
	ip := ap.Addr().Unmap()
	return netip.AddrPortFrom(ip, ap.Port())
}

// FindByDestinationIP finds the peer that should receive packets destined for addr.
// Fast path: O(1) lookup by internal IP.
// Slow path: O(n) scan through all peers checking AllowedIPs prefixes.
//
// SECURITY NOTE (R-18): The O(n) scan creates a minor timing side-channel that
// could reveal the approximate number of peers. This is an ACCEPTED RISK because:
// - Peer count is not highly sensitive information
// - Impact is minimal (attacker learns "few" vs "many" peers)
// - Fix would require radix tree, adding complexity without meaningful security benefit
// - Typical deployments have <100 peers, making timing differences negligible
func (s *DefaultRepository) FindByDestinationIP(addr netip.Addr) (*Peer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	normalized := addr.Unmap()

	// Fast path: exact match on internal IP
	if peer, found := s.internalIpToPeer[normalized]; found {
		return peer, nil
	}

	// Slow path: check AllowedIPs for each peer
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

	// Step 4: Zero key material - safe because:
	// - Peer is marked closed (handlers will abort)
	// - Egress is closed (no new writes)
	// - Peer is removed from maps (no new lookups)
	if crypto := peer.Crypto(); crypto != nil {
		if zeroizer, ok := crypto.(connection.CryptoZeroizer); ok {
			zeroizer.Zeroize()
		}
	}
}
