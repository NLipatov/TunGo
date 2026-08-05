package udp

import (
	"crypto/rand"

	"golang.org/x/crypto/chacha20poly1305"
)

func randKey() []byte {
	key := make([]byte, chacha20poly1305.KeySize)
	_, _ = rand.Read(key)
	return key
}

func randID() [32]byte {
	var id [32]byte
	_, _ = rand.Read(id[:])
	return id
}
