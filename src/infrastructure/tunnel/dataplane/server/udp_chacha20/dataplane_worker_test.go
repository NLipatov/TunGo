package udp_chacha20

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"testing"

	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	chacha20 "tungo/infrastructure/cryptography/chacha20/udp"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/service_packet"
	"tungo/infrastructure/tunnel/controlplane"
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

func (dpMockEgress) SendDataIP([]byte) error  { return nil }
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

type dpMockEpochManager struct{}

func (dpMockEpochManager) StageEpoch(_, _ []byte) (uint16, error) { return 0, nil }
func (dpMockEpochManager) PromoteSendEpoch(uint16)                {}
func (dpMockEpochManager) RetirePreviousEpoch() bool              { return true }

type dpCountingEpochManager struct {
	nextEpoch uint16
}

func (m *dpCountingEpochManager) StageEpoch(_, _ []byte) (uint16, error) {
	m.nextEpoch++
	return m.nextEpoch, nil
}
func (*dpCountingEpochManager) PromoteSendEpoch(uint16)   {}
func (*dpCountingEpochManager) RetirePreviousEpoch() bool { return true }

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
	buf := make([]byte, chacha20.EpochOffset+2)
	binary.BigEndian.PutUint16(buf[chacha20.EpochOffset:chacha20.EpochOffset+2], epoch)
	return buf
}

func makeRekeyInit(t *testing.T, crypto primitives.KeyDeriver) []byte {
	t.Helper()
	clientPub, _, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	packet := make([]byte, service_packet.RekeyPacketLen)
	if _, err := service_packet.EncodeV1Header(service_packet.RekeyInit, packet); err != nil {
		t.Fatal(err)
	}
	copy(packet[3:], clientPub)
	return packet
}

func newTestPeer(crypto connection.Crypto, fsm connection.RekeyController, internalIP netip.Addr) *session.Peer {
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
	fsm := rekey.NewStateMachine(dpMockEpochManager{}, []byte("c2s"), []byte("s2c"))
	peer := newTestPeer(crypto, fsm, srcIP)

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

func TestHandleEstablished_RejectsStaleRekeyInitAcrossTransactions(t *testing.T) {
	keyDeriver := &primitives.DefaultKeyDeriver{}
	epochManager := &dpCountingEpochManager{}
	fsm := rekey.NewStateMachine(epochManager, make([]byte, 32), make([]byte, 32))
	coordinator := controlplane.NewServerRekeyCoordinator(fsm)
	crypto := &dpMockCrypto{}
	peer := newTestPeer(crypto, coordinator, netip.MustParseAddr("10.0.0.2"))
	worker := newUdpDataplaneWorker(&dpMockTunWriter{}, newServicePacketHandler(keyDeriver))
	firstInit := makeRekeyInit(t, keyDeriver)
	secondInit := makeRekeyInit(t, keyDeriver)

	crypto.decOut = firstInit
	if err := worker.HandleEstablished(peer, makeCiphertext(0)); err != nil {
		t.Fatalf("first init: %v", err)
	}
	if epochManager.nextEpoch != 1 {
		t.Fatalf("first staged epoch = %d, want 1", epochManager.nextEpoch)
	}

	// Receiving the next Init under epoch 1 completes transaction 1 before
	// the control-plane handler stages transaction 2.
	crypto.decOut = secondInit
	if err := worker.HandleEstablished(peer, makeCiphertext(1)); err != nil {
		t.Fatalf("second init: %v", err)
	}
	if epochManager.nextEpoch != 2 {
		t.Fatalf("second staged epoch = %d, want 2", epochManager.nextEpoch)
	}

	// Confirm transaction 2, then replay transaction 1 under its original
	// authenticated carrier epoch. It must not become transaction 3.
	if err := worker.HandleEstablished(peer, makeCiphertext(2)); err != nil {
		t.Fatalf("second init confirmation: %v", err)
	}
	crypto.decOut = firstInit
	if err := worker.HandleEstablished(peer, makeCiphertext(0)); err != nil {
		t.Fatalf("stale first init: %v", err)
	}
	if epochManager.nextEpoch != 2 {
		t.Fatalf("stale init staged epoch %d, want 2", epochManager.nextEpoch)
	}
}
