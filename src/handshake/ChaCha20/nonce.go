package ChaCha20

import (
	"crypto/sha256"
	"encoding/binary"
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

	return nonce, highVal, lowVal, nil
}

func Encode(high uint32, low uint64) []byte {
	nonce := make([]byte, 12)
	for i := 0; i < 8; i++ {
		nonce[i] = byte(low >> (8 * i))
	}
	for i := 0; i < 4; i++ {
		nonce[8+i] = byte(high >> (8 * i))
	}

	return nonce
}

func (n *Nonce) Hash() uint64 {
	hash := sha256.Sum256([]byte{
		byte(n.High >> 24), byte(n.High >> 16), byte(n.High >> 8), byte(n.High),
		byte(n.Low >> 56), byte(n.Low >> 48), byte(n.Low >> 40), byte(n.Low >> 32),
		byte(n.Low >> 24), byte(n.Low >> 16), byte(n.Low >> 8), byte(n.Low),
	})
	return binary.LittleEndian.Uint64(hash[:8])
}
