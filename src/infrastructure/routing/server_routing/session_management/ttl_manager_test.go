package session_management

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestSession is a simple type implementing ClientSession and comparable.
type TestSession struct {
	internal, external [4]byte
}

func (s TestSession) InternalIP() [4]byte { return s.internal }
func (s TestSession) ExternalIP() [4]byte { return s.external }

// FakeManager is a stub for WorkerSessionManager[TestSession].
type FakeManager struct {
	mu         sync.Mutex
	added      []TestSession
	deleted    []TestSession
	byInternal map[[4]byte]TestSession
	byExternal map[[4]byte]TestSession
}

func NewFakeManager() *FakeManager {
	return &FakeManager{
		byInternal: make(map[[4]byte]TestSession),
		byExternal: make(map[[4]byte]TestSession),
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

func (f *FakeManager) GetByInternalIP(ip [4]byte) (TestSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.byInternal[ip]
	if !ok {
		return TestSession{}, ErrSessionNotFound
	}
	return s, nil
}

func (f *FakeManager) GetByExternalIP(ip [4]byte) (TestSession, error) {
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

	s := TestSession{internal: [4]byte{1, 2, 3, 4}, external: [4]byte{4, 3, 2, 1}}
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

	s1 := TestSession{internal: [4]byte{1, 1, 1, 1}, external: [4]byte{2, 2, 2, 2}}
	s2 := TestSession{internal: [4]byte{3, 3, 3, 3}, external: [4]byte{4, 4, 4, 4}}
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

	s := TestSession{internal: [4]byte{9, 9, 9, 9}, external: [4]byte{8, 8, 8, 8}}
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
