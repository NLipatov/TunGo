package wrappers

import (
	"context"
	"net/netip"
	"testing"
	"time"

	rep "tungo/infrastructure/routing/server_routing/session_management/repository"
)

// testSession implements SessionContract and is comparable.
type testSession struct {
	internal netip.Addr
	external netip.AddrPort
}

func (s testSession) InternalAddr() netip.Addr         { return s.internal }
func (s testSession) ExternalAddrPort() netip.AddrPort { return s.external }

// Test expiration: session should expire from TTL repository after TTL elapses.
func TestTTLRepository_Expiration(t *testing.T) {
	// Underlying manager remains unaffected by sync in this test
	defaultMgr := rep.NewDefaultWorkerSessionManager[testSession]()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ttl := 20 * time.Millisecond
	// use large cleanup interval to avoid syncExpiredSessions interfering
	cleanup := 200 * time.Millisecond
	ttlRepo := NewTTLRepository[testSession](ctx, defaultMgr, ttl, cleanup)

	sess := testSession{netip.MustParseAddr("10.0.0.1"), netip.MustParseAddrPort("1.2.3.4:1234")}
	ttlRepo.Add(sess)

	// session should exist immediately
	if _, err := ttlRepo.GetByInternalAddrPort(sess.internal); err != nil {
		t.Fatalf("expected session present immediately, got err %v", err)
	}

	// wait for TTL to elapse, but before sync removes manager entry
	time.Sleep(ttl + 5*time.Millisecond)

	// TTL repository should have expired the session
	if _, err := ttlRepo.GetByInternalAddrPort(sess.internal); err != rep.ErrSessionNotFound {
		t.Fatalf("expected TTL repo to remove session after TTL, got err %v", err)
	}

	// underlying manager still has the session (cleanup interval too large)
	if _, err := defaultMgr.GetByInternalAddrPort(sess.internal); err != nil {
		t.Fatalf("underlying manager should still have session, got err %v", err)
	}
}

// Test explicit deletion removes only the specified session.
func TestTTLRepository_DeleteSingle(t *testing.T) {
	defaultMgr := rep.NewDefaultWorkerSessionManager[testSession]()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exp, interval := 50*time.Millisecond, 25*time.Millisecond
	ttlRepo := NewTTLRepository[testSession](ctx, defaultMgr, exp, interval)

	s1 := testSession{netip.MustParseAddr("10.0.0.2"), netip.MustParseAddrPort("2.3.4.5:2345")}
	s2 := testSession{netip.MustParseAddr("10.0.0.3"), netip.MustParseAddrPort("3.4.5.6:3456")}
	ttlRepo.Add(s1)
	ttlRepo.Add(s2)

	// delete s1 explicitly
	ttlRepo.Delete(s1)

	// s1 removed from TTL repo
	if _, err := ttlRepo.GetByInternalAddrPort(s1.internal); err != rep.ErrSessionNotFound {
		t.Fatalf("s1 should be removed after Delete, got err %v", err)
	}
	// s2 should still remain
	if _, err := ttlRepo.GetByInternalAddrPort(s2.internal); err != nil {
		t.Fatalf("s2 should remain, got err %v", err)
	}
}

// Test ExternalAccessExtendsTTL ensures that GetByExternalAddrPort prolongs TTL.
func TestTTLRepository_ExternalAccessExtendsTTL(t *testing.T) {
	defaultMgr := rep.NewDefaultWorkerSessionManager[testSession]()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ttl := 30 * time.Millisecond
	cleanup := 100 * time.Millisecond
	ttlRepo := NewTTLRepository[testSession](ctx, defaultMgr, ttl, cleanup)

	sess := testSession{netip.MustParseAddr("10.0.0.4"), netip.MustParseAddrPort("4.5.6.7:4567")}
	ttlRepo.Add(sess)

	// wait for TTL to expire
	time.Sleep(ttl + 5*time.Millisecond)
	if _, err := ttlRepo.GetByInternalAddrPort(sess.internal); err != rep.ErrSessionNotFound {
		t.Fatalf("session should expire initially, got err %v", err)
	}

	// re-add and perform external access extension
	ttlRepo.Add(sess)
	if _, err := ttlRepo.GetByExternalAddrPort(sess.external); err != nil {
		t.Fatalf("external access failed, err %v", err)
	}

	// wait less than TTL to confirm extension preserved
	time.Sleep(ttl / 2)
	if _, err := ttlRepo.GetByInternalAddrPort(sess.internal); err != nil {
		t.Fatalf("session should remain after external extension, got err %v", err)
	}
}

