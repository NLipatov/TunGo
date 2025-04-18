package chacha20

import (
	"encoding/binary"
	"errors"
	"sync"
)

type NonceCounter struct {
	mu      sync.Mutex
	maxHigh uint32
	maxLow  uint64
}

// NewNonceCounter — GC‑free anti-replay counter
func NewNonceCounter() *NonceCounter { return &NonceCounter{} }

// Insert checks 96‑bit counter BE (low||high).
func (c *NonceCounter) Insert(nonce [12]byte) error {
	low := binary.BigEndian.Uint64(nonce[0:8])
	high := binary.BigEndian.Uint32(nonce[8:12])

	if high == ^uint32(0) && low == ^uint64(0) {
		return errors.New("nonce overflow: maximum messages reached")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if high < c.maxHigh ||
		(high == c.maxHigh && low <= c.maxLow) {
		return ErrNonUniqueNonce
	}

	c.maxHigh, c.maxLow = high, low
	return nil
}
