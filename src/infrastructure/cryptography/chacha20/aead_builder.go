package chacha20

import (
	"crypto/cipher"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
	"tungo/application"
)

// DefaultAEADBuilder builds AEAD cipher pairs (send/recv) from a handshake.
// Semantics:
//   - KeyServerToClient(): key for traffic the server SENDS and the client RECEIVES.
//   - KeyClientToServer(): key for traffic the client SENDS and the server RECEIVES.
//
// Mapping:
//   - isServer == true  → send=S→C, recv=C→S
//   - isServer == false → send=C→S, recv=S→C
type DefaultAEADBuilder struct{}

func NewDefaultAEADBuilder() application.AEADBuilder {
	return &DefaultAEADBuilder{}
}

func (a *DefaultAEADBuilder) FromHandshake(
	h application.Handshake,
	isServer bool,
) (send cipher.AEAD, recv cipher.AEAD, err error) {
	kS2C := h.KeyServerToClient()
	kC2S := h.KeyClientToServer()

	// Defensive: ChaCha20-Poly1305 requires 32-byte keys.
	if len(kS2C) != chacha20poly1305.KeySize || len(kC2S) != chacha20poly1305.KeySize {
		return nil, nil, fmt.Errorf("handshake produced invalid key sizes: s2c=%d c2s=%d (want %d)",
			len(kS2C), len(kC2S), chacha20poly1305.KeySize)
	}

	if isServer {
		send, err = chacha20poly1305.New(kS2C) // server sends → S→C
		if err != nil {
			return nil, nil, fmt.Errorf("new AEAD (server send S→C): %w", err)
		}
		recv, err = chacha20poly1305.New(kC2S) // server receives ← C→S
		if err != nil {
			return nil, nil, fmt.Errorf("new AEAD (server recv C→S): %w", err)
		}
	} else {
		send, err = chacha20poly1305.New(kC2S) // client sends → C→S
		if err != nil {
			return nil, nil, fmt.Errorf("new AEAD (client send C→S): %w", err)
		}
		recv, err = chacha20poly1305.New(kS2C) // client receives ← S→C
		if err != nil {
			return nil, nil, fmt.Errorf("new AEAD (client recv S→C): %w", err)
		}
	}
	return send, recv, nil
}
