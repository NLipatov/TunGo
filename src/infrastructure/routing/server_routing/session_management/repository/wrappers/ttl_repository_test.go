package wrappers

import (
	"context"
	"fmt"
	"net/netip"
	"sync"
	"testing"
	"time"
	"tungo/infrastructure/routing/server_routing/session_management/repository"
)

// === Test timing configuration ===
const (
	testTTL     = 2 * time.Millisecond
	testCleanup = 1 * time.Millisecond
	testWait    = 4 * time.Millisecond // testWait > testTTL+testCleanup
)

// TestSession is a dummy implementation of SessionContract for testing purposes.
type TestSession struct {
	internal netip.Addr
	external netip.AddrPort
	closed   *bool
}

// InternalAddr returns the internal address of the session.
func (s TestSession) InternalAddr() netip.Addr { return s.internal }

// ExternalAddrPort returns the external address and port of the session.
func (s TestSession) ExternalAddrPort() netip.AddrPort { return s.external }

// Close marks the session as closed (for test verification).
func (s TestSession) Close() error {
	if s.closed != nil {
		*s.closed = true
	}
	return nil
}

// FakeManager is a mock implementation of SessionRepository for unit tests.
type FakeManager struct {
	mu         sync.Mutex
	added      []TestSession
	deleted    []TestSession
	byInternal map[netip.Addr]TestSession
	byExternal map[netip.AddrPort]TestSession
}

// NewFakeManager creates and returns a new FakeManager.
func NewFakeManager() *FakeManager {
	return &FakeManager{
		byInternal: make(map[netip.Addr]TestSession),
		byExternal: make(map[netip.AddrPort]TestSession),
	}
}

// Add adds a session to the fake manager (for tracking and lookup).
func (f *FakeManager) Add(s TestSession) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.added = append(f.added, s)
	f.byInternal[s.internal] = s
	f.byExternal[s.external] = s
	fmt.Printf("[FakeManager] Add %v\n", s.InternalAddr())
}

// Delete removes a session from the fake manager (for tracking and lookup).
func (f *FakeManager) Delete(s TestSession) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, s)
	delete(f.byInternal, s.internal)
	delete(f.byExternal, s.external)
	fmt.Printf("[FakeManager] Delete %v\n", s.InternalAddr())
}

// GetByInternalAddrPort retrieves a session by its internal address.
func (f *FakeManager) GetByInternalAddrPort(ip netip.Addr) (TestSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.byInternal[ip]
	if !ok {
		return TestSession{}, repository.ErrSessionNotFound
	}
	return s, nil
}

// GetByExternalAddrPort retrieves a session by its external address and port.
func (f *FakeManager) GetByExternalAddrPort(ip netip.AddrPort) (TestSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.byExternal[ip]
	if !ok {
		return TestSession{}, repository.ErrSessionNotFound
	}
	return s, nil
}

// TestAddAndGetResetsTTL verifies that session is added and TTL is reset on each Get.
func TestAddAndGetResetsTTL(t *testing.T) {
	fake := NewFakeManager()
	m := NewTTLManager[TestSession](context.Background(), fake, testTTL, testCleanup)

	in, _ := netip.ParseAddr("1.2.3.4")
	ex, _ := netip.ParseAddrPort("4.3.2.1:9000")
	s := TestSession{internal: in, external: ex}
	m.Add(s)

	s2, err := m.GetByInternalAddrPort(s.internal)
	if err != nil || s2 != s {
		t.Fatalf("GetByInternalAddrPort: expected %v, got %v, err=%v", s, s2, err)
	}
	s3, err := m.GetByExternalAddrPort(s.external)
	if err != nil || s3 != s {
		t.Fatalf("GetByExternalAddrPort: expected %v, got %v, err=%v", s, s3, err)
	}
}

// TestAdd_OverwriteSession checks that adding a session with same internal address deletes the old session.
func TestAdd_OverwriteSession(t *testing.T) {
	fake := NewFakeManager()
	m := NewTTLManager[TestSession](context.Background(), fake, testTTL, testCleanup)
	in, _ := netip.ParseAddr("5.5.5.5")
	ex1, _ := netip.ParseAddrPort("6.6.6.6:9000")
	ex2, _ := netip.ParseAddrPort("7.7.7.7:9000")
	s1 := TestSession{internal: in, external: ex1}
	s2 := TestSession{internal: in, external: ex2}

	m.Add(s1)
	m.Add(s2) // overwrite

	fake.mu.Lock()
	defer fake.mu.Unlock()
	found := false
	for _, d := range fake.deleted {
		if d == s1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected s1 deleted on overwrite, got %v", fake.deleted)
	}
}

