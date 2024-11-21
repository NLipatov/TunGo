package ChaCha20

import (
	"encoding/binary"
	"encoding/hex"
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

func (n *Nonce) incrementNonce() ([]byte, uint32, uint64, error) {
	// Ensure nonce does not overflow
	if atomic.LoadUint32(&n.High) == ^uint32(0) && atomic.LoadUint64(&n.Low) == ^uint64(0) {
		return nil, 0, 0, fmt.Errorf("nonce overflow: maximum number of messages reached")
	}

	if atomic.LoadUint64(&n.Low) == ^uint64(0) {
		atomic.AddUint32(&n.High, 1)
		atomic.StoreUint64(&n.Low, 0)
	} else {
		atomic.AddUint64(&n.Low, 1)
	}

	lowVal := atomic.LoadUint64(&n.Low)
	highVal := atomic.LoadUint32(&n.High)
	nonce := Encode(highVal, lowVal)

	return nonce[:], highVal, lowVal, nil
}

func (n *Nonce) Hash() string {
	lowVal := atomic.LoadUint64(&n.Low)
	highVal := atomic.LoadUint32(&n.High)
	nonce := Encode(highVal, lowVal)

	return hex.EncodeToString(nonce[:])
}

func Encode(high uint32, low uint64) [12]byte {
	var nonce [12]byte
	binary.BigEndian.PutUint64(nonce[:8], low)
	binary.BigEndian.PutUint32(nonce[8:], high)

	return nonce
}
