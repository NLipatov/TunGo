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

// peekEncode computes the next nonce value (without incrementing the receiver)
// and encodes it directly into buf. Returns buf as a convenience.
// Zero allocation — avoids the heap-allocated *Nonce that peek() required.
func (n *Nonce) peekEncode(buf []byte) ([]byte, error) {
	if n.counterHigh == ^uint16(0) && n.counterLow == ^uint64(0) {
		return nil, fmt.Errorf("nonce overflow: maximum number of messages reached")
	}

	if n.counterLow == ^uint64(0) {
		binary.BigEndian.PutUint64(buf[0:8], 0)
		binary.BigEndian.PutUint16(buf[8:10], n.counterHigh+1)
	} else {
		binary.BigEndian.PutUint64(buf[0:8], n.counterLow+1)
		binary.BigEndian.PutUint16(buf[8:10], n.counterHigh)
	}
	binary.BigEndian.PutUint16(buf[10:12], uint16(n.epoch))

	return buf, nil
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
