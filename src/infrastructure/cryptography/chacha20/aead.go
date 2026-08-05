package chacha20

import (
	"crypto/cipher"
	"fmt"
	"tungo/application/network/connection"

	"golang.org/x/crypto/chacha20poly1305"
)

// NewAEADsFromHandshake creates send and receive ciphers for the local peer.
func NewAEADsFromHandshake(
	h connection.Handshake,
	isServer bool,
) (send cipher.AEAD, recv cipher.AEAD, err error) {
	c2s := h.KeyClientToServer()
	s2c := h.KeyServerToClient()
	if len(c2s) != chacha20poly1305.KeySize || len(s2c) != chacha20poly1305.KeySize {
		return nil, nil, fmt.Errorf(
			"handshake produced invalid key sizes: c2s=%d s2c=%d (want %d)",
			len(c2s), len(s2c), chacha20poly1305.KeySize,
		)
	}

	sendKey, recvKey := c2s, s2c
	if isServer {
		sendKey, recvKey = s2c, c2s
	}

	send, err = chacha20poly1305.New(sendKey)
	if err != nil {
		return nil, nil, fmt.Errorf("create send AEAD: %w", err)
	}
	recv, err = chacha20poly1305.New(recvKey)
	if err != nil {
		return nil, nil, fmt.Errorf("create receive AEAD: %w", err)
	}
	return send, recv, nil
}
