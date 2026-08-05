package udp_chacha20

import (
	"errors"
	"sync"
	"testing"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/cryptography/chacha20/udp"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/service_packet"
	"tungo/infrastructure/tunnel/controlplane"

	"golang.org/x/crypto/chacha20poly1305"
)

// spTestEgress captures packets sent through egress for test assertions.
type spTestEgress struct {
	mu      sync.Mutex
	packets [][]byte
}

func (e *spTestEgress) SendDataIP(plaintext []byte) error {
	return e.send(plaintext)
}

func (e *spTestEgress) SendControl(plaintext []byte) error {
	return e.send(plaintext)
}

func (e *spTestEgress) send(plaintext []byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	buf := make([]byte, len(plaintext))
	copy(buf, plaintext)
	e.packets = append(e.packets, buf)
	return nil
}

func (e *spTestEgress) Close() error { return nil }

func TestHandle_Ping_SendsPong(t *testing.T) {
	handler := newServicePacketHandler(&primitives.DefaultKeyDeriver{})

	// Build a valid Ping packet (3 bytes: 0xFF, 0x01, Ping type).
	ping := make([]byte, 3)
	if _, err := service_packet.EncodeV1Header(service_packet.Ping, ping); err != nil {
		t.Fatalf("failed to encode ping: %v", err)
	}

	eg := &spTestEgress{}
	handled, err := handler.Handle(0, ping, eg, nil)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if !handled {
		t.Fatal("expected Ping to be handled")
	}

	eg.mu.Lock()
	defer eg.mu.Unlock()
	if len(eg.packets) != 1 {
		t.Fatalf("expected 1 Pong packet, got %d", len(eg.packets))
	}
	pkt := eg.packets[0]
	payload := pkt[udp.RouteIDLength+chacha20poly1305.NonceSize:]
	if len(payload) < 3 {
		t.Fatalf("pong payload too short: %d", len(payload))
	}
	if payload[0] != service_packet.Prefix || payload[1] != service_packet.VersionV1 || payload[2] != byte(service_packet.Pong) {
		t.Fatalf("unexpected pong payload: %v", payload[:3])
	}
}

// spTestEpochManager is a controllable mock for rekey.EpochManager.
type spTestEpochManager struct {
	nextEpoch uint16
	err       error
}

func (r *spTestEpochManager) StageEpoch(_, _ []byte) (uint16, error) {
	if r.err != nil {
		return 0, r.err
	}
	r.nextEpoch++
	return r.nextEpoch, nil
}
func (*spTestEpochManager) PromoteSendEpoch(uint16)   {}
func (*spTestEpochManager) RetirePreviousEpoch() bool { return true }

// errTestEgress returns an error on SendControl.
type errTestEgress struct {
	sendErr error
}

func (e *errTestEgress) SendDataIP(_ []byte) error  { return e.sendErr }
func (e *errTestEgress) SendControl(_ []byte) error { return e.sendErr }
func (*errTestEgress) Close() error                 { return nil }

func TestHandle_NonServicePacket_ReturnsFalse(t *testing.T) {
	handler := newServicePacketHandler(&primitives.DefaultKeyDeriver{})
	eg := &spTestEgress{}
	handled, err := handler.Handle(0, []byte{0x45, 0x00, 0x00, 0x28}, eg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Fatal("expected Handle to return false for non-service packet")
	}
}

func TestHandle_RekeyInit_Success_SendsAck(t *testing.T) {
	crypto := &primitives.DefaultKeyDeriver{}
	handler := newServicePacketHandler(crypto)

	rk := &spTestEpochManager{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32))
	coordinator := controlplane.NewServerRekeyCoordinator(fsm)
	eg := &spTestEgress{}

	pub, _, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pkt := make([]byte, service_packet.RekeyPacketLen)
	_, _ = service_packet.EncodeV1Header(service_packet.RekeyInit, pkt)
	copy(pkt[3:], pub)

	handled, handleErr := handler.Handle(0, pkt, eg, coordinator)
	if handleErr != nil {
		t.Fatalf("unexpected error: %v", handleErr)
	}
	if !handled {
		t.Fatal("expected RekeyInit to be handled")
	}

	eg.mu.Lock()
	defer eg.mu.Unlock()
	if len(eg.packets) != 1 {
		t.Fatalf("expected 1 ACK packet, got %d", len(eg.packets))
	}
	ack := eg.packets[0]
	ackPayload := ack[udp.RouteIDLength+chacha20poly1305.NonceSize:]
	if len(ackPayload) < 3 {
		t.Fatalf("ACK payload too short: %d", len(ackPayload))
	}
	if ackPayload[0] != service_packet.Prefix || ackPayload[1] != service_packet.VersionV1 || ackPayload[2] != byte(service_packet.RekeyAck) {
		t.Fatalf("unexpected ACK header: %v", ackPayload[:3])
	}
}

