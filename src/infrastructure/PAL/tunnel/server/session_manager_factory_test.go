package server

import (
	"net/netip"
	"testing"
	"tungo/infrastructure/tunnel/session"
)

func TestSessionManagerFactory_CreateManager(t *testing.T) {
	f := newSessionManagerFactory()
	mgr := f.createManager()

	in, _ := netip.ParseAddr("10.0.0.1")
	ex, _ := netip.ParseAddrPort("1.2.3.4:9000")

	peer := session.NewPeer(nil, nil, in, ex, nil)

	mgr.Add(peer)
	gotByInt, err := mgr.GetByInternalAddrPort(in)
	if err != nil {
		t.Fatalf("GetByInternalAddrPort: unexpected error: %v", err)
	}
	if gotByInt != peer {
		t.Errorf("GetByInternalAddrPort: got different peer")
	}

	mgr.Delete(peer)
	if _, err := mgr.GetByInternalAddrPort(in); err == nil {
		t.Error("after Delete, GetByInternalAddrPort should return error, got nil")
	}
}
