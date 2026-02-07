package chacha20

import (
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"sync"
	"tungo/infrastructure/cryptography/mem"

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
	epoch := Epoch(binary.BigEndian.Uint16(ciphertext[NonceEpochOffset : NonceEpochOffset+2]))
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

	// Protect against evicting the active send epoch when the ring is full.
	sendEpoch := c.currentSendEpoch()
	if oldest, ok := c.ring.Oldest(); ok &&
		c.ring.Len() == c.ring.Capacity() &&
		oldest == sendEpoch {
		// SECURITY (R-19): Generic error to avoid revealing epoch state.
		// Detailed reason: active send epoch would be evicted before confirmation.
		return 0, fmt.Errorf("rekey refused: wait for confirmation before next rekey")
	}

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

func (c *EpochUdpCrypto) currentSendEpoch() Epoch {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sendEpoch
}

// RemoveEpoch removes a session for the specified epoch, if present.
// Returns true if removed.
func (c *EpochUdpCrypto) RemoveEpoch(epoch uint16) bool {
	// Never remove active send epoch; never remove last remaining entry.
	if Epoch(epoch) == c.currentSendEpoch() {
		return false
	}
	if c.ring.Len() <= 1 {
		return false
	}
	return c.ring.Remove(Epoch(epoch))
}

// Zeroize overwrites all key material with zeros.
// After this call, the crypto instance is unusable.
// Implements connection.CryptoZeroizer.
//
// SECURITY INVARIANT: All session keys in the EpochRing are zeroed.
// This is guaranteed by the EpochRing interface (ZeroizeAll is mandatory).
func (c *EpochUdpCrypto) Zeroize() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rekeyMu.Lock()
	defer c.rekeyMu.Unlock()

	// Zero the session ID
	mem.ZeroBytes(c.sessionId[:])

	// Zero all sessions in the ring.
	// ZeroizeAll is part of EpochRing interface - no type assertion needed.
	c.ring.ZeroizeAll()
}
