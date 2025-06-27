package session_management

import (
	"sync"
	"testing"
)

type concurrentManagerMockSession struct {
	ext, in [4]byte
}

func (s concurrentManagerMockSession) ExternalIP() [4]byte { return s.ext }
func (s concurrentManagerMockSession) InternalIP() [4]byte { return s.in }

type concurrentManagerMockManager struct {
	add, del, getInt, getExt int
	lastSession              concurrentManagerMockSession
	lastIP                   [4]byte
}

func (m *concurrentManagerMockManager) Add(s concurrentManagerMockSession) {
	m.add++
	m.lastSession = s
}
func (m *concurrentManagerMockManager) Delete(s concurrentManagerMockSession) {
	m.del++
	m.lastSession = s
}
func (m *concurrentManagerMockManager) GetByInternalIP(b [4]byte) (concurrentManagerMockSession, error) {
	m.getInt++
	m.lastIP = b
	return m.lastSession, nil
}
func (m *concurrentManagerMockManager) GetByExternalIP(b [4]byte) (concurrentManagerMockSession, error) {
	m.getExt++
	m.lastIP = b
	return m.lastSession, nil
}

func TestConcurrentManager_Delegation(t *testing.T) {
	base := &concurrentManagerMockManager{}
	cm := NewConcurrentManager[concurrentManagerMockSession](base)

	s := concurrentManagerMockSession{ext: [4]byte{1, 1, 1, 1}, in: [4]byte{2, 2, 2, 2}}
	ip := [4]byte{3, 3, 3, 3}

	cm.Add(s)
	cm.Delete(s)
	_, _ = cm.GetByInternalIP(ip)
	_, _ = cm.GetByExternalIP(ip)

	switch {
	case base.add != 1, base.del != 1, base.getInt != 1, base.getExt != 1:
		t.Fatalf("not all methods delegated: %+v", base)
	case string(base.lastSession.in[:]) != string(s.in[:]), string(base.lastIP[:]) != string(ip[:]):
		t.Fatalf("wrong args forwarded")
	}
}

func TestConcurrentManager_Parallel_NoRace(t *testing.T) {
	base := &concurrentManagerMockManager{}
	cm := NewConcurrentManager[concurrentManagerMockSession](base)
	s := concurrentManagerMockSession{ext: [4]byte{9, 9, 9, 9}, in: [4]byte{8, 8, 8, 8}}

	const readers = 50
	var wg sync.WaitGroup
	wg.Add(readers + 1)

	go func() {
		defer wg.Done()
		for i := 0; i < 1_000; i++ {
			cm.Add(s)
		}
	}()

	for r := 0; r < readers; r++ {
		go func() {
			defer wg.Done()
			for i := 0; i < 2_000; i++ {
				_, _ = cm.GetByInternalIP(s.in)
			}
		}()
	}
	wg.Wait()
}
