package session_management

import "sync"

type ConcurrentManager[s ClientSession] struct {
	mu      sync.RWMutex
	manager WorkerSessionManager[s]
}

func NewConcurrentManager[s ClientSession](manager WorkerSessionManager[s]) WorkerSessionManager[s] {
	return &ConcurrentManager[s]{
		manager: manager,
	}
}

func (c *ConcurrentManager[s]) Add(session s) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.manager.Add(session)
}

func (c *ConcurrentManager[s]) Delete(session s) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.manager.Delete(session)
}

func (c *ConcurrentManager[s]) GetByInternalIP(ip []byte) (s, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.manager.GetByInternalIP(ip)
}

func (c *ConcurrentManager[s]) GetByExternalIP(ip []byte) (s, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.manager.GetByExternalIP(ip)
}
