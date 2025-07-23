package wrappers

import (
	"context"
	"net/netip"
	"sync"
	"time"
	"tungo/infrastructure/PAL/server_configuration"
	"tungo/infrastructure/routing/server_routing/session_management"
	"tungo/infrastructure/routing/server_routing/session_management/repository"
)

type sessionWithTTL[cs comparable] struct {
	session cs
	expire  time.Time
}

// TTLManager manages sessions with automatic TTL expiration and efficient lookups.
type TTLManager[cs interface {
	session_management.SessionContract
	comparable
}] struct {
	ctx                         context.Context
	manager                     repository.SessionRepository[cs]
	mu                          sync.RWMutex
	ipToSession                 map[netip.Addr]sessionWithTTL[cs]     // Internal address lookup
	externalToSession           map[netip.AddrPort]sessionWithTTL[cs] // External address lookup
	sessionTtl, cleanupInterval time.Duration
}

// NewTTLManager creates a new TTLManager with custom TTL and cleanup intervals.
func NewTTLManager[cs interface {
	session_management.SessionContract
	comparable
}](
	ctx context.Context,
	manager repository.SessionRepository[cs],
	expDuration, sanitizeInterval time.Duration,
) repository.SessionRepository[cs] {
	tm := &TTLManager[cs]{
		ctx:               ctx,
		manager:           manager,
		ipToSession:       make(map[netip.Addr]sessionWithTTL[cs]),
		externalToSession: make(map[netip.AddrPort]sessionWithTTL[cs]),
		sessionTtl:        expDuration,
		cleanupInterval:   sanitizeInterval,
	}
	go tm.sanitize()
	return tm
}

// Add adds a new session and removes any existing session with the same internal address.
func (t *TTLManager[cs]) Add(session cs) {
	t.Delete(session)

	t.mu.Lock()
	defer t.mu.Unlock()
	t.manager.Add(session)
	entry := sessionWithTTL[cs]{
		session: session,
		expire:  time.Now().Add(t.sessionTtl),
	}
	t.ipToSession[session.InternalAddr()] = entry
	t.externalToSession[session.ExternalAddrPort()] = entry
}

// Delete removes a session from both internal and external lookup maps.
func (t *TTLManager[cs]) Delete(session cs) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// close previous session
	if entry, ok := t.ipToSession[session.InternalAddr()]; ok {
		_ = entry.session.Close()
	}

	// delete session from manager
	t.manager.Delete(session)
	delete(t.ipToSession, session.InternalAddr())
	delete(t.externalToSession, session.ExternalAddrPort())
}

// GetByInternalAddrPort gets a session by internal address and resets its TTL.
func (t *TTLManager[cs]) GetByInternalAddrPort(addr netip.Addr) (cs, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if entry, ok := t.ipToSession[addr]; ok {
		entry.expire = time.Now().Add(t.sessionTtl)
		t.ipToSession[addr] = entry
		t.externalToSession[entry.session.ExternalAddrPort()] = entry
		return entry.session, nil
	}

	var zero cs
	session, sessionErr := t.manager.GetByInternalAddrPort(addr)
	if sessionErr != nil {
		return zero, sessionErr
	}
	entry := sessionWithTTL[cs]{
		expire:  time.Now().Add(t.sessionTtl),
		session: session,
	}
	t.ipToSession[session.InternalAddr()] = entry
	t.externalToSession[session.ExternalAddrPort()] = entry
	return session, nil
}

// GetByExternalAddrPort gets a session by external address and resets its TTL.
func (t *TTLManager[cs]) GetByExternalAddrPort(addrPort netip.AddrPort) (cs, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if entry, ok := t.externalToSession[addrPort]; ok {
		entry.expire = time.Now().Add(t.sessionTtl)
		t.externalToSession[addrPort] = entry
		t.ipToSession[entry.session.InternalAddr()] = entry
		return entry.session, nil
	}

	var zero cs
	session, sessionErr := t.manager.GetByExternalAddrPort(addrPort)
	if sessionErr != nil {
		return zero, sessionErr
	}
	entry := sessionWithTTL[cs]{
		expire:  time.Now().Add(t.sessionTtl),
		session: session,
	}
	t.ipToSession[session.InternalAddr()] = entry
	t.externalToSession[session.ExternalAddrPort()] = entry
	return session, nil
}

// sanitize periodically removes expired sessions from both maps.
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
			now := time.Now()
			expired := make([]cs, 0)
			t.mu.Lock()
			for ip, entry := range t.ipToSession {
				if now.After(entry.expire) {
					_ = entry.session.Close()
					expired = append(expired, entry.session)
					delete(t.ipToSession, ip)
					delete(t.externalToSession, entry.session.ExternalAddrPort())
				}
			}
			t.mu.Unlock()

			for _, session := range expired {
				t.manager.Delete(session)
			}
		}
	}
}
