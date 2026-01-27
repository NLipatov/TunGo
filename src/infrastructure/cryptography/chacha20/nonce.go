package chacha20

import (
	"encoding/binary"
	"fmt"
)

// Nonce represents an epoch-prefixed counter:
// | 0..1 epoch | 2..3 counterHigh | 4..11 counterLow |
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

func (n *Nonce) Encode(buffer []byte) []byte {
	binary.BigEndian.PutUint16(buffer[0:2], uint16(n.epoch))
	binary.BigEndian.PutUint16(buffer[2:4], n.counterHigh)
	binary.BigEndian.PutUint64(buffer[4:12], n.counterLow)
	return buffer
}
