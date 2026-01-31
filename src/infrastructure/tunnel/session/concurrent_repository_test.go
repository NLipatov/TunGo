package session

import (
	"net/netip"
	"sync"
	"testing"
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20/rekey"
)

type concurrentRepoMockSession struct {
	ext netip.AddrPort
	in  netip.Addr
}

func (s concurrentRepoMockSession) ExternalAddrPort() netip.AddrPort { return s.ext }
func (s concurrentRepoMockSession) InternalAddr() netip.Addr         { return s.in }
func (s concurrentRepoMockSession) Crypto() connection.Crypto        { return nil }
func (s concurrentRepoMockSession) RekeyController() rekey.FSM       { return nil }

type concurrentRepoMockManager struct {
	add, del, getInt, getExt int
	lastPeer                 *Peer
	lastIP                   netip.Addr
}

func (m *concurrentRepoMockManager) Add(p *Peer) {
	m.add++
	m.lastPeer = p
}
func (m *concurrentRepoMockManager) Delete(p *Peer) {
	m.del++
	m.lastPeer = p
}
func (m *concurrentRepoMockManager) GetByInternalAddrPort(b netip.Addr) (*Peer, error) {
	m.getInt++
	m.lastIP = b
	return m.lastPeer, nil
}
func (m *concurrentRepoMockManager) GetByExternalAddrPort(b netip.AddrPort) (*Peer, error) {
	m.getExt++
	m.lastIP = b.Addr()
	return m.lastPeer, nil
}

func TestConcurrentRepository_Delegation(t *testing.T) {
	base := &concurrentRepoMockManager{}
	cm := NewConcurrentRepository(base)

	in, _ := netip.ParseAddr("1.1.1.1")
	ex, _ := netip.ParseAddrPort("2.2.2.2:9000")
	sess := concurrentRepoMockSession{ext: ex, in: in}
	p := NewPeer(sess, nil)
	addr, _ := netip.ParseAddr("3.3.3.3")
	addrPort, _ := netip.ParseAddrPort("3.3.3.3:9000")

	cm.Add(p)
	cm.Delete(p)
	_, _ = cm.GetByExternalAddrPort(addrPort)
	_, _ = cm.GetByInternalAddrPort(addr)

	switch {
	case base.add != 1, base.del != 1, base.getInt != 1, base.getExt != 1:
		t.Fatalf("not all methods delegated: %+v", base)
	case base.lastPeer.InternalAddr() != p.InternalAddr(), base.lastIP != addr:
		t.Fatalf("wrong args forwarded")
	}
}

func TestConcurrentRepository_Parallel_NoRace(t *testing.T) {
	base := &concurrentRepoMockManager{}
	cm := NewConcurrentRepository(base)
	in, _ := netip.ParseAddr("8.8.8.8")
	ex, _ := netip.ParseAddrPort("9.9.9.9:9000")
	sess := concurrentRepoMockSession{ext: ex, in: in}
	p := NewPeer(sess, nil)

	const readers = 50
	var wg sync.WaitGroup
	wg.Add(readers + 1)

	go func() {
		defer wg.Done()
		for i := 0; i < 1_000; i++ {
			cm.Add(p)
		}
	}()

	for r := 0; r < readers; r++ {
		go func() {
			defer wg.Done()
			for i := 0; i < 2_000; i++ {
				_, _ = cm.GetByInternalAddrPort(p.InternalAddr())
			}
		}()
	}
	wg.Wait()
}
