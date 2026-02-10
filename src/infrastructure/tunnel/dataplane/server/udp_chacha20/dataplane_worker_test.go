package udp_chacha20

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"testing"
	"time"
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/tunnel/session"
)

// --- mocks ---

type dpMockCrypto struct {
	decOut []byte
	decErr error
}

func (m *dpMockCrypto) Encrypt(b []byte) ([]byte, error) { return b, nil }
func (m *dpMockCrypto) Decrypt(_ []byte) ([]byte, error) { return m.decOut, m.decErr }

type dpMockEgress struct{}

func (dpMockEgress) SendDataIP([]byte) error { return nil }
func (dpMockEgress) SendControl([]byte) error { return nil }
func (dpMockEgress) Close() error             { return nil }

type dpMockTunWriter struct {
	writes int
	err    error
}

func (m *dpMockTunWriter) Write(p []byte) (int, error) {
	m.writes++
	if m.err != nil {
		return 0, m.err
	}
	return len(p), nil
}

type dpMockRekeyer struct{}

func (dpMockRekeyer) Rekey(_, _ []byte) (uint16, error) { return 0, nil }
func (dpMockRekeyer) SetSendEpoch(uint16)               {}
func (dpMockRekeyer) RemoveEpoch(uint16) bool            { return true }

// makeIPv4Packet builds a minimal valid IPv4 packet with the given source IP.
func makeIPv4Packet(srcIP netip.Addr) []byte {
	pkt := make([]byte, 20)
	pkt[0] = 0x45 // version 4, IHL 5
	ip4 := srcIP.As4()
	copy(pkt[12:16], ip4[:])
	return pkt
}

// makeCiphertext returns a byte slice large enough for the epoch offset.
func makeCiphertext(epoch uint16) []byte {
	buf := make([]byte, chacha20.NonceEpochOffset+2)
	binary.BigEndian.PutUint16(buf[chacha20.NonceEpochOffset:chacha20.NonceEpochOffset+2], epoch)
	return buf
}

func newTestPeer(crypto connection.Crypto, fsm rekey.FSM, internalIP netip.Addr) *session.Peer {
	sess := session.NewSessionWithAuth(
		crypto, fsm, internalIP, netip.AddrPort{},
		nil, []netip.Prefix{netip.MustParsePrefix(internalIP.String() + "/32")},
	)
	return session.NewPeer(sess, dpMockEgress{})
}

// --- tests ---

func TestHandleEstablished_PeerClosed_ReturnsNil(t *testing.T) {
	crypto := &dpMockCrypto{decOut: makeIPv4Packet(netip.MustParseAddr("10.0.0.2"))}
	peer := newTestPeer(crypto, nil, netip.MustParseAddr("10.0.0.2"))
	// Simulate closed peer by marking it.
	peer.MarkClosedForTest()

	w := newUdpDataplaneWorker(&dpMockTunWriter{}, controlPlaneHandler{})
	err := w.HandleEstablished(peer, makeCiphertext(0))
	if err != nil {
		t.Fatalf("expected nil for closed peer, got %v", err)
	}
}

func TestHandleEstablished_DecryptFails_DropsPacket(t *testing.T) {
	crypto := &dpMockCrypto{decErr: errors.New("bad auth tag")}
	peer := newTestPeer(crypto, nil, netip.MustParseAddr("10.0.0.2"))

	tun := &dpMockTunWriter{}
	w := newUdpDataplaneWorker(tun, controlPlaneHandler{})
	err := w.HandleEstablished(peer, makeCiphertext(0))
	if err != nil {
		t.Fatalf("expected nil (drop), got %v", err)
	}
	if tun.writes != 0 {
		t.Fatal("should not write to TUN on decryption failure")
	}
}

func TestHandleEstablished_MalformedIP_DropsPacket(t *testing.T) {
	// Decrypted payload too short for any valid IP header.
	crypto := &dpMockCrypto{decOut: []byte{0x00}}
	peer := newTestPeer(crypto, nil, netip.MustParseAddr("10.0.0.2"))

	tun := &dpMockTunWriter{}
	w := newUdpDataplaneWorker(tun, controlPlaneHandler{})
	err := w.HandleEstablished(peer, makeCiphertext(0))
	if err != nil {
		t.Fatalf("expected nil (drop), got %v", err)
	}
	if tun.writes != 0 {
		t.Fatal("should not write to TUN for malformed IP")
	}
}

func TestHandleEstablished_AllowedIPsViolation_DropsPacket(t *testing.T) {
	// Source IP 10.0.0.99 not in AllowedIPs for peer 10.0.0.2.
	crypto := &dpMockCrypto{decOut: makeIPv4Packet(netip.MustParseAddr("10.0.0.99"))}
	peer := newTestPeer(crypto, nil, netip.MustParseAddr("10.0.0.2"))

	tun := &dpMockTunWriter{}
	w := newUdpDataplaneWorker(tun, controlPlaneHandler{})
	err := w.HandleEstablished(peer, makeCiphertext(0))
	if err != nil {
		t.Fatalf("expected nil (drop), got %v", err)
	}
	if tun.writes != 0 {
		t.Fatal("should not write to TUN for AllowedIPs violation")
	}
}

func TestHandleEstablished_HappyPath_WritesToTUN(t *testing.T) {
	srcIP := netip.MustParseAddr("10.0.0.2")
	crypto := &dpMockCrypto{decOut: makeIPv4Packet(srcIP)}
	peer := newTestPeer(crypto, nil, srcIP)

	tun := &dpMockTunWriter{}
	w := newUdpDataplaneWorker(tun, controlPlaneHandler{})
	err := w.HandleEstablished(peer, makeCiphertext(0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tun.writes != 1 {
		t.Fatalf("expected 1 TUN write, got %d", tun.writes)
	}
}

func TestHandleEstablished_TUNWriteError_ReturnsError(t *testing.T) {
	srcIP := netip.MustParseAddr("10.0.0.2")
	crypto := &dpMockCrypto{decOut: makeIPv4Packet(srcIP)}
	peer := newTestPeer(crypto, nil, srcIP)

	tunErr := errors.New("TUN write failed")
	tun := &dpMockTunWriter{err: tunErr}
	w := newUdpDataplaneWorker(tun, controlPlaneHandler{})
	err := w.HandleEstablished(peer, makeCiphertext(0))
	if err == nil {
		t.Fatal("expected error from TUN write")
	}
}

func TestHandleEstablished_WithRekeyController_ActivatesEpoch(t *testing.T) {
	srcIP := netip.MustParseAddr("10.0.0.2")
	crypto := &dpMockCrypto{decOut: makeIPv4Packet(srcIP)}
	fsm := rekey.NewStateMachine(dpMockRekeyer{}, []byte("c2s"), []byte("s2c"), true)
	peer := newTestPeer(crypto, fsm, srcIP)

	tun := &dpMockTunWriter{}
	w := newUdpDataplaneWorker(tun, controlPlaneHandler{})
	w.now = func() time.Time { return time.Now() }

	err := w.HandleEstablished(peer, makeCiphertext(0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tun.writes != 1 {
		t.Fatalf("expected 1 TUN write, got %d", tun.writes)
	}
}
