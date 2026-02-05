package chacha20

import (
	"encoding/binary"
	"fmt"
)

// NonceEpochOffset is the byte offset of the epoch field within the 12-byte nonce.
const NonceEpochOffset = 10

// Nonce represents an epoch-suffixed counter:
// | 0..7 counterLow | 8..9 counterHigh | 10..11 epoch |
// At epoch=0 the wire format is byte-identical to the pre-epoch nonce layout.
// Epoch is immutable per session. Counter is per-session monotonic.
// Not concurrency-safe by design; each session owns a single instance.
type Nonce struct {
	epoch       Epoch
	counterLow  uint64
	counterHigh uint16
}

func NewNonce(epoch Epoch) *Nonce {
	return &Nonce{
		epoch: epoch,
	}
}

func (n *Nonce) incrementNonce() error {
	// Ensure counter does not overflow.
	if n.counterHigh == ^uint16(0) && n.counterLow == ^uint64(0) {
		return fmt.Errorf("nonce overflow: maximum number of messages reached")
	}

	if n.counterLow == ^uint64(0) {
		n.counterHigh++
		n.counterLow = 0
	} else {
		n.counterLow++
	}

	return nil
}

// peek returns a copy of the nonce with the counter incremented,
// without modifying the original. Used for tentative decryption
// where we only commit the increment after successful validation.
func (n *Nonce) peek() (*Nonce, error) {
	// Check for overflow
	if n.counterHigh == ^uint16(0) && n.counterLow == ^uint64(0) {
		return nil, fmt.Errorf("nonce overflow: maximum number of messages reached")
	}

	peeked := &Nonce{epoch: n.epoch}
	if n.counterLow == ^uint64(0) {
		peeked.counterHigh = n.counterHigh + 1
		peeked.counterLow = 0
	} else {
		peeked.counterHigh = n.counterHigh
		peeked.counterLow = n.counterLow + 1
	}

	return peeked, nil
}

func (n *Nonce) Encode(buffer []byte) []byte {
	binary.BigEndian.PutUint64(buffer[0:8], n.counterLow)
	binary.BigEndian.PutUint16(buffer[8:10], n.counterHigh)
	binary.BigEndian.PutUint16(buffer[10:12], uint16(n.epoch))
	return buffer
}

// Zeroize zeros the nonce state.
func (n *Nonce) Zeroize() {
	n.epoch = 0
	n.counterLow = 0
	n.counterHigh = 0
}
