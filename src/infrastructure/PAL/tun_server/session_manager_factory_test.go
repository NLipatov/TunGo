package tun_server

import (
	"reflect"
	"testing"
)

type sessionManagerFactoryDummySession struct {
	internalIP [4]byte
	externalIP [4]byte
}

func (d sessionManagerFactoryDummySession) InternalIP() [4]byte {
	return d.internalIP
}
func (d sessionManagerFactoryDummySession) ExternalIP() [4]byte {
	return d.externalIP
}

func TestSessionManagerFactory_CreateManager(t *testing.T) {
	f := newSessionManagerFactory[sessionManagerFactoryDummySession]()
	mgr := f.createManager()

	sess := sessionManagerFactoryDummySession{
		internalIP: [4]byte{10, 0, 0, 1},
		externalIP: [4]byte{1, 2, 3, 4},
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

	sess := sessionManagerFactoryDummySession{
		internalIP: [4]byte{172, 16, 0, 2},
		externalIP: [4]byte{8, 8, 8, 8},
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
