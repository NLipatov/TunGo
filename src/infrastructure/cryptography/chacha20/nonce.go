package chacha20

import (
	"encoding/binary"
	"fmt"
	"sync"
)

type Nonce struct {
	low  uint64
	high uint32
	mu   sync.Mutex
}

func NewNonce() *Nonce {
	return &Nonce{low: 0, high: 0}
}

func (n *Nonce) incrementNonce() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Ensure nonce does not overflow
	if n.high == ^uint32(0) && n.low == ^uint64(0) {
		return fmt.Errorf("nonce overflow: maximum number of messages reached")
	}

	if n.low == ^uint64(0) {
		n.high++
		n.low = 0
	} else {
		n.low++
	}

	return nil
}

func (n *Nonce) Encode(buffer []byte) []byte {
	binary.BigEndian.PutUint64(buffer[:8], n.low)
	binary.BigEndian.PutUint32(buffer[8:], n.high)
	return buffer
}
