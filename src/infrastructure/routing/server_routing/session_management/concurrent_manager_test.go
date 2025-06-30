package session_management

import (
	"net/netip"
	"sync"
	"testing"
)

type concurrentManagerMockSession struct {
	ext, in netip.Addr
}

func (s concurrentManagerMockSession) ExternalIP() netip.Addr { return s.ext }
func (s concurrentManagerMockSession) InternalIP() netip.Addr { return s.in }

type concurrentManagerMockManager struct {
	add, del, getInt, getExt int
	lastSession              concurrentManagerMockSession
	lastIP                   netip.Addr
}

func (m *concurrentManagerMockManager) Add(s concurrentManagerMockSession) {
	m.add++
	m.lastSession = s
}
func (m *concurrentManagerMockManager) Delete(s concurrentManagerMockSession) {
	m.del++
	m.lastSession = s
}
func (m *concurrentManagerMockManager) GetByInternalIP(b netip.Addr) (concurrentManagerMockSession, error) {
	m.getInt++
	m.lastIP = b
	return m.lastSession, nil
}
func (m *concurrentManagerMockManager) GetByExternalIP(b netip.Addr) (concurrentManagerMockSession, error) {
	m.getExt++
	m.lastIP = b
	return m.lastSession, nil
}

func TestConcurrentManager_Delegation(t *testing.T) {
	base := &concurrentManagerMockManager{}
	cm := NewConcurrentManager[concurrentManagerMockSession](base)

	in, _ := netip.ParseAddr("1.1.1.1")
	ex, _ := netip.ParseAddr("2.2.2.2")
	s := concurrentManagerMockSession{ext: ex, in: in}
	ip, _ := netip.ParseAddr("3.3.3.3")

	cm.Add(s)
	cm.Delete(s)
	_, _ = cm.GetByInternalIP(ip)
	_, _ = cm.GetByExternalIP(ip)

	switch {
	case base.add != 1, base.del != 1, base.getInt != 1, base.getExt != 1:
		t.Fatalf("not all methods delegated: %+v", base)
	case (base.lastSession.in) != s.in, base.lastIP != ip:
		t.Fatalf("wrong args forwarded")
	}
}

func TestConcurrentManager_Parallel_NoRace(t *testing.T) {
	base := &concurrentManagerMockManager{}
	cm := NewConcurrentManager[concurrentManagerMockSession](base)
	ex, _ := netip.ParseAddr("9.9.9.9")
	in, _ := netip.ParseAddr("8.8.8.8")
	s := concurrentManagerMockSession{ext: ex, in: in}

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
