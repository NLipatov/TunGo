package session

import (
	"net/netip"
	"testing"
)

func TestCompositeSessionRevoker_RevokeByPubKey(t *testing.T) {
	pubKey := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}

	// Create two repositories with sessions for the same pubkey
	repo1 := NewDefaultRepository().(*DefaultRepository)
	repo2 := NewDefaultRepository().(*DefaultRepository)

	s1 := &fakeSessionWithIdentity{
		fakeSession: fakeSession{
			internal: netip.MustParseAddr("10.0.0.1"),
			external: netip.MustParseAddrPort("1.1.1.1:1000"),
		},
		pubKey: pubKey,
	}
	s2 := &fakeSessionWithIdentity{
		fakeSession: fakeSession{
			internal: netip.MustParseAddr("10.0.0.2"),
			external: netip.MustParseAddrPort("2.2.2.2:2000"),
		},
		pubKey: pubKey,
	}

	repo1.Add(NewPeer(s1, nil))
	repo2.Add(NewPeer(s2, nil))

	// Create composite revoker and register both repos
	revoker := NewCompositeSessionRevoker()
	revoker.Register(repo1)
	revoker.Register(repo2)

	// Revoke by pubkey - should terminate sessions in both repos
	count := revoker.RevokeByPubKey(pubKey)
	if count != 2 {
		t.Errorf("expected 2 sessions revoked, got %d", count)
	}

	// Verify sessions are gone
	if _, err := repo1.GetByInternalAddrPort(s1.internal); err != ErrNotFound {
		t.Error("expected s1 to be removed from repo1")
	}
	if _, err := repo2.GetByInternalAddrPort(s2.internal); err != ErrNotFound {
		t.Error("expected s2 to be removed from repo2")
	}
}

func TestCompositeSessionRevoker_EmptyRevoker(t *testing.T) {
	revoker := NewCompositeSessionRevoker()

	// Should not panic with no registered repos
	count := revoker.RevokeByPubKey([]byte{1, 2, 3})
	if count != 0 {
		t.Errorf("expected 0 sessions revoked from empty revoker, got %d", count)
	}
}

func TestCompositeSessionRevoker_RegisterDuringRevoke(t *testing.T) {
	pubKey := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}

	repo := NewDefaultRepository().(*DefaultRepository)
	s := &fakeSessionWithIdentity{
		fakeSession: fakeSession{
			internal: netip.MustParseAddr("10.0.0.1"),
			external: netip.MustParseAddrPort("1.1.1.1:1000"),
		},
		pubKey: pubKey,
	}
	repo.Add(NewPeer(s, nil))

	revoker := NewCompositeSessionRevoker()
	revoker.Register(repo)

	// Should work correctly
	count := revoker.RevokeByPubKey(pubKey)
	if count != 1 {
		t.Errorf("expected 1 session revoked, got %d", count)
	}
}
