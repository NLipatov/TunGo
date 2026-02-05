package tun_server

import (
	"net/netip"
	"testing"
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/tunnel/session"
)

type sessionManagerFactoryDummySession struct {
	internalIP netip.Addr
	externalIP netip.AddrPort
}

func (d sessionManagerFactoryDummySession) InternalAddr() netip.Addr {
	return d.internalIP
}
func (d sessionManagerFactoryDummySession) ExternalAddrPort() netip.AddrPort {
	return d.externalIP
}
func (d sessionManagerFactoryDummySession) Crypto() connection.Crypto {
	return nil
}
func (d sessionManagerFactoryDummySession) RekeyController() rekey.FSM {
	return nil
}
func (d sessionManagerFactoryDummySession) IsSourceAllowed(netip.Addr) bool {
	return true
}

func TestSessionManagerFactory_CreateManager(t *testing.T) {
	f := newSessionManagerFactory()
	mgr := f.createManager()

	in, _ := netip.ParseAddr("10.0.0.1")
	ex, _ := netip.ParseAddrPort("1.2.3.4:9000")

	sess := sessionManagerFactoryDummySession{
		internalIP: in,
		externalIP: ex,
	}
	peer := session.NewPeer(sess, nil)

	mgr.Add(peer)
	gotByInt, err := mgr.GetByInternalAddrPort(sess.InternalAddr())
	if err != nil {
		t.Fatalf("GetByInternalAddrPort: unexpected error: %v", err)
	}
	if gotByInt != peer {
		t.Errorf("GetByInternalAddrPort: got different peer")
	}

	gotByExt, err := mgr.GetByExternalAddrPort(sess.ExternalAddrPort())
	if err != nil {
		t.Fatalf("GetByExternalAddrPort: unexpected error: %v", err)
	}
	if gotByExt != peer {
		t.Errorf("GetByExternalAddrPort: got different peer")
	}

	mgr.Delete(peer)
	if _, err := mgr.GetByInternalAddrPort(sess.InternalAddr()); err == nil {
		t.Error("after Delete, GetByInternalAddrPort should return error, got nil")
	}
}

func TestSessionManagerFactory_CreateConcurrentManager(t *testing.T) {
	f := newSessionManagerFactory()
	cmgr := f.createConcurrentManager()

	in, _ := netip.ParseAddr("172.16.0.2")
	ex, _ := netip.ParseAddrPort("8.8.8.8:9000")

	sess := sessionManagerFactoryDummySession{
		internalIP: in,
		externalIP: ex,
	}
	peer := session.NewPeer(sess, nil)

	cmgr.Add(peer)

	gotByInt, err := cmgr.GetByInternalAddrPort(sess.InternalAddr())
	if err != nil {
		t.Fatalf("concurrent GetByInternalAddrPort: unexpected error: %v", err)
	}
	if gotByInt != peer {
		t.Errorf("concurrent GetByInternalAddrPort: got different peer")
	}

	gotByExt, err := cmgr.GetByExternalAddrPort(sess.ExternalAddrPort())
	if err != nil {
		t.Fatalf("concurrent GetByExternalAddrPort: unexpected error: %v", err)
	}
	if gotByExt != peer {
		t.Errorf("concurrent GetByExternalAddrPort: got different peer")
	}

	cmgr.Delete(peer)
	if _, err := cmgr.GetByInternalAddrPort(sess.InternalAddr()); err == nil {
		t.Error("after concurrent Delete, GetByInternalAddrPort should return error, got nil")
	}
}
