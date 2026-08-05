package udp

import "sync"

// EpochRing manages a fixed-capacity ring of UDP sessions indexed by epoch.
//
// SECURITY INVARIANT: ZeroizeAll MUST be called when the crypto instance is
// destroyed. This is enforced by the interface - implementations cannot omit it.
type EpochRing interface {
	Current() uint16
	Resolve(epoch uint16) (*Session, bool)
	Insert(epoch uint16, session *Session)
	ResolveCurrent() (*Session, bool)
	Oldest() (uint16, bool)
	Len() int
	Capacity() int
	Remove(epoch uint16) bool
	// ZeroizeAll zeros all sessions in the ring.
	// MUST be called during crypto teardown to ensure key material is cleared.
	// After this call, all sessions in the ring are unusable.
	ZeroizeAll()
}

type epochEntry struct {
	epoch   uint16
	session *Session
}

// defaultEpochRing is a fixed-capacity FIFO ring safe for concurrent Resolve calls.
// Insert may evict the oldest entry when capacity is exceeded.
type defaultEpochRing struct {
	mu       sync.RWMutex
	capacity int
	entries  []epochEntry
}

func NewEpochRing(capacity int, initialEpoch uint16, initial *Session) EpochRing {
	r := &defaultEpochRing{
		capacity: capacity,
	}
	if initial != nil {
		r.entries = append(r.entries, epochEntry{epoch: initialEpoch, session: initial})
	}
	return r
}

func (r *defaultEpochRing) Current() uint16 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.entries) == 0 {
		return 0
	}
	return r.entries[len(r.entries)-1].epoch
}

func (r *defaultEpochRing) Resolve(epoch uint16) (*Session, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, e := range r.entries {
		if e.epoch == epoch {
			return e.session, true
		}
	}
	return nil, false
}

func (r *defaultEpochRing) Insert(epoch uint16, session *Session) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.entries) == r.capacity {
		// Evict oldest; zero key material before releasing.
		if r.entries[0].session != nil {
			r.entries[0].session.Zeroize()
		}
		r.entries = r.entries[1:]
	}
	r.entries = append(r.entries, epochEntry{epoch: epoch, session: session})
}

func (r *defaultEpochRing) ResolveCurrent() (*Session, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.entries) == 0 {
		return nil, false
	}
	return r.entries[len(r.entries)-1].session, true
}

func (r *defaultEpochRing) Oldest() (uint16, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.entries) == 0 {
		return 0, false
	}
	return r.entries[0].epoch, true
}

func (r *defaultEpochRing) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.entries)
}

func (r *defaultEpochRing) Capacity() int {
	return r.capacity
}

func (r *defaultEpochRing) Remove(epoch uint16) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, e := range r.entries {
		if e.epoch == epoch {
			if e.session != nil {
				e.session.Zeroize()
			}
			r.entries = append(r.entries[:i], r.entries[i+1:]...)
			return true
		}
	}
	return false
}

// ZeroizeAll zeros all sessions in the ring.
// Called during crypto teardown to ensure key material is cleared.
func (r *defaultEpochRing) ZeroizeAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.entries {
		if e.session != nil {
			e.session.Zeroize()
		}
	}
	r.entries = nil
}
