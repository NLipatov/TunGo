package wrappers

import (
	"context"
	"net/netip"
	"time"
	"tungo/infrastructure/PAL/server_configuration"
	"tungo/infrastructure/routing/server_routing/session_management"
	"tungo/infrastructure/routing/server_routing/session_management/repository"

	"github.com/NLipatov/goutils/maps"
)

type TTLRepository[cs interface {
	session_management.SessionContract
	comparable
}] struct {
	ctx     context.Context
	manager repository.SessionRepository[cs]
	ttl     *maps.TtlTypedSyncMap[netip.Addr, cs]
}

func NewTTLRepository[cs interface {
	session_management.SessionContract
	comparable
}](
	ctx context.Context,
	manager repository.SessionRepository[cs],
	expDuration, sanitizeInterval time.Duration,
) repository.SessionRepository[cs] {
	if expDuration <= 0 {
		expDuration = time.Duration(server_configuration.DefaultSessionTtl)
	}
	if sanitizeInterval <= 0 {
		sanitizeInterval = time.Duration(server_configuration.DefaultSessionCleanupInterval)
	}

	ttlMgr := &TTLRepository[cs]{
		ctx:     ctx,
		manager: manager,
		ttl:     maps.NewTtlTypedSyncMap[netip.Addr, cs](ctx, expDuration, sanitizeInterval),
	}

	go ttlMgr.syncExpiredSessions(sanitizeInterval)

	return ttlMgr
}

func (t *TTLRepository[cs]) Add(session cs) {
	t.manager.Add(session)
	t.ttl.Store(session.InternalAddr(), session)
}

func (t *TTLRepository[cs]) Delete(session cs) {
	t.manager.Delete(session)
	t.ttl.Delete(session.InternalAddr())
}

func (t *TTLRepository[cs]) GetByInternalAddrPort(addr netip.Addr) (cs, error) {
	session, ok := t.ttl.Load(addr)
	if !ok {
		var zero cs
		return zero, repository.ErrSessionNotFound
	}
	return session, nil
}

func (t *TTLRepository[cs]) GetByExternalAddrPort(addrPort netip.AddrPort) (cs, error) {
	session, err := t.manager.GetByExternalAddrPort(addrPort)
	if err != nil {
		var zero cs
		return zero, repository.ErrSessionNotFound
	}

	// extend TTL upon external access
	t.ttl.Store(session.InternalAddr(), session)
	return session, nil
}

func (c *TTLRepository[cs]) Range(f func(session cs) bool) {
	c.manager.Range(f)
}

// syncExpiredSessions sync expired session removal from manager
func (t *TTLRepository[cs]) syncExpiredSessions(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			var activeSessions = make(map[netip.Addr]struct{})

			t.ttl.Range(func(key netip.Addr, _ cs) bool {
				activeSessions[key] = struct{}{}
				return true
			})

			t.removeExpiredSessionsFromManager(activeSessions)
		}
	}
}

// removeExpiredSessionsFromManager removes sessions from the manager that are no longer present in the TTL map
func (t *TTLRepository[cs]) removeExpiredSessionsFromManager(active map[netip.Addr]struct{}) {
	var toDelete []cs

	// collect sessions not present in the TTL map
	t.manager.Range(func(session cs) bool {
		if _, exists := active[session.InternalAddr()]; !exists {
			toDelete = append(toDelete, session)
		}
		return true
	})

	// delete expired sessions from the manager
	for _, session := range toDelete {
		t.manager.Delete(session)
	}
}
