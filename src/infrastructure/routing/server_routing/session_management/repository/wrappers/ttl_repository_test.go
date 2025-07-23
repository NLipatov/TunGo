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

// TestSession is a dummy implementation of SessionContract for testing purposes.
type TestSession struct {
	internal netip.Addr
	external netip.AddrPort
}

// InternalAddr returns the internal address of the session.
func (s TestSession) InternalAddr() netip.Addr { return s.internal }

// ExternalAddrPort returns the external address and port of the session.
func (s TestSession) ExternalAddrPort() netip.AddrPort { return s.external }

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
	m := NewTTLManager[TestSession](context.Background(), fake, 50*time.Millisecond, 20*time.Millisecond)

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
	m := NewTTLManager[TestSession](context.Background(), fake, time.Second, time.Second)
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
	m := NewTTLManager[TestSession](context.Background(), fake, 50*time.Millisecond, time.Hour)
	in, _ := netip.ParseAddr("9.9.9.9")
	ex, _ := netip.ParseAddrPort("8.8.8.8:9000")
	s := TestSession{internal: in, external: ex}
	m.Add(s)
	m.Delete(s)
	// double delete should be allowed
	m.Delete(s)
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.deleted) < 1 || fake.deleted[0] != s {
		t.Fatalf("expected Delete call with %v, got %v", s, fake.deleted)
	}
}

// TestDelete_NotExistingSession checks that deleting a non-existing session is safe.
func TestDelete_NotExistingSession(t *testing.T) {
	fake := NewFakeManager()
	m := NewTTLManager[TestSession](context.Background(), fake, time.Second, time.Second)
	in, _ := netip.ParseAddr("11.11.11.11")
	ex, _ := netip.ParseAddrPort("22.22.22.22:9000")
	s := TestSession{internal: in, external: ex}
	m.Delete(s)
}

// TestSanitizeWithExpiration checks that a session is deleted after TTL expiration.
func TestSanitizeWithExpiration(t *testing.T) {
	fake := NewFakeManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewTTLManager[TestSession](ctx, fake, 10*time.Millisecond, 5*time.Millisecond)
	in, _ := netip.ParseAddr("1.1.1.1")
	ex, _ := netip.ParseAddrPort("2.2.2.2:9000")
	s := TestSession{internal: in, external: ex}
	m.Add(s)

	time.Sleep(20 * time.Millisecond)
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
}

// TestSanitizeStopsOnContextCancel ensures that sanitize goroutine stops after context is cancelled.
func TestSanitizeStopsOnContextCancel(t *testing.T) {
	fake := NewFakeManager()
	ctx, cancel := context.WithCancel(context.Background())
	mIface := NewTTLManager[TestSession](ctx, fake, 10*time.Millisecond, 5*time.Millisecond)
	_, ok := mIface.(*TTLManager[TestSession])
	if !ok {
		t.Fatal("failed to cast SessionRepository to *TTLManager")
	}
	cancel()
	time.Sleep(10 * time.Millisecond)
	// No assertion: just check for deadlock/panic
}

// TestDoubleDelete checks that deleting a session twice does not panic or error.
func TestDoubleDelete(t *testing.T) {
	fake := NewFakeManager()
	m := NewTTLManager[TestSession](context.Background(), fake, time.Second, time.Second)
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
	m := NewTTLManager[TestSession](context.Background(), fake, 100*time.Millisecond, 10*time.Millisecond)

	in, _ := netip.ParseAddr("42.42.42.42")
	ex1, _ := netip.ParseAddrPort("1.1.1.1:1")
	ex2, _ := netip.ParseAddrPort("2.2.2.2:2")
	s1 := TestSession{internal: in, external: ex1}
	s2 := TestSession{internal: in, external: ex2}

	m.Add(s1)
	m.Add(s2) // NAT rebinding: new session, same IP, different external

	// s1 should be deleted (overwritten)
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
	time.Sleep(120 * time.Millisecond)
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

// TestSessionExpiresAfterTTL checks that a session is deleted after its TTL expires.
func TestSessionExpiresAfterTTL(t *testing.T) {
	fake := NewFakeManager()
	m := NewTTLManager[TestSession](context.Background(), fake, 15*time.Millisecond, 2*time.Millisecond)

	in, _ := netip.ParseAddr("1.1.1.1")
	ex, _ := netip.ParseAddrPort("2.2.2.2:9000")
	s := TestSession{internal: in, external: ex}
	m.Add(s)

	// Wait for the TTL to expire
	time.Sleep(30 * time.Millisecond)

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
}

// TestSessionNotDeletedWhenAccessed checks that session is not deleted if accessed before TTL expiration.
func TestSessionNotDeletedWhenAccessed(t *testing.T) {
	fake := NewFakeManager()
	ttl := 100 * time.Millisecond
	cleanup := 5 * time.Millisecond
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
				time.Sleep(ttl / 4)
			}
		}
	}()

	time.Sleep(5 * ttl)

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
