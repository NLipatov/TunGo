package tcp_chacha20

import (
	"errors"
	"sync"
	"testing"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/service_packet"
)

// tcpTestLogger captures log output for assertions.
type tcpTestLogger struct {
	mu   sync.Mutex
	msgs []string
}

func (l *tcpTestLogger) Printf(format string, _ ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.msgs = append(l.msgs, format)
}

// tcpTestRekeyer is a controllable mock for rekey.Rekeyer.
type tcpTestRekeyer struct {
	nextEpoch uint16
}

func (r *tcpTestRekeyer) Rekey(_, _ []byte) (uint16, error) {
	r.nextEpoch++
	return r.nextEpoch, nil
}
func (r *tcpTestRekeyer) SetSendEpoch(uint16)     {}
func (r *tcpTestRekeyer) RemoveEpoch(uint16) bool { return true }

// tcpTestEgress captures packets sent through egress.
type tcpTestEgress struct {
	mu      sync.Mutex
	packets [][]byte
	sendErr error
}

func (e *tcpTestEgress) SendDataIP(plaintext []byte) error  { return e.send(plaintext) }
func (e *tcpTestEgress) SendControl(plaintext []byte) error { return e.send(plaintext) }
func (e *tcpTestEgress) Close() error                       { return nil }

func (e *tcpTestEgress) send(plaintext []byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.sendErr != nil {
		return e.sendErr
	}
	buf := make([]byte, len(plaintext))
	copy(buf, plaintext)
	e.packets = append(e.packets, buf)
	return nil
}

func buildTCPRekeyInitPacket(t *testing.T, crypto primitives.KeyDeriver) []byte {
	t.Helper()
	pub, _, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pkt := make([]byte, service_packet.RekeyPacketLen)
	if _, err := service_packet.EncodeV1Header(service_packet.RekeyInit, pkt); err != nil {
		t.Fatal(err)
	}
	copy(pkt[3:], pub)
	return pkt
}

func TestTCPHandle_NonServicePacket_ReturnsFalse(t *testing.T) {
	logger := &tcpTestLogger{}
	h := newControlPlaneHandler(&primitives.DefaultKeyDeriver{}, logger)
	eg := &tcpTestEgress{}

	// Random data that is not a service packet.
	handled := h.Handle([]byte{0x45, 0x00, 0x00, 0x28}, eg, nil)
	if handled {
		t.Fatal("expected Handle to return false for non-service packet")
	}
}

func TestTCPHandle_UnknownServicePacket_ReturnsTrue(t *testing.T) {
	logger := &tcpTestLogger{}
	h := newControlPlaneHandler(&primitives.DefaultKeyDeriver{}, logger)
	eg := &tcpTestEgress{}

	// A valid V1 header for Ping (3 bytes) — not RekeyInit, so falls to default case.
	pkt := make([]byte, 3)
	_, _ = service_packet.EncodeV1Header(service_packet.Ping, pkt)

	handled := h.Handle(pkt, eg, nil)
	if !handled {
		t.Fatal("expected Handle to return true for recognized service packet")
	}
}

func TestTCPHandle_RekeyInit_Success_SendsAckAndActivates(t *testing.T) {
	logger := &tcpTestLogger{}
	crypto := &primitives.DefaultKeyDeriver{}
	h := newControlPlaneHandler(crypto, logger)

	rk := &tcpTestRekeyer{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32), true)
	eg := &tcpTestEgress{}

	pkt := buildTCPRekeyInitPacket(t, crypto)
	handled := h.Handle(pkt, eg, fsm)
	if !handled {
		t.Fatal("expected Handle to return true for RekeyInit")
	}

	eg.mu.Lock()
	defer eg.mu.Unlock()
	if len(eg.packets) != 1 {
		t.Fatalf("expected 1 ACK packet sent, got %d", len(eg.packets))
	}

	// Verify ACK packet has RekeyAck header.
	ack := eg.packets[0]
	if len(ack) < 3 {
		t.Fatalf("ACK packet too short: %d", len(ack))
	}
	if ack[0] != service_packet.Prefix || ack[1] != service_packet.VersionV1 || ack[2] != byte(service_packet.RekeyAck) {
		t.Fatalf("unexpected ACK header: %v", ack[:3])
	}
}

func TestTCPHandle_RekeyInit_ShortPacket_NilFSM(t *testing.T) {
	logger := &tcpTestLogger{}
	crypto := &primitives.DefaultKeyDeriver{}
	h := newControlPlaneHandler(crypto, logger)
	eg := &tcpTestEgress{}

	// RekeyInit packet with nil FSM — ServerHandleRekeyInit returns ok=false.
	pkt := buildTCPRekeyInitPacket(t, crypto)
	handled := h.Handle(pkt, eg, nil)
	if !handled {
		t.Fatal("expected Handle to return true (RekeyInit parsed)")
	}

	eg.mu.Lock()
	defer eg.mu.Unlock()
	if len(eg.packets) != 0 {
		t.Fatalf("expected no ACK sent with nil FSM, got %d packets", len(eg.packets))
	}
}

func TestTCPHandle_RekeyInit_EpochExhausted_Logs(t *testing.T) {
	logger := &tcpTestLogger{}
	crypto := &primitives.DefaultKeyDeriver{}
	h := newControlPlaneHandler(crypto, logger)

	rk := &tcpTestRekeyer{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32), true)
	// Force epoch exhaustion so ServerHandleRekeyInit returns ErrEpochExhausted.
	fsm.LastRekeyEpoch = 65001

	eg := &tcpTestEgress{}
	pkt := buildTCPRekeyInitPacket(t, crypto)
	handled := h.Handle(pkt, eg, fsm)
	if !handled {
		t.Fatal("expected Handle to return true for RekeyInit")
	}

	logger.mu.Lock()
	defer logger.mu.Unlock()
	if len(logger.msgs) == 0 {
		t.Fatal("expected logger to capture epoch exhaustion error")
	}
	// No ACK should be sent.
	eg.mu.Lock()
	defer eg.mu.Unlock()
	if len(eg.packets) != 0 {
		t.Fatalf("expected no ACK on epoch exhaustion, got %d", len(eg.packets))
	}
}

func TestTCPHandle_RekeyInit_EgressError_Logs(t *testing.T) {
	logger := &tcpTestLogger{}
	crypto := &primitives.DefaultKeyDeriver{}
	h := newControlPlaneHandler(crypto, logger)

	rk := &tcpTestRekeyer{}
	fsm := rekey.NewStateMachine(rk, make([]byte, 32), make([]byte, 32), true)
	eg := &tcpTestEgress{sendErr: errors.New("send failed")}

	pkt := buildTCPRekeyInitPacket(t, crypto)
	handled := h.Handle(pkt, eg, fsm)
	if !handled {
		t.Fatal("expected Handle to return true for RekeyInit")
	}

	logger.mu.Lock()
	defer logger.mu.Unlock()
	if len(logger.msgs) == 0 {
		t.Fatal("expected logger to capture send error")
	}
}
