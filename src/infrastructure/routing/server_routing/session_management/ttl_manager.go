package session_management

import (
	"context"
	"net/netip"
	"sync"
	"time"
)

const (
	defaultSessionTTL       = 12 * time.Hour
	defaultSanitizeInterval = 6 * time.Hour
)

type TTLManager[cs interface {
	ClientSession
	comparable
}] struct {
	ctx                           context.Context
	manager                       WorkerSessionManager[cs]
	mu                            sync.RWMutex
	expMap                        map[cs]time.Time
	expDuration, sanitizeInterval time.Duration
}

func NewTTLManager[cs interface {
	ClientSession
	comparable
}](
	ctx context.Context,
	manager WorkerSessionManager[cs],
	expDuration, sanitizeInterval time.Duration,
) WorkerSessionManager[cs] {
	if expDuration <= 0 {
		expDuration = defaultSessionTTL
	}

	if sanitizeInterval <= 0 {
		sanitizeInterval = defaultSanitizeInterval
	}

	tm := &TTLManager[cs]{
		ctx:              ctx,
		manager:          manager,
		expMap:           make(map[cs]time.Time),
		expDuration:      expDuration,
		sanitizeInterval: sanitizeInterval,
	}
	go tm.sanitize()
	return tm
}

func (t *TTLManager[cs]) Add(session cs) {
	t.manager.Add(session)

	t.mu.Lock()
	t.expMap[session] = time.Now().Add(t.expDuration)
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
	t.expMap[session] = time.Now().Add(t.expDuration)
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
	t.expMap[session] = time.Now().Add(t.expDuration)
	t.mu.Unlock()

	return session, nil
}

func (t *TTLManager[cs]) sanitize() {
	ticker := time.NewTicker(t.sanitizeInterval)
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
