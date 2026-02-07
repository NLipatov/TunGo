package tcp_chacha20

import (
	"context"
	"net/netip"
	"testing"

	"tungo/application/network/connection"
	"tungo/infrastructure/tunnel/session"
)

type dpwTestCrypto struct {
	plaintext []byte
}

func (c *dpwTestCrypto) Encrypt(plaintext []byte) ([]byte, error) { return plaintext, nil }
func (c *dpwTestCrypto) Decrypt(_ []byte) ([]byte, error)         { return c.plaintext, nil }

func newDPWPeer(internal string, crypto connection.Crypto) *session.Peer {
	in := netip.MustParseAddr(internal)
	ex := netip.MustParseAddrPort("203.0.113.10:41000")
	s := session.NewSession(crypto, nil, in, ex)
	return session.NewPeer(s, nil)
}

func newDPWWorker(peer *session.Peer, tr connection.Transport, tun *fakeWriter, repo session.Repository) *tcpDataplaneWorker {
	return &tcpDataplaneWorker{
		ctx:            context.Background(),
		peer:           peer,
		transport:      tr,
		tunFile:        tun,
		sessionManager: repo,
		logger:         &fakeLogger{},
	}
}

func TestTCPDataplaneWorker_Run_StopsWhenPeerClosed(t *testing.T) {
	crypto := &dpwTestCrypto{plaintext: makeValidIPv4Packet(netip.MustParseAddr("10.0.0.2"))}
	peer := newDPWPeer("10.0.0.1", crypto)
	repo := session.NewDefaultRepository()
	repo.Add(peer)
	repo.Delete(peer) // marks peer closed

	worker := newDPWWorker(
		peer,
		&fakeConn{readBufs: [][]byte{make([]byte, 32)}},
		&fakeWriter{},
		repo,
	)

	worker.Run()
}

func TestTCPDataplaneWorker_Run_DropsMalformedIP(t *testing.T) {
	crypto := &dpwTestCrypto{plaintext: []byte{0x01, 0x02, 0x03}} // invalid IP header
	peer := newDPWPeer("10.0.0.1", crypto)
	repo := &fakeSessionRepo{}
	tun := &fakeWriter{}

	worker := newDPWWorker(
		peer,
		&fakeConn{readBufs: [][]byte{make([]byte, 32)}},
		tun,
		repo,
	)

	worker.Run()
	if len(tun.wrote) != 0 {
		t.Fatalf("expected no writes to tun for malformed packet, got %d", len(tun.wrote))
	}
}

func TestTCPDataplaneWorker_Run_DropsDisallowedSourceIP(t *testing.T) {
	crypto := &dpwTestCrypto{plaintext: makeValidIPv4Packet(netip.MustParseAddr("10.0.0.99"))}
	peer := newDPWPeer("10.0.0.1", crypto) // only 10.0.0.1 is allowed
	repo := &fakeSessionRepo{}
	tun := &fakeWriter{}

	worker := newDPWWorker(
		peer,
		&fakeConn{readBufs: [][]byte{make([]byte, 32)}},
		tun,
		repo,
	)

	worker.Run()
	if len(tun.wrote) != 0 {
		t.Fatalf("expected no writes to tun for disallowed source IP, got %d", len(tun.wrote))
	}
}
