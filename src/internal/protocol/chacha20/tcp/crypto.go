package tcp

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

// EpochPrefixSize is the number of bytes prepended to every encrypted frame.
const EpochPrefixSize = 2

type epochSession struct {
	epoch   uint16
	session *Session
}

// Crypto manages the send session and the two receive sessions needed during
// an epoch transition.
//
// Every encrypted frame is prefixed with a 2-byte epoch tag:
//
//	[2B epoch BE] [ciphertext + Poly1305 tag]
type Crypto struct {
	mu           sync.RWMutex
	recvNewest   epochSession
	recvPrevious epochSession
	send         epochSession
	sessionId    [32]byte
	isServer     bool
	epochCounter uint16
}

func NewCrypto(id [32]byte, sendCipher, recvCipher cipher.AEAD, isServer bool) *Crypto {
	session := newSessionWithCiphers(id, sendCipher, recvCipher, isServer, 0)
	initial := epochSession{epoch: 0, session: session}

	return &Crypto{
		recvNewest: initial,
		send:       initial,
		sessionId:  id,
		isServer:   isServer,
	}
}

// Encrypt encrypts the data portion of the buffer and prepends the epoch.
//
// Buffer layout contract:
//
//	input:  [ 2B epoch reserved ][ plaintext (n bytes) ][ Overhead capacity ]
//	output: [ 2B epoch          ][ ciphertext (n + 16 bytes)                ]
func (c *Crypto) Encrypt(buf []byte) ([]byte, error) {
	if len(buf) < EpochPrefixSize {
		return nil, fmt.Errorf("buffer too short for epoch prefix: len=%d", len(buf))
	}
	if cap(buf) < len(buf)+chacha20poly1305.Overhead {
		return nil, fmt.Errorf(
			"insufficient capacity for epoch-prefixed encryption: cap=%d, need>=%d",
			cap(buf),
			len(buf)+chacha20poly1305.Overhead,
		)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	send := c.send
	if send.session == nil {
		return nil, fmt.Errorf("no active send session")
	}

	ciphertext, err := send.session.Encrypt(buf[EpochPrefixSize:])
	if err != nil {
		return nil, err
	}

	binary.BigEndian.PutUint16(buf[:EpochPrefixSize], send.epoch)
	return buf[:EpochPrefixSize+len(ciphertext)], nil
}

func (c *Crypto) Decrypt(data []byte) ([]byte, error) {
	if len(data) < EpochPrefixSize {
		return nil, fmt.Errorf("frame too short for epoch header")
	}
	epoch := binary.BigEndian.Uint16(data[:EpochPrefixSize])

	c.mu.RLock()
	defer c.mu.RUnlock()

	session := c.receiveSessionForEpoch(epoch)
	if session == nil {
		return nil, ErrUnknownEpoch
	}

	return session.Decrypt(data[EpochPrefixSize:])
}

func (c *Crypto) receiveSessionForEpoch(epoch uint16) *Session {
	if c.recvNewest.session != nil && epoch == c.recvNewest.epoch {
		return c.recvNewest.session
	}
	if c.recvPrevious.session != nil && epoch == c.recvPrevious.epoch {
		return c.recvPrevious.session
	}
	return nil
}

// StageEpoch installs a new receive session while keeping the previous session for
// in-flight frames. PromoteSendEpoch activates it for outbound traffic.
func (c *Crypto) StageEpoch(c2s, s2c []byte) (uint16, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.recvPrevious.session != nil {
		return 0, fmt.Errorf("rekey refused: previous epoch not retired")
	}
	if c.epochCounter >= core.MaxEpoch {
		return 0, chacha20.ErrEpochExhausted
	}

	sendKey, recvKey := c2s, s2c
	if c.isServer {
		sendKey, recvKey = recvKey, sendKey
	}
	sendCipher, err := chacha20poly1305.New(sendKey)
	if err != nil {
		return 0, err
	}
	recvCipher, err := chacha20poly1305.New(recvKey)
	if err != nil {
		return 0, err
	}

	c.epochCounter++
	newest := epochSession{
		epoch: c.epochCounter,
		session: newSessionWithCiphers(
			c.sessionId,
			sendCipher,
			recvCipher,
			c.isServer,
			c.epochCounter,
		),
	}
	c.recvPrevious = c.recvNewest
	c.recvNewest = newest

	return newest.epoch, nil
}

// PromoteSendEpoch switches outbound encryption to a staged epoch.
func (c *Crypto) PromoteSendEpoch(epoch uint16) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.recvNewest.session != nil && c.recvNewest.epoch == epoch {
		c.send = c.recvNewest
	}
}

// RetirePreviousEpoch removes a previous epoch after local send and authenticated peer
// traffic have both advanced to the newest epoch.
func (c *Crypto) RetirePreviousEpoch() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.recvPrevious.session != nil &&
		c.send == c.recvNewest {
		c.recvPrevious.session.zeroize()
		c.recvPrevious = epochSession{}
		return true
	}
	return false
}

// Zeroize overwrites the session metadata that can be explicitly cleared.
func (c *Crypto) Zeroize() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.recvNewest.session != nil {
		c.recvNewest.session.zeroize()
	}
	if c.recvPrevious.session != nil {
		c.recvPrevious.session.zeroize()
	}
	securemem.ZeroBytes(c.sessionId[:])
}
