package tun_server

import (
	"net/netip"
	"reflect"
	"testing"
)

type sessionManagerFactoryDummySession struct {
	internalIP netip.Addr
	externalIP netip.AddrPort
}

func (d sessionManagerFactoryDummySession) InternalIP() netip.Addr {
	return d.internalIP
}
func (d sessionManagerFactoryDummySession) ExternalIP() netip.AddrPort {
	return d.externalIP
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
	gotByInt, err := mgr.GetByInternalIP(sess.InternalIP())
	if err != nil {
		t.Fatalf("GetByInternalIP: unexpected error: %v", err)
	}
	if !reflect.DeepEqual(gotByInt.InternalIP(), sess.InternalIP()) {
		t.Errorf("InternalIP: got %v, want %v", gotByInt.InternalIP(), sess.InternalIP())
	}

	gotByExt, err := mgr.GetByExternalIP(sess.ExternalIP())
	if err != nil {
		t.Fatalf("GetByExternalIP: unexpected error: %v", err)
	}
	if !reflect.DeepEqual(gotByExt.ExternalIP(), sess.ExternalIP()) {
		t.Errorf("ExternalIP: got %v, want %v", gotByExt.ExternalIP(), sess.ExternalIP())
	}

	mgr.Delete(sess)
	if _, err := mgr.GetByInternalIP(sess.InternalIP()); err == nil {
		t.Error("after Delete, GetByInternalIP should return error, got nil")
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

	gotByInt, err := cmgr.GetByInternalIP(sess.InternalIP())
	if err != nil {
		t.Fatalf("concurrent GetByInternalIP: unexpected error: %v", err)
	}
	if !reflect.DeepEqual(gotByInt.InternalIP(), sess.InternalIP()) {
		t.Errorf("concurrent InternalIP: got %v, want %v", gotByInt.InternalIP(), sess.InternalIP())
	}

	gotByExt, err := cmgr.GetByExternalIP(sess.ExternalIP())
	if err != nil {
		t.Fatalf("concurrent GetByExternalIP: unexpected error: %v", err)
	}
	if !reflect.DeepEqual(gotByExt.ExternalIP(), sess.ExternalIP()) {
		t.Errorf("concurrent ExternalIP: got %v, want %v", gotByExt.ExternalIP(), sess.ExternalIP())
	}

	cmgr.Delete(sess)
	if _, err := cmgr.GetByInternalIP(sess.InternalIP()); err == nil {
		t.Error("after concurrent Delete, GetByInternalIP should return error, got nil")
	}
}
