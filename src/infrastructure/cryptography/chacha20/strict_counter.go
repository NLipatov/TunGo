package chacha20

import (
	"encoding/binary"
	"sync"

	"golang.org/x/crypto/chacha20poly1305"
)

type StrictCounter struct {
	mu      sync.Mutex
	maxHigh uint16
	maxLow  uint64
}

func NewStrictCounter() *StrictCounter { return &StrictCounter{} }

func (c *StrictCounter) Validate(nonce [chacha20poly1305.NonceSize]byte) error {
	low := binary.BigEndian.Uint64(nonce[0:8])
	high := binary.BigEndian.Uint16(nonce[8:10])

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
	high   uint16
}

const sliding64Cap = 4

type Sliding64 struct {
	mu   sync.Mutex
	wins []sliding64
}

func NewSliding64() *Sliding64 {
	return &Sliding64{}
}

// Check returns nil if nonce would be accepted, without modifying state.
func (s *Sliding64) Check(nonce [chacha20poly1305.NonceSize]byte) error {
	low := binary.BigEndian.Uint64(nonce[0:8])
	high := binary.BigEndian.Uint16(nonce[8:10])

	s.mu.Lock()
	defer s.mu.Unlock()

	var w *sliding64
	for i := range s.wins {
		if s.wins[i].high == high {
			w = &s.wins[i]
			break
		}
	}
	if w == nil {
		return nil // New epoch window, would be accepted
	}

	switch {
	case low > w.max:
		return nil // Would be accepted
	case w.max-low >= 64:
		return ErrNonUniqueNonce
	default:
		bit := uint64(1) << (w.max - low)
		if w.bitmap&bit != 0 {
			return ErrNonUniqueNonce
		}
		return nil // Would be accepted
	}
}

// Accept commits the nonce to the window. Must be called only after
// decryption succeeds. Assumes Check(nonce) returned nil.
func (s *Sliding64) Accept(nonce [chacha20poly1305.NonceSize]byte) {
	low := binary.BigEndian.Uint64(nonce[0:8])
	high := binary.BigEndian.Uint16(nonce[8:10])

	s.mu.Lock()
	defer s.mu.Unlock()

	var w *sliding64
	for i := range s.wins {
		if s.wins[i].high == high {
			w = &s.wins[i]
			break
		}
	}
	if w == nil {
		if len(s.wins) == sliding64Cap {
			s.wins = s.wins[1:]
		}
		s.wins = append(s.wins, sliding64{high: high})
		w = &s.wins[len(s.wins)-1]
	}

	switch {
	case low > w.max:
		shift := low - w.max
		if shift >= 64 {
			w.bitmap = 1
		} else {
			w.bitmap = (w.bitmap << shift) | 1
		}
		w.max = low
	case w.max-low < 64:
		bit := uint64(1) << (w.max - low)
		w.bitmap |= bit
	}
}

// Validate checks and accepts in one call. Kept for backward compatibility.
func (s *Sliding64) Validate(nonce [chacha20poly1305.NonceSize]byte) error {
	low := binary.BigEndian.Uint64(nonce[0:8])
	high := binary.BigEndian.Uint16(nonce[8:10])

	s.mu.Lock()
	defer s.mu.Unlock()

	var w *sliding64
	for i := range s.wins {
		if s.wins[i].high == high {
			w = &s.wins[i]
			break
		}
	}
	if w == nil {
		if len(s.wins) == sliding64Cap {
			// evict oldest (index 0)
			s.wins = s.wins[1:]
		}
		s.wins = append(s.wins, sliding64{high: high})
		w = &s.wins[len(s.wins)-1]
	}

	switch {
	case low > w.max:
		shift := low - w.max
		if shift >= 64 {
			w.bitmap = 1
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

// Zeroize clears all replay window state.
//
// SECURITY INVARIANT: Nonce history is security-sensitive metadata.
// Clearing reduces forensic exposure of packet sequence patterns.
// Must be called during session teardown.
func (s *Sliding64) Zeroize() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.wins {
		s.wins[i].max = 0
		s.wins[i].bitmap = 0
		s.wins[i].high = 0
	}
	s.wins = nil
}