func TestHandle_RekeyInit_RetryResendsSameAckWithoutStagingAnotherEpoch(t *testing.T) {
	crypto := &primitives.DefaultKeyDeriver{}
	handler := newServicePacketHandler(crypto)
	router := &spTestEpochManager{}
	fsm := rekey.NewStateMachine(router, make([]byte, 32), make([]byte, 32))
	coordinator := controlplane.NewServerRekeyCoordinator(fsm)
	egress := &spTestEgress{}

	pub, _, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	packet := make([]byte, service_packet.RekeyPacketLen)
	_, _ = service_packet.EncodeV1Header(service_packet.RekeyInit, packet)
	copy(packet[3:], pub)

	for range 2 {
		handled, handleErr := handler.Handle(0, packet, egress, coordinator)
		if handleErr != nil || !handled {
			t.Fatalf("retry: handled=%v err=%v", handled, handleErr)
		}
	}

	if router.nextEpoch != 1 {
		t.Fatalf("expected one staged epoch, got %d", router.nextEpoch)
	}
	egress.mu.Lock()
	defer egress.mu.Unlock()
	if len(egress.packets) != 2 {
		t.Fatalf("expected two ACK attempts, got %d", len(egress.packets))
	}
	firstPayload := egress.packets[0][udp.RouteIDLength+chacha20poly1305.NonceSize:]
	secondPayload := egress.packets[1][udp.RouteIDLength+chacha20poly1305.NonceSize:]
	if string(firstPayload) != string(secondPayload) {
		t.Fatal("retry returned a different ACK")
	}
}

func TestHandle_RekeyInit_NilFSM_NoAck(t *testing.T) {
	crypto := &primitives.DefaultKeyDeriver{}
	handler := newServicePacketHandler(crypto)

	pub, _, _ := crypto.GenerateX25519KeyPair()
	pkt := make([]byte, service_packet.RekeyPacketLen)
	_, _ = service_packet.EncodeV1Header(service_packet.RekeyInit, pkt)
	copy(pkt[3:], pub)

	eg := &spTestEgress{}
	handled, err := handler.Handle(0, pkt, eg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected RekeyInit to be handled even with nil FSM")
	}

	eg.mu.Lock()
	defer eg.mu.Unlock()
	if len(eg.packets) != 0 {
		t.Fatalf("expected no ACK with nil FSM, got %d", len(eg.packets))
	}
}

func TestHandle_RekeyInit_EgressError_Swallowed(t *testing.T) {
	crypto := &primitives.DefaultKeyDeriver{}
	handler := newServicePacketHandler(crypto)

	rk := &spTestEpochManager{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32))
	coordinator := controlplane.NewServerRekeyCoordinator(fsm)
	eg := &errTestEgress{sendErr: errors.New("send failed")}

	pub, _, _ := crypto.GenerateX25519KeyPair()
	pkt := make([]byte, service_packet.RekeyPacketLen)
	_, _ = service_packet.EncodeV1Header(service_packet.RekeyInit, pkt)
	copy(pkt[3:], pub)

	handled, err := handler.Handle(0, pkt, eg, coordinator)
	if err != nil {
		t.Fatalf("expected nil error (egress errors swallowed), got %v", err)
	}
	if !handled {
		t.Fatal("expected RekeyInit to be handled")
	}
}

func TestHandle_Pong_ReturnsTrueNilErr(t *testing.T) {
	handler := newServicePacketHandler(&primitives.DefaultKeyDeriver{})
	pkt := make([]byte, 3)
	_, _ = service_packet.EncodeV1Header(service_packet.Pong, pkt)

	eg := &spTestEgress{}
	handled, err := handler.Handle(0, pkt, eg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("expected Pong to be handled (default case)")
	}
}

func TestHandle_RekeyInit_EpochExhausted_ReturnsError(t *testing.T) {
	crypto := &primitives.DefaultKeyDeriver{}
	handler := newServicePacketHandler(crypto)

	rk := &spTestEpochManager{err: chacha20.ErrEpochExhausted}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32))
	coordinator := controlplane.NewServerRekeyCoordinator(fsm)

	pub, _, _ := crypto.GenerateX25519KeyPair()
	pkt := make([]byte, service_packet.RekeyPacketLen)
	_, _ = service_packet.EncodeV1Header(service_packet.RekeyInit, pkt)
	copy(pkt[3:], pub)

	eg := &spTestEgress{}
	handled, err := handler.Handle(0, pkt, eg, coordinator)
	if !handled {
		t.Fatal("expected RekeyInit to be handled")
	}
	if !errors.Is(err, chacha20.ErrEpochExhausted) {
		t.Fatalf("expected ErrEpochExhausted, got %v", err)
	}
}

func TestHandle_RekeyInit_NotStable_Swallowed(t *testing.T) {
	crypto := &primitives.DefaultKeyDeriver{}
	handler := newServicePacketHandler(crypto)

	rk := &spTestEpochManager{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32))
	// Put FSM in non-stable state so the coordinator ignores the request.
	_, _ = fsm.StartRekey(make([]byte, chacha20poly1305.KeySize), make([]byte, chacha20poly1305.KeySize))
	coordinator := controlplane.NewServerRekeyCoordinator(fsm)

	pub, _, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pkt := make([]byte, service_packet.RekeyPacketLen)
	_, _ = service_packet.EncodeV1Header(service_packet.RekeyInit, pkt)
	copy(pkt[3:], pub)

	eg := &spTestEgress{}
	handled, handleErr := handler.Handle(0, pkt, eg, coordinator)
	if !handled {
		t.Fatal("expected RekeyInit to be handled")
	}
	if handleErr != nil {
		t.Fatalf("expected nil error for non-stable FSM, got %v", handleErr)
	}
	eg.mu.Lock()
	defer eg.mu.Unlock()
	if len(eg.packets) != 0 {
		t.Fatalf("expected no ACK for non-stable FSM, got %d", len(eg.packets))
	}
}
