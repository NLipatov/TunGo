package session_management

import "sync"

type ConcurrentManager[cs ClientSession] struct {
	mu      sync.RWMutex
	manager WorkerSessionManager[cs]
}

func NewConcurrentManager[cs ClientSession](manager WorkerSessionManager[cs]) WorkerSessionManager[cs] {
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

func (c *ConcurrentManager[cs]) GetByInternalIP(ip [4]byte) (cs, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.manager.GetByInternalIP(ip)
}

func (c *ConcurrentManager[cs]) GetByExternalIP(ip [4]byte) (cs, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.manager.GetByExternalIP(ip)
}
