package wrappers

import (
	"net/netip"
	"sync"
	"tungo/application"
	"tungo/infrastructure/routing/server_routing/session_management/repository"
)

type ConcurrentManager[cs application.Session] struct {
	mu      sync.RWMutex
	manager repository.SessionRepository[cs]
}

func NewConcurrentManager[cs application.Session](manager repository.SessionRepository[cs]) repository.SessionRepository[cs] {
	return &ConcurrentManager[cs]{
		manager: manager,
	}
}

func (c *ConcurrentManager[cs]) Add(session cs) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.manager.Add(session)
}

func (c *ConcurrentManager[cs]) Delete(session cs) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.manager.Delete(session)
}

func (c *ConcurrentManager[cs]) GetByInternalAddrPort(addr netip.Addr) (cs, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.manager.GetByInternalAddrPort(addr)
}

func (c *ConcurrentManager[cs]) GetByExternalAddrPort(addrPort netip.AddrPort) (cs, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.manager.GetByExternalAddrPort(addrPort)
}