// TestManualDelete checks that sessions can be deleted manually and double delete is a no-op.
func TestManualDelete(t *testing.T) {
	fake := NewFakeManager()
	m := NewTTLManager[TestSession](context.Background(), fake, testTTL, time.Hour)
	in, _ := netip.ParseAddr("9.9.9.9")
	ex, _ := netip.ParseAddrPort("8.8.8.8:9000")
	closed := false
	s := TestSession{internal: in, external: ex, closed: &closed}
	m.Add(s)
	m.Delete(s)
	// double delete should be allowed
	m.Delete(s)
	fake.mu.Lock()
	defer fake.mu.Unlock()

	if len(fake.deleted) < 1 || fake.deleted[0] != s {
		t.Fatalf("expected Delete call with %v, got %v", s, fake.deleted)
	}
	if !closed {
		t.Fatalf("expected Close() to be called on session deletion")
	}
}

// TestDelete_NotExistingSession checks that deleting a non-existing session is safe.
func TestDelete_NotExistingSession(t *testing.T) {
	fake := NewFakeManager()
	m := NewTTLManager[TestSession](context.Background(), fake, testTTL, testCleanup)
	in, _ := netip.ParseAddr("11.11.11.11")
	ex, _ := netip.ParseAddrPort("22.22.22.22:9000")
	s := TestSession{internal: in, external: ex}
	m.Delete(s)
}

// TestSanitizeWithExpiration checks that a session is deleted after TTL expiration and Close() is called.
func TestSanitizeWithExpiration(t *testing.T) {
	fake := NewFakeManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	closed := false
	m := NewTTLManager[TestSession](ctx, fake, testTTL, testCleanup)
	in, _ := netip.ParseAddr("1.1.1.1")
	ex, _ := netip.ParseAddrPort("2.2.2.2:9000")
	s := TestSession{internal: in, external: ex, closed: &closed}
	m.Add(s)

	time.Sleep(testWait)
	fake.mu.Lock()
	defer fake.mu.Unlock()
	foundDeleted := false
	for _, d := range fake.deleted {
		if d == s {
			foundDeleted = true
			break
		}
	}
	if !foundDeleted {
		t.Errorf("expected session to be deleted by sanitize after expiration")
	}
	if !closed {
		t.Fatalf("expected Close() to be called by sanitize")
	}
}

// TestSanitizeStopsOnContextCancel ensures that sanitize goroutine stops after context is cancelled.
func TestSanitizeStopsOnContextCancel(t *testing.T) {
	fake := NewFakeManager()
	ctx, cancel := context.WithCancel(context.Background())
	mIface := NewTTLManager[TestSession](ctx, fake, testTTL, testCleanup)
	_, ok := mIface.(*TTLManager[TestSession])
	if !ok {
		t.Fatal("failed to cast SessionRepository to *TTLManager")
	}
	cancel()
	time.Sleep(testWait)
	// No assertion: just check for deadlock/panic
}

// TestDoubleDelete checks that deleting a session twice does not panic or error.
func TestDoubleDelete(t *testing.T) {
	fake := NewFakeManager()
	m := NewTTLManager[TestSession](context.Background(), fake, testTTL, testCleanup)
	in, _ := netip.ParseAddr("101.101.101.101")
	ex, _ := netip.ParseAddrPort("202.202.202.202:9000")
	s := TestSession{internal: in, external: ex}
	m.Add(s)
	m.Delete(s)
	m.Delete(s)
}

// TestNATRebinding_ReplacesSessionAndTTL tests that NAT rebinding is handled: old session is deleted and only the new one expires by TTL.
func TestNATRebinding_ReplacesSessionAndTTL(t *testing.T) {
	fake := NewFakeManager()
	m := NewTTLManager[TestSession](context.Background(), fake, testTTL, testCleanup)

	in, _ := netip.ParseAddr("42.42.42.42")
	ex1, _ := netip.ParseAddrPort("1.1.1.1:1")
	ex2, _ := netip.ParseAddrPort("2.2.2.2:2")
	s1 := TestSession{internal: in, external: ex1}
	s2 := TestSession{internal: in, external: ex2}

	m.Add(s1)
	m.Add(s2) // NAT rebinding: new session, same IP, different external

	fake.mu.Lock()
	var deletedS1 bool
	for _, d := range fake.deleted {
		if d == s1 {
			deletedS1 = true
		}
	}
	fake.mu.Unlock()
	if !deletedS1 {
		t.Fatalf("NAT rebinding: s1 (old) should be deleted on Add(s2), got deleted: %v", fake.deleted)
	}

	// Wait for TTL expiration (only s2 should expire, not s1)
	time.Sleep(testWait)
	fake.mu.Lock()
	var deletedS2 bool
	for _, d := range fake.deleted {
		if d == s2 {
			deletedS2 = true
		}
	}
	fake.mu.Unlock()
	if !deletedS2 {
		t.Errorf("NAT rebinding: s2 (new) should be deleted by TTL expiration, got deleted: %v", fake.deleted)
	}
}

