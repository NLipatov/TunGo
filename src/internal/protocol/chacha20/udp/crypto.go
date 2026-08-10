package udp

import (
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"sync"
	"tungo/internal/protocol/chacha20"
	"tungo/internal/protocol/chacha20/internal/core"
	"tungo/internal/protocol/securemem"

	"golang.org/x/crypto/chacha20poly1305"
)

const defaultEpochRingCapacity = 4

// Crypto manages immutable UDP sessions and resolves them via an EpochRing.
// It holds no raw keys or handshake state.
type Crypto struct {
	ring         EpochRing
	isServer     bool
	sessionId    [32]byte
	routeID      uint64
	mu           sync.RWMutex
	rekeyMu      sync.Mutex
	sendEpoch    uint16
	epochCounter uint16
}

func NewCrypto(
	sessionId [32]byte,
	sendCipher, recvCipher cipher.AEAD,
	isServer bool,
) *Crypto {
	const initialEpoch uint16 = 0
	initialSession := newSessionWithCiphers(sessionId, sendCipher, recvCipher, isServer, initialEpoch)

	return &Crypto{
		ring:      NewEpochRing(defaultEpochRingCapacity, initialEpoch, initialSession),
		isServer:  isServer,
		sessionId: sessionId,
		routeID:   RouteIDFromSessionID(sessionId),
		sendEpoch: initialEpoch,
	}
}

func (c *Crypto) Encrypt(plaintext []byte) ([]byte, error) {
	if len(plaintext) < RouteIDLength+chacha20poly1305.NonceSize {
		return nil, fmt.Errorf("buffer too short for route-id+nonce prefix: %d", len(plaintext))
	}

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
	// Layout contract for UDP encrypt input:
	// [8B route-id reserved][12B nonce reserved][payload]
	//
	// The session encryptor works over the nonce+payload segment.
	encrypted, err := session.Encrypt(plaintext[RouteIDLength:])
	if err != nil {
		return nil, err
	}
	binary.BigEndian.PutUint64(plaintext[:RouteIDLength], c.routeID)
	return plaintext[:RouteIDLength+len(encrypted)], nil
}

func (c *Crypto) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < MinPacketSize {
		return nil, fmt.Errorf("cipher too short: %d", len(ciphertext))
	}
	routeID, ok := ReadRouteID(ciphertext)
	if !ok {
		return nil, fmt.Errorf("cipher too short: %d", len(ciphertext))
	}
	if routeID != c.routeID {
		return nil, ErrUnknownRouteID
	}

	cipherPayload := ciphertext[NonceOffset:]
	epoch := binary.BigEndian.Uint16(cipherPayload[NonceEpochOffset : NonceEpochOffset+2])
	session, ok := c.ring.Resolve(epoch)
	if !ok {
		return nil, ErrUnknownEpoch
	}
	if session.Epoch() != epoch {
		return nil, ErrUnknownEpoch
	}
	return session.Decrypt(cipherPayload)
}

// StageEpoch installs a new immutable session with fresh nonce/replay state.
// It inserts the session into the ring with a monotonically allocated epoch.
// Returns the new epoch value.
func (c *Crypto) StageEpoch(c2s, s2c []byte) (uint16, error) {
	c.rekeyMu.Lock()
	defer c.rekeyMu.Unlock()
	if c.epochCounter >= core.MaxEpoch {
		return 0, chacha20.ErrEpochExhausted
	}
	nextEpoch := c.epochCounter + 1
	// Protect against evicting the active send epoch when the ring is full.
	sendEpoch := c.currentSendEpoch()
	if oldest, ok := c.ring.Oldest(); ok &&
		c.ring.Len() == c.ring.Capacity() &&
		oldest == sendEpoch {
		// SECURITY (R-19): Generic error to avoid revealing epoch state.
		// Detailed reason: active send epoch would be evicted before confirmation.
		return 0, fmt.Errorf("rekey refused: wait for confirmation before next rekey")
	}

	sendKey, recvKey := c2s, s2c
	if c.isServer {
		sendKey, recvKey = recvKey, sendKey
	}
	sendCipher, err := chacha20poly1305.New(sendKey)
	if err != nil {
		return 0, fmt.Errorf("rekey: build send cipher: %w", err)
	}
	recvCipher, err := chacha20poly1305.New(recvKey)
	if err != nil {
		return 0, fmt.Errorf("rekey: build recv cipher: %w", err)
	}
	newSession := newSessionWithCiphers(c.sessionId, sendCipher, recvCipher, c.isServer, nextEpoch)
	c.ring.Insert(nextEpoch, newSession)
	c.epochCounter = nextEpoch
	// sendEpoch is intentionally NOT updated here.
	return nextEpoch, nil
}

// PromoteSendEpoch switches outbound encryption to a staged epoch.
func (c *Crypto) PromoteSendEpoch(epoch uint16) {
	c.mu.Lock()
	c.sendEpoch = epoch
	c.mu.Unlock()
}

func (c *Crypto) currentSendEpoch() uint16 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sendEpoch
}

func (c *Crypto) RouteID() uint64 {
	return c.routeID
}

// RetirePreviousEpoch acknowledges the completed logical transition. UDP keeps older
// sessions in its bounded ring to accept reordered datagrams and retires them
// through normal ring eviction.
func (c *Crypto) RetirePreviousEpoch() bool { return true }

// Zeroize overwrites all key material with zeros.
// After this call, the crypto instance is unusable.
// SECURITY INVARIANT: All session keys in the EpochRing are zeroed.
// This is guaranteed by the EpochRing interface (ZeroizeAll is mandatory).
func (c *Crypto) Zeroize() {
	c.rekeyMu.Lock()
	defer c.rekeyMu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()

	// Zero the session ID
	securemem.ZeroBytes(c.sessionId[:])

	// Zero all sessions in the ring.
	// ZeroizeAll is part of EpochRing interface - no type assertion needed.
	c.ring.ZeroizeAll()
}
