package chacha20

import (
	"encoding/binary"
	"encoding/hex"
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

func (n *Nonce) Hash() string {
	nonce := n.Encode()

	return hex.EncodeToString(nonce[:])
}

func (n *Nonce) Encode() []byte {
	var nonce [12]byte
	binary.BigEndian.PutUint64(nonce[:8], n.low)
	binary.BigEndian.PutUint32(nonce[8:], n.high)

	return nonce[:]
}

func (n *Nonce) InplaceEncode(data []byte) error {
	binary.BigEndian.PutUint64(data[:8], n.low)
	binary.BigEndian.PutUint32(data[8:], n.high)
	return nil
}
