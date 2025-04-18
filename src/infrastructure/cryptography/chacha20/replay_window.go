package chacha20

import "sync"

type ReplayWindow struct {
	mu     sync.Mutex
	max    uint64
	bitmap uint64
}

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
