package udp_chacha20

import (
	"net"
	"testing"
	"time"
)

type pipeAdapter struct{ net.Conn }

func (p *pipeAdapter) Close() error { return p.Conn.Close() }

// noopCrypto implements application.CryptographyService without actual crypto.
type noopCrypto struct{}

func (noopCrypto) Encrypt(p []byte) ([]byte, error) { return p, nil }
func (noopCrypto) Decrypt(p []byte) ([]byte, error) { return p, nil }

func TestMTUProbeHandlerAwaitAckTimeout(t *testing.T) {
	client, _ := net.Pipe()
	defer client.Close()

	prober := NewMTUProbeHandler(&pipeAdapter{client}, noopCrypto{}).(*MTUProbeHandler)

	ok, _, err := prober.AwaitAck(50 * time.Millisecond)
	if err != nil {
		t.Fatalf("AwaitAck returned error: %v", err)
	}
	if ok {
		t.Fatalf("expected no acknowledgement on timeout")
	}
}
