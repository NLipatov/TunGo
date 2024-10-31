package ChaCha20

import (
	"fmt"
	"sync/atomic"
)

type Nonce struct {
	Low  uint64
	High uint32
}

func NewNonce() *Nonce {
	return &Nonce{Low: 0, High: 0}
}

func (n *Nonce) incrementNonce() ([]byte, error) {
	// Ensure nonce does not overflow
	if atomic.LoadUint32(&n.High) == ^uint32(0) && atomic.LoadUint64(&n.Low) == ^uint64(0) {
		return nil, fmt.Errorf("nonce overflow: maximum number of messages reached")
	}

	nonce := make([]byte, 12)

	if atomic.LoadUint64(&n.Low) == ^uint64(0) {
		atomic.AddUint32(&n.High, 1)
		atomic.StoreUint64(&n.Low, 0)
	} else {
		atomic.AddUint64(&n.Low, 1)
	}

	lowVal := atomic.LoadUint64(&n.Low)
	highVal := atomic.LoadUint32(&n.High)

	for i := 0; i < 8; i++ {
		nonce[i] = byte(lowVal >> (8 * i))
	}
	for i := 0; i < 4; i++ {
		nonce[8+i] = byte(highVal >> (8 * i))
	}

	return nonce, nil
}

func (n *Nonce) Hash() uint64 {
	return n.Low ^ uint64(n.High)
}