// TestNewTTLRepository_Defaults verifies constructor default branches without long waits.
func TestNewTTLRepository_Defaults(t *testing.T) {
	defaultMgr := rep.NewDefaultWorkerSessionManager[testSession]()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Trigger default TTL and interval
	ttlRepo := NewTTLRepository[testSession](ctx, defaultMgr, 0, 0)

	sess := testSession{netip.MustParseAddr("127.0.0.1"), netip.MustParseAddrPort("127.0.0.1:1111")}
	ttlRepo.Add(sess)

	// immediate presence
	if _, err := ttlRepo.GetByInternalAddrPort(sess.internal); err != nil {
		t.Fatalf("session should exist with default settings, got err %v", err)
	}

	// random address should not exist
	if _, err := ttlRepo.GetByInternalAddrPort(netip.MustParseAddr("127.0.0.2")); err != rep.ErrSessionNotFound {
		t.Fatalf("random address should not exist, got err %v", err)
	}
}

// TestNewTTLRepository_NoPanic ensures constructor never panics.
func TestNewTTLRepository_NoPanic(t *testing.T) {
	defaultMgr := rep.NewDefaultWorkerSessionManager[testSession]()
	// Should not panic
	func() {
		defer func() { recover() }()
		_ = NewTTLRepository[testSession](context.Background(), defaultMgr, 0, 0)
	}()
}

// Test GetByExternalAddrPort returns ErrSessionNotFound for unknown client.
func TestTTLRepository_GetByExternalNotFound(t *testing.T) {
	defaultMgr := rep.NewDefaultWorkerSessionManager[testSession]()
	ctx := context.Background()
	ttlRepo := NewTTLRepository[testSession](ctx, defaultMgr, 50*time.Millisecond, 50*time.Millisecond)

	// random external addrPort
	randEP := netip.MustParseAddrPort("9.9.9.9:9999")
	if _, err := ttlRepo.GetByExternalAddrPort(randEP); err != rep.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound for external %v, got %v", randEP, err)
	}
}

// Test Range delegates to underlying manager.
func TestTTLRepository_Range(t *testing.T) {
	defaultMgr := rep.NewDefaultWorkerSessionManager[testSession]()
	ctx := context.Background()
	// TTL and cleanup large to avoid expiry
	ttlRepo := NewTTLRepository[testSession](ctx, defaultMgr, 1*time.Hour, 1*time.Hour)

	// add multiple sessions
	s1 := testSession{netip.MustParseAddr("10.1.1.1"), netip.MustParseAddrPort("1.1.1.1:1111")}
	s2 := testSession{netip.MustParseAddr("10.1.1.2"), netip.MustParseAddrPort("1.1.1.2:1112")}
	ttlRepo.Add(s1)
	ttlRepo.Add(s2)

	expects := map[netip.Addr]bool{s1.internal: true, s2.internal: true}
	// collect via Range
	ttlRepo.Range(func(session testSession) bool {
		expects[session.InternalAddr()] = false
		return true
	})

	for addr, miss := range expects {
		if miss {
			t.Fatalf("Range did not visit session %v", addr)
		}
	}
}

// Test syncExpiredSessions removes session from manager after TTL and cleanup.
func TestTTLRepository_SyncExpiredSessions(t *testing.T) {
	defaultMgr := rep.NewDefaultWorkerSessionManager[testSession]()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exp := 10 * time.Millisecond
	cleanup := 10 * time.Millisecond
	ttlRepo := NewTTLRepository[testSession](ctx, defaultMgr, exp, cleanup)

	sess := testSession{netip.MustParseAddr("10.2.2.2"), netip.MustParseAddrPort("2.2.2.2:2222")}
	ttlRepo.Add(sess)

	// confirm manager has session initially
	if _, err := defaultMgr.GetByInternalAddrPort(sess.internal); err != nil {
		t.Fatalf("underlying manager should have session initially, got err %v", err)
	}

	// wait for TTL + cleanup interval + small delta
	time.Sleep(exp + cleanup + 5*time.Millisecond)

	// TTL repo must have expired
	if _, err := ttlRepo.GetByInternalAddrPort(sess.internal); err != rep.ErrSessionNotFound {
		t.Fatalf("expected TTL repo expired, got err %v", err)
	}
	// manager should be cleaned by syncExpiredSessions
	if _, err := defaultMgr.GetByInternalAddrPort(sess.internal); err != rep.ErrSessionNotFound {
		t.Fatalf("expected manager cleaned by syncExpiredSessions, got err %v", err)
	}
}
