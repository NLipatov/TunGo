package udp_chacha20

import (
	"sync"
	"testing"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/network/service_packet"

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
	handler := newServicePacketHandler(&handshake.DefaultCrypto{})

	// Build a valid Ping packet (3 bytes: 0xFF, 0x01, Ping type).
	ping := make([]byte, 3)
	if _, err := service_packet.EncodeV1Header(service_packet.Ping, ping); err != nil {
		t.Fatalf("failed to encode ping: %v", err)
	}

	eg := &spTestEgress{}
	handled, err := handler.Handle(ping, eg, nil)
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
	payload := pkt[chacha20poly1305.NonceSize:]
	if len(payload) < 3 {
		t.Fatalf("pong payload too short: %d", len(payload))
	}
	if payload[0] != service_packet.Prefix || payload[1] != service_packet.VersionV1 || payload[2] != byte(service_packet.Pong) {
		t.Fatalf("unexpected pong payload: %v", payload[:3])
	}
}
