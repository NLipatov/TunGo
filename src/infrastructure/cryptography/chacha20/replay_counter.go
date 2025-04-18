package chacha20

import (
	"encoding/binary"
	"sync"
)

type StrictCounter struct {
	mu      sync.Mutex
	maxHigh uint32
	maxLow  uint64
}

func NewStrictCounter() *StrictCounter { return &StrictCounter{} }

func (c *StrictCounter) Validate(nonce [12]byte) error {
	low := binary.BigEndian.Uint64(nonce[0:8])
	high := binary.BigEndian.Uint32(nonce[8:12])

	c.mu.Lock()
	defer c.mu.Unlock()

	if high < c.maxHigh ||
		(high == c.maxHigh && low <= c.maxLow) {
		return ErrNonUniqueNonce
	}

	c.maxHigh, c.maxLow = high, low
	return nil
}

type sliding64 struct {
	max    uint64
	bitmap uint64
}

type Sliding64 struct {
	mu  sync.Mutex
	win map[uint32]*sliding64
}

func NewSliding64() *Sliding64 {
	return &Sliding64{win: make(map[uint32]*sliding64, 1)}
}

func (s *Sliding64) Validate(nonce [12]byte) error {
	low := binary.BigEndian.Uint64(nonce[0:8])
	high := binary.BigEndian.Uint32(nonce[8:12])

	s.mu.Lock()
	defer s.mu.Unlock()

	w, ok := s.win[high]
	if !ok {
		w = &sliding64{}
		s.win[high] = w
	}

	switch {
	case low > w.max:
		shift := low - w.max
		if shift >= 64 {
			w.bitmap = 0
		} else {
			w.bitmap = (w.bitmap << shift) | 1
		}
		w.max = low
		return nil

	case w.max-low >= 64:
		return ErrNonUniqueNonce

	default:
		bit := uint64(1) << (w.max - low)
		if w.bitmap&bit != 0 {
			return ErrNonUniqueNonce
		}
		w.bitmap |= bit
		return nil
	}
}
