package session

import (
	"net/netip"
	"sync"
)

type ConcurrentRepository struct {
	mu      sync.RWMutex
	manager Repository
}

func NewConcurrentRepository(manager Repository) Repository {
	return &ConcurrentRepository{
		manager: manager,
	}
}

func (c *ConcurrentRepository) Add(peer *Peer) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.manager.Add(peer)
}

func (c *ConcurrentRepository) Delete(peer *Peer) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.manager.Delete(peer)
}

func (c *ConcurrentRepository) GetByInternalAddrPort(addr netip.Addr) (*Peer, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.manager.GetByInternalAddrPort(addr)
}

func (c *ConcurrentRepository) GetByExternalAddrPort(addrPort netip.AddrPort) (*Peer, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.manager.GetByExternalAddrPort(addrPort)
}

func (c *ConcurrentRepository) FindByDestinationIP(addr netip.Addr) (*Peer, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.manager.FindByDestinationIP(addr)
}

// TerminateByPubKey implements RepositoryWithRevocation.
// Thread-safe wrapper that delegates to the underlying repository.
func (c *ConcurrentRepository) TerminateByPubKey(pubKey []byte) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if revocable, ok := c.manager.(RepositoryWithRevocation); ok {
		return revocable.TerminateByPubKey(pubKey)
	}
	return 0
}
