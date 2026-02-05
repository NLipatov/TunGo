package chacha20

import "sync"

type ReplayWindow struct {
	mu     sync.Mutex
	max    uint64
	bitmap uint64
}

// Check returns nil if seq would be accepted, without modifying state.
// Call Accept after successful decryption to commit the update.
func (w *ReplayWindow) Check(seq uint64) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	switch {
	case seq > w.max:
		return nil // Would be accepted
	case w.max-seq >= 64:
		return ErrNonUniqueNonce
	default:
		bit := uint64(1) << (w.max - seq)
		if w.bitmap&bit != 0 {
			return ErrNonUniqueNonce
		}
		return nil // Would be accepted
	}
}

// Accept commits the sequence number to the window. Must be called
// only after decryption succeeds. Assumes Check(seq) returned nil.
func (w *ReplayWindow) Accept(seq uint64) {
	w.mu.Lock()
	defer w.mu.Unlock()

	switch {
	case seq > w.max:
		shift := seq - w.max
		if shift >= 64 {
			w.bitmap = 0
		} else {
			w.bitmap = (w.bitmap << shift) | 1
		}
		w.max = seq
	case w.max-seq < 64:
		bit := uint64(1) << (w.max - seq)
		w.bitmap |= bit
	}
}

// Validate checks and accepts in one call. Kept for backward compatibility
// but should not be used for UDP where decryption may fail.
func (w *ReplayWindow) Validate(seq uint64) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	switch {
	case seq > w.max:
		shift := seq - w.max
		if shift >= 64 {
			w.bitmap = 0
		} else {
			w.bitmap = (w.bitmap << shift) | 1
		}
		w.max = seq
		return nil

	case w.max-seq >= 64:
		return ErrNonUniqueNonce
	default:
		bit := uint64(1) << (w.max - seq)
		if w.bitmap&bit != 0 {
			return ErrNonUniqueNonce
		}
		w.bitmap |= bit
		return nil
	}
}
