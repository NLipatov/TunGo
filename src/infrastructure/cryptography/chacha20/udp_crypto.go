package chacha20

import (
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"sync"

	"golang.org/x/crypto/chacha20poly1305"
)

const defaultEpochRingCapacity = 4

// EpochUdpCrypto manages immutable UDP sessions and resolves them via an EpochRing.
// It implements connection.Crypto. It holds no raw keys or handshake state.
type EpochUdpCrypto struct {
	ring      EpochRing
	isServer  bool
	sessionId [32]byte
	mu        sync.RWMutex
	rekeyMu   sync.Mutex
	sendEpoch Epoch
}

func NewEpochUdpCrypto(
	sessionId [32]byte,
	sendCipher, recvCipher cipher.AEAD,
	isServer bool,
) *EpochUdpCrypto {
	initialEpoch := Epoch(0)
	initialSession := NewUdpSessionWithCiphers(sessionId, sendCipher, recvCipher, isServer, initialEpoch)

	return &EpochUdpCrypto{
		ring:      NewEpochRing(defaultEpochRingCapacity, initialEpoch, initialSession),
		isServer:  isServer,
		sessionId: sessionId,
		sendEpoch: initialEpoch,
	}
}

func (c *EpochUdpCrypto) Encrypt(plaintext []byte) ([]byte, error) {
	c.mu.RLock()
	epoch := c.sendEpoch
	c.mu.RUnlock()

	session, ok := c.ring.Resolve(epoch)
	if !ok {
		// Should not happen; fall back to latest known session to avoid drop.
		session, ok = c.ring.ResolveCurrent()
		if !ok {
			return nil, fmt.Errorf("no active session")
		}
	}
	return session.Encrypt(plaintext)
}

func (c *EpochUdpCrypto) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < chacha20poly1305.NonceSize {
		return nil, fmt.Errorf("cipher too short: %d", len(ciphertext))
	}
	epoch := Epoch(binary.BigEndian.Uint16(ciphertext[:2]))
	session, ok := c.ring.Resolve(epoch)
	if !ok {
		return nil, ErrUnknownEpoch
	}
	if session.Epoch() != epoch {
		return nil, ErrUnknownEpoch
	}
	return session.Decrypt(ciphertext)
}

// Rekey installs a new immutable session with fresh nonce/replay state.
// It inserts the session into the ring with epoch = Current()+1.
// Returns the new epoch value.
func (c *EpochUdpCrypto) Rekey(sendKey, recvKey []byte) (uint16, error) {
	c.rekeyMu.Lock()
	defer c.rekeyMu.Unlock()

	sendCipher, err := chacha20poly1305.New(sendKey)
	if err != nil {
		return 0, fmt.Errorf("rekey: build send cipher: %w", err)
	}
	recvCipher, err := chacha20poly1305.New(recvKey)
	if err != nil {
		return 0, fmt.Errorf("rekey: build recv cipher: %w", err)
	}

	nextEpoch := c.ring.Current() + 1
	newSession := NewUdpSessionWithCiphers(c.sessionId, sendCipher, recvCipher, c.isServer, nextEpoch)
	c.ring.Insert(nextEpoch, newSession)
	return uint16(nextEpoch), nil
}

// SetSendEpoch switches the epoch used for outbound encryption.
func (c *EpochUdpCrypto) SetSendEpoch(epoch uint16) {
	c.mu.Lock()
	c.sendEpoch = Epoch(epoch)
	c.mu.Unlock()
}
