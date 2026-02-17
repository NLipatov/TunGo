package chacha20

const slidingWindowWords = 16 // 16 × 64 = 1024-bit replay window

type slidingWindowEntry struct {
	max    uint64
	bitmap [slidingWindowWords]uint64
	high   uint16
}

const slidingWindowCap = 4

type SlidingWindow struct {
	wins []slidingWindowEntry
}

func NewSlidingWindow() *SlidingWindow {
	return &SlidingWindow{}
}

// Check returns nil if nonce would be accepted, without modifying state.
// Nonce bytes are pre-parsed by caller to avoid redundant decoding.
func (s *SlidingWindow) Check(low uint64, high uint16) error {
	var w *slidingWindowEntry
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
	case w.max-low >= slidingWindowWords*64:
		return ErrNonUniqueNonce
	default:
		diff := w.max - low
		word := diff / 64
		bit := uint64(1) << (diff % 64)
		if w.bitmap[word]&bit != 0 {
			return ErrNonUniqueNonce
		}
		return nil // Would be accepted
	}
}

// Accept commits the nonce to the window. Must be called only after
// decryption succeeds. Assumes Check returned nil for the same values.
func (s *SlidingWindow) Accept(low uint64, high uint16) {
	var w *slidingWindowEntry
	for i := range s.wins {
		if s.wins[i].high == high {
			w = &s.wins[i]
			break
		}
	}
	if w == nil {
		if len(s.wins) == slidingWindowCap {
			s.wins = s.wins[1:]
		}
		s.wins = append(s.wins, slidingWindowEntry{high: high})
		w = &s.wins[len(s.wins)-1]
	}

	switch {
	case low > w.max:
		shift := low - w.max
		shiftBitmap(&w.bitmap, shift)
		w.bitmap[0] |= 1
		w.max = low
	case w.max-low < slidingWindowWords*64:
		diff := w.max - low
		word := diff / 64
		bit := uint64(1) << (diff % 64)
		w.bitmap[word] |= bit
	}
}

// shiftBitmap shifts the multi-word bitmap left by n positions.
func shiftBitmap(b *[slidingWindowWords]uint64, n uint64) {
	if n >= slidingWindowWords*64 {
		*b = [slidingWindowWords]uint64{}
		return
	}
	wordShift := n / 64
	bitShift := n % 64
	if wordShift > 0 {
		copy(b[wordShift:], b[:slidingWindowWords-wordShift])
		for i := uint64(0); i < wordShift; i++ {
			b[i] = 0
		}
	}
	if bitShift > 0 {
		for i := slidingWindowWords - 1; i > 0; i-- {
			b[i] = (b[i] << bitShift) | (b[i-1] >> (64 - bitShift))
		}
		b[0] <<= bitShift
	}
}

// Zeroize clears all replay window state.
//
// SECURITY INVARIANT: Nonce history is security-sensitive metadata.
// Clearing reduces forensic exposure of packet sequence patterns.
// Must be called during session teardown.
func (s *SlidingWindow) Zeroize() {
	for i := range s.wins {
		s.wins[i].max = 0
		s.wins[i].bitmap = [slidingWindowWords]uint64{}
		s.wins[i].high = 0
	}
	s.wins = nil
}