// TestSessionExpiresAfterTTL checks that a session is deleted after its TTL expires and Close() is called.
func TestSessionExpiresAfterTTL(t *testing.T) {
	fake := NewFakeManager()
	closed := false
	m := NewTTLManager[TestSession](context.Background(), fake, testTTL, testCleanup)

	in, _ := netip.ParseAddr("1.1.1.1")
	ex, _ := netip.ParseAddrPort("2.2.2.2:9000")
	s := TestSession{internal: in, external: ex, closed: &closed}
	m.Add(s)

	time.Sleep(testWait)
	fake.mu.Lock()
	var deleted bool
	for _, d := range fake.deleted {
		if d == s {
			deleted = true
			break
		}
	}
	fake.mu.Unlock()
	if !deleted {
		t.Fatalf("expected session to be deleted by TTL expiration")
	}
	if !closed {
		t.Fatalf("expected Close() to be called on session TTL expiration")
	}
}

// TestSessionNotDeletedWhenAccessed checks that session is not deleted if accessed before TTL expiration.
func TestSessionNotDeletedWhenAccessed(t *testing.T) {
	fake := NewFakeManager()
	ttl := testTTL
	cleanup := testCleanup
	mIface := NewTTLManager[TestSession](context.Background(), fake, ttl, cleanup)

	// Cast interface to TTLManager struct
	m, ok := mIface.(*TTLManager[TestSession])
	if !ok {
		t.Fatal("failed to cast SessionRepository to *TTLManager")
	}

	in, _ := netip.ParseAddr("3.3.3.3")
	ex, _ := netip.ParseAddrPort("4.4.4.4:9000")
	s := TestSession{internal: in, external: ex}

	// Add session to FakeManager directly
	fake.Add(s)

	// Also add it to TTLManager's maps
	m.mu.Lock()
	m.ipToSession[s.internal] = sessionWithTTL[TestSession]{session: s, expire: time.Now().Add(ttl)}
	m.mu.Unlock()

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_, err := m.GetByInternalAddrPort(s.internal)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				time.Sleep(ttl / 3)
			}
		}
	}()

	time.Sleep(4 * ttl)
	close(stop)
	wg.Wait()

	fake.mu.Lock()
	defer fake.mu.Unlock()
	var deleted bool
	for _, d := range fake.deleted {
		if d == s {
			deleted = true
			break
		}
	}
	if deleted {
		t.Fatalf("session should not have been deleted, but was: %v", s)
	}
}

func TestNewTTLManager_Defaults(t *testing.T) {
	fake := NewFakeManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create TTLManager with 0-valued TTL and cleanupInterval
	mIface := NewTTLManager[TestSession](ctx, fake, 0, 0)
	m, ok := mIface.(*TTLManager[TestSession])
	if !ok {
		t.Fatalf("failed to cast SessionRepository to *TTLManager")
	}

	if m.sessionTtl != 0 || m.cleanupInterval != 0 {
		t.Fatalf("TTLManager should have 0 TTL/cleanupInterval initially")
	}

	in, _ := netip.ParseAddr("10.0.0.1")
	ex, _ := netip.ParseAddrPort("20.0.0.1:9000")
	s := TestSession{internal: in, external: ex}
	m.Add(s)
}

func TestAdd_CallsCloseOnOverwrite(t *testing.T) {
	fake := NewFakeManager()
	m := NewTTLManager[TestSession](context.Background(), fake, testTTL, testCleanup)
	in, _ := netip.ParseAddr("50.50.50.50")
	ex1, _ := netip.ParseAddrPort("60.60.60.60:9000")
	ex2, _ := netip.ParseAddrPort("70.70.70.70:9000")
	closed1 := false
	s1 := TestSession{internal: in, external: ex1, closed: &closed1}
	s2 := TestSession{internal: in, external: ex2}

	m.Add(s1)
	m.Add(s2)

	if !closed1 {
		t.Fatalf("Close() should be called on s1 when overwritten by Add(s2)")
	}
}

func TestDelete_DoesNotCallCloseIfSessionNotInMap(t *testing.T) {
	fake := NewFakeManager()
	m := NewTTLManager[TestSession](context.Background(), fake, testTTL, testCleanup)

	closed := false
	in, _ := netip.ParseAddr("12.34.56.78")
	ex, _ := netip.ParseAddrPort("87.65.43.21:9000")
	s := TestSession{internal: in, external: ex, closed: &closed}

	m.Delete(s)

	if closed {
		t.Fatalf("Close() should NOT be called if session not in ipToSession")
	}
}
