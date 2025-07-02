package session_management

import (
	"context"
	"net/netip"
	"sync"
	"time"
	"tungo/infrastructure/PAL/server_configuration"
)

type TTLManager[cs interface {
	ClientSession
	comparable
}] struct {
	ctx                         context.Context
	manager                     WorkerSessionManager[cs]
	mu                          sync.RWMutex
	expMap                      map[cs]time.Time
	sessionTtl, cleanupInterval time.Duration
}

func NewTTLManager[cs interface {
	ClientSession
	comparable
}](
	ctx context.Context,
	manager WorkerSessionManager[cs],
	expDuration, sanitizeInterval time.Duration,
) WorkerSessionManager[cs] {
	tm := &TTLManager[cs]{
		ctx:             ctx,
		manager:         manager,
		expMap:          make(map[cs]time.Time),
		sessionTtl:      expDuration,
		cleanupInterval: sanitizeInterval,
	}
	go tm.sanitize()
	return tm
}

func (t *TTLManager[cs]) Add(session cs) {
	t.manager.Add(session)

	t.mu.Lock()
	t.expMap[session] = time.Now().Add(t.sessionTtl)
	t.mu.Unlock()
}
func (t *TTLManager[cs]) Delete(session cs) {
	t.manager.Delete(session)

	t.mu.Lock()
	delete(t.expMap, session)
	t.mu.Unlock()
}
func (t *TTLManager[cs]) GetByInternalIP(addr netip.Addr) (cs, error) {
	var zero cs
	session, sessionErr := t.manager.GetByInternalIP(addr)
	if sessionErr != nil {
		return zero, sessionErr
	}

	t.mu.Lock()
	t.expMap[session] = time.Now().Add(t.sessionTtl)
	t.mu.Unlock()

	return session, nil
}
func (t *TTLManager[cs]) GetByExternalIP(addrPort netip.AddrPort) (cs, error) {
	var zero cs
	session, sessionErr := t.manager.GetByExternalIP(addrPort)
	if sessionErr != nil {
		return zero, sessionErr
	}

	t.mu.Lock()
	t.expMap[session] = time.Now().Add(t.sessionTtl)
	t.mu.Unlock()

	return session, nil
}

func (t *TTLManager[cs]) sanitize() {
	if t.cleanupInterval == 0 {
		t.cleanupInterval = time.Duration(server_configuration.DefaultSessionCleanupInterval)
	}

	if t.sessionTtl == 0 {
		t.sessionTtl = time.Duration(server_configuration.DefaultSessionTtl)
	}

	ticker := time.NewTicker(t.cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			expired := make([]cs, 0, len(t.expMap))
			t.mu.RLock()
			for session, expiresAt := range t.expMap {
				if time.Now().After(expiresAt) {
					expired = append(expired, session)
				}
			}
			t.mu.RUnlock()

			if len(expired) > 0 {
				t.mu.Lock()
				for _, session := range expired {
					t.manager.Delete(session)
					delete(t.expMap, session)
				}
				t.mu.Unlock()
			}
		}
	}
}
