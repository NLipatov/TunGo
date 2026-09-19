package core

import (
	"encoding/binary"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
)

// Nonce structure:
// [Low (8 bytes)][High (2 bytes)][Epoch (2 bytes)]
// Total 12 bytes
const (
	NonceEpochOffset = 10
	NonceHighOffset  = 8
	NonceLowOffset   = 0
)

// Nonce represents an epoch-suffixed counter:
// | 0..7 counterLow | 8..9 counterHigh | 10..11 epoch |
// At epoch=0 the wire format is byte-identical to the pre-epoch nonce layout.
// Epoch is immutable per session. Counter is per-session monotonic.
// Not concurrency-safe by design; each session owns a single instance.
type Nonce struct {
	epoch       uint16
	CounterLow  uint64
	CounterHigh uint16
}

func NewNonce(epoch uint16) *Nonce {
	return &Nonce{
		epoch: epoch,
	}
}

func (n *Nonce) Increment() error {
	// Ensure counter does not overflow.
	if n.CounterHigh == ^uint16(0) && n.CounterLow == ^uint64(0) {
		return fmt.Errorf("nonce overflow: maximum number of messages reached")
	}

	if n.CounterLow == ^uint64(0) {
		n.CounterHigh++
		n.CounterLow = 0
	} else {
		n.CounterLow++
	}

	return nil
}

// PeekEncode computes the next nonce value (without incrementing the receiver)
// and encodes it directly into buf. Returns buf as a convenience.
// Zero allocation — avoids the heap-allocated *Nonce that peek() required.
func (n *Nonce) PeekEncode(buf []byte) ([]byte, error) {
	if n.CounterHigh == ^uint16(0) && n.CounterLow == ^uint64(0) {
		return nil, fmt.Errorf("nonce overflow: maximum number of messages reached")
	}

	if n.CounterLow == ^uint64(0) {
		binary.BigEndian.PutUint64(buf[NonceLowOffset:NonceHighOffset], 0)
		binary.BigEndian.PutUint16(buf[NonceHighOffset:NonceEpochOffset], n.CounterHigh+1)
	} else {
		binary.BigEndian.PutUint64(buf[NonceLowOffset:NonceHighOffset], n.CounterLow+1)
		binary.BigEndian.PutUint16(buf[NonceHighOffset:NonceEpochOffset], n.CounterHigh)
	}
	binary.BigEndian.PutUint16(buf[NonceEpochOffset:chacha20poly1305.NonceSize], uint16(n.epoch))

	return buf, nil
}

func (n *Nonce) Encode(buffer []byte) []byte {
	binary.BigEndian.PutUint64(buffer[NonceLowOffset:NonceHighOffset], n.CounterLow)
	binary.BigEndian.PutUint16(buffer[NonceHighOffset:NonceEpochOffset], n.CounterHigh)
	binary.BigEndian.PutUint16(buffer[NonceEpochOffset:chacha20poly1305.NonceSize], uint16(n.epoch))
	return buffer
}

// Zeroize zeros the nonce state.
func (n *Nonce) Zeroize() {
	n.epoch = 0
	n.CounterLow = 0
	n.CounterHigh = 0
}
