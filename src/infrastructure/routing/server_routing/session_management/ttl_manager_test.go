package session_management

import (
	"context"
	"net/netip"
	"sync"
	"testing"
	"time"
)

// TestSession is a simple type implementing ClientSession and comparable.
type TestSession struct {
	internal netip.Addr
	external netip.AddrPort
}

func (s TestSession) InternalIP() netip.Addr     { return s.internal }
func (s TestSession) ExternalIP() netip.AddrPort { return s.external }

// FakeManager is a stub for WorkerSessionManager[TestSession].
type FakeManager struct {
	mu         sync.Mutex
	added      []TestSession
	deleted    []TestSession
	byInternal map[netip.Addr]TestSession
	byExternal map[netip.AddrPort]TestSession
}

func NewFakeManager() *FakeManager {
	return &FakeManager{
		byInternal: make(map[netip.Addr]TestSession),
		byExternal: make(map[netip.AddrPort]TestSession),
	}
}

func (f *FakeManager) Add(s TestSession) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.added = append(f.added, s)
	f.byInternal[s.internal] = s
	f.byExternal[s.external] = s
}

func (f *FakeManager) Delete(s TestSession) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, s)
	delete(f.byInternal, s.internal)
	delete(f.byExternal, s.external)
}

func (f *FakeManager) GetByInternalIP(ip netip.Addr) (TestSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.byInternal[ip]
	if !ok {
		return TestSession{}, ErrSessionNotFound
	}
	return s, nil
}

func (f *FakeManager) GetByExternalIP(ip netip.AddrPort) (TestSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.byExternal[ip]
	if !ok {
		return TestSession{}, ErrSessionNotFound
	}
	return s, nil
}

func TestAddAndGetResetsTTL(t *testing.T) {
	fake := NewFakeManager()
	m := NewTTLManager[TestSession](context.Background(), fake, 50*time.Millisecond, 20*time.Millisecond)

	in, _ := netip.ParseAddr("1.2.3.4")
	ex, _ := netip.ParseAddrPort("4.3.2.1:9000")
	s := TestSession{internal: in, external: ex}
	m.Add(s)

	if len(fake.added) != 1 || fake.added[0] != s {
		t.Fatalf("expected Add call with %v, got %v", s, fake.added)
	}

	s2, err := m.GetByInternalIP(s.internal)
	if err != nil {
		t.Fatalf("GetByInternalIP returned error: %v", err)
	}
	if s2 != s {
		t.Fatalf("GetByInternalIP: expected %v, got %v", s, s2)
	}

	s3, err := m.GetByExternalIP(s.external)
	if err != nil {
		t.Fatalf("GetByExternalIP returned error: %v", err)
	}
	if s3 != s {
		t.Fatalf("GetByExternalIP: expected %v, got %v", s, s3)
	}
}

func TestExpirationAndSanitize(t *testing.T) {
	fake := NewFakeManager()
	m := NewTTLManager[TestSession](context.Background(), fake, 10*time.Millisecond, time.Millisecond)

	s1In, _ := netip.ParseAddr("1.1.1.1")
	s1Ex, _ := netip.ParseAddrPort("2.2.2.2:9000")
	s1 := TestSession{internal: s1In, external: s1Ex}

	s2In, _ := netip.ParseAddr("3.3.3.3")
	s2Ex, _ := netip.ParseAddrPort("4.4.4.4:9000")
	s2 := TestSession{internal: s2In, external: s2Ex}
	m.Add(s1)
	m.Add(s2)

	time.Sleep(7 * time.Millisecond)
	_, getByExternalIPErr := m.GetByExternalIP(s2.external)
	if getByExternalIPErr != nil {
		t.Fatal(getByExternalIPErr)
	}
	time.Sleep(5 * time.Millisecond)

	fake.mu.Lock()
	deleted := append([]TestSession(nil), fake.deleted...)
	fake.mu.Unlock()

	var saw1, saw2 bool
	for _, d := range deleted {
		if d == s1 {
			saw1 = true
		}
		if d == s2 {
			saw2 = true
		}
	}
	if !saw1 {
		t.Errorf("expected session %v to be deleted, deleted list: %v", s1, deleted)
	}
	if saw2 {
		t.Errorf("did not expect session %v to be deleted yet", s2)
	}
}

func TestManualDelete(t *testing.T) {
	fake := NewFakeManager()
	m := NewTTLManager[TestSession](context.Background(), fake, 50*time.Millisecond, time.Hour)

	in, _ := netip.ParseAddr("9.9.9.9")
	ex, _ := netip.ParseAddrPort("8.8.8.8:9000")
	s := TestSession{internal: in, external: ex}
	m.Add(s)
	m.Delete(s)

	fake.mu.Lock()
	if len(fake.deleted) != 1 || fake.deleted[0] != s {
		fake.mu.Unlock()
		t.Fatalf("expected manual Delete call with %v, got %v", s, fake.deleted)
	}
	fake.mu.Unlock()

	// wait one sanitize tick to ensure no additional deletes
	time.Sleep(25 * time.Millisecond)

	fake.mu.Lock()
	again := len(fake.deleted)
	fake.mu.Unlock()
	if again != 1 {
		t.Errorf("expected no additional deletes after sanitize, got %d total", again)
	}
}

func TestSanitizeStopsOnContextCancel(t *testing.T) {
	fake := NewFakeManager()
	ctx, cancel := context.WithCancel(context.Background())
	mIface := NewTTLManager[TestSession](ctx, fake, 10*time.Millisecond, 5*time.Millisecond)

	// Assert interface to concrete type
	_, ok := mIface.(*TTLManager[TestSession])
	if !ok {
		t.Fatal("failed to cast WorkerSessionManager to *TTLManager")
	}

	cancel() // cancel context

	// Wait a bit for goroutine to finish
	time.Sleep(10 * time.Millisecond)

	// No explicit asserts here, just checking no panic or deadlock
}

func TestSanitizeWithExpiration(t *testing.T) {
	fake := NewFakeManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mIface := NewTTLManager[TestSession](ctx, fake, 10*time.Millisecond, 5*time.Millisecond)

	m, ok := mIface.(*TTLManager[TestSession])
	if !ok {
		t.Fatal("failed to cast WorkerSessionManager to *TTLManager")
	}

	in, _ := netip.ParseAddr("1.1.1.1")
	ex, _ := netip.ParseAddrPort("2.2.2.2:9000")
	s := TestSession{internal: in, external: ex}
	m.Add(s)

	m.mu.Lock()
	m.expMap[s] = time.Now().Add(-time.Millisecond)
	m.mu.Unlock()

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
