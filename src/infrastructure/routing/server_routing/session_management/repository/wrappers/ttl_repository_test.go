package wrappers

import (
	"context"
	"net/netip"
	"testing"
	"time"

	rep "tungo/infrastructure/routing/server_routing/session_management/repository"
)

// testSession implements the SessionContract and is comparable.
// InternalAddr and ExternalAddrPort identify the session.
type testSession struct {
	internal netip.Addr
	external netip.AddrPort
}

func (s testSession) InternalAddr() netip.Addr         { return s.internal }
func (s testSession) ExternalAddrPort() netip.AddrPort { return s.external }

func TestTTLRepository_NoPrematureDeletion(t *testing.T) {
	// Underlying manager
	defaultMgr := rep.NewDefaultWorkerSessionManager[testSession]()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Configure TTL shorter than cleanup interval for deterministic expiration
	exp := 10 * time.Millisecond
	interval := 30 * time.Millisecond
	ttlRepo := NewTTLRepository[testSession](ctx, defaultMgr, exp, interval)

	// Create and add session
	sess := testSession{
		internal: netip.MustParseAddr("10.0.0.1"),
		external: netip.MustParseAddrPort("1.2.3.4:1234"),
	}
	ttlRepo.Add(sess)

	// Immediately, session must be present
	if got, err := ttlRepo.GetByInternalAddrPort(sess.internal); err != nil || got != sess {
		t.Fatalf("session should exist immediately, got %v, err %v", got, err)
	}

	// Wait less than TTL so it is still active
	time.Sleep(exp / 2)
	if got, err := ttlRepo.GetByInternalAddrPort(sess.internal); err != nil || got != sess {
		t.Fatalf("session should still exist before expiration, got %v, err %v", got, err)
	}

	// Wait for TTL to expire and for cleanup to run
	time.Sleep(exp + interval)

	// Session should be removed from TTL repository
	if _, err := ttlRepo.GetByInternalAddrPort(sess.internal); err != rep.ErrSessionNotFound {
		t.Fatalf("expected session removed from TTL repo, got err %v", err)
	}

	// Underlying manager must also have deleted the session
	if _, err := defaultMgr.GetByInternalAddrPort(sess.internal); err != rep.ErrSessionNotFound {
		t.Fatalf("expected session removed from underlying manager, got err %v", err)
	}
}
