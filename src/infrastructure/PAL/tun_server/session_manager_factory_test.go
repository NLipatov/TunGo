package tun_server

import (
	"net/netip"
	"reflect"
	"testing"
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20/rekey"
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
func (d sessionManagerFactoryDummySession) Transport() connection.Transport {
	return nil
}
func (d sessionManagerFactoryDummySession) Crypto() connection.Crypto {
	return nil
}
func (d sessionManagerFactoryDummySession) RekeyController() rekey.FSM {
	return nil
}

func TestSessionManagerFactory_CreateManager(t *testing.T) {
	f := newSessionManagerFactory[sessionManagerFactoryDummySession]()
	mgr := f.createManager()

	in, _ := netip.ParseAddr("10.0.0.1")
	ex, _ := netip.ParseAddrPort("1.2.3.4:9000")

	sess := sessionManagerFactoryDummySession{
		internalIP: in,
		externalIP: ex,
	}

	mgr.Add(sess)
	gotByInt, err := mgr.GetByInternalAddrPort(sess.InternalAddr())
	if err != nil {
		t.Fatalf("GetByInternalAddrPort: unexpected error: %v", err)
	}
	if !reflect.DeepEqual(gotByInt.InternalAddr(), sess.InternalAddr()) {
		t.Errorf("InternalAddr: got %v, want %v", gotByInt.InternalAddr(), sess.InternalAddr())
	}

	gotByExt, err := mgr.GetByExternalAddrPort(sess.ExternalAddrPort())
	if err != nil {
		t.Fatalf("GetByExternalAddrPort: unexpected error: %v", err)
	}
	if !reflect.DeepEqual(gotByExt.ExternalAddrPort(), sess.ExternalAddrPort()) {
		t.Errorf("ExternalAddrPort: got %v, want %v", gotByExt.ExternalAddrPort(), sess.ExternalAddrPort())
	}

	mgr.Delete(sess)
	if _, err := mgr.GetByInternalAddrPort(sess.InternalAddr()); err == nil {
		t.Error("after Delete, GetByInternalAddrPort should return error, got nil")
	}
}

func TestSessionManagerFactory_CreateConcurrentManager(t *testing.T) {
	f := newSessionManagerFactory[sessionManagerFactoryDummySession]()
	cmgr := f.createConcurrentManager()

	in, _ := netip.ParseAddr("172.16.0.2")
	ex, _ := netip.ParseAddrPort("8.8.8.8:9000")

	sess := sessionManagerFactoryDummySession{
		internalIP: in,
		externalIP: ex,
	}

	cmgr.Add(sess)

	gotByInt, err := cmgr.GetByInternalAddrPort(sess.InternalAddr())
	if err != nil {
		t.Fatalf("concurrent GetByInternalAddrPort: unexpected error: %v", err)
	}
	if !reflect.DeepEqual(gotByInt.InternalAddr(), sess.InternalAddr()) {
		t.Errorf("concurrent InternalAddr: got %v, want %v", gotByInt.InternalAddr(), sess.InternalAddr())
	}

	gotByExt, err := cmgr.GetByExternalAddrPort(sess.ExternalAddrPort())
	if err != nil {
		t.Fatalf("concurrent GetByExternalAddrPort: unexpected error: %v", err)
	}
	if !reflect.DeepEqual(gotByExt.ExternalAddrPort(), sess.ExternalAddrPort()) {
		t.Errorf("concurrent ExternalAddrPort: got %v, want %v", gotByExt.ExternalAddrPort(), sess.ExternalAddrPort())
	}

	cmgr.Delete(sess)
	if _, err := cmgr.GetByInternalAddrPort(sess.InternalAddr()); err == nil {
		t.Error("after concurrent Delete, GetByInternalAddrPort should return error, got nil")
	}
}
