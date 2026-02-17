package chacha20

import "sync"

// Epoch is encoded into the nonce prefix. We use uint16 for a compact, fixed width field.
type Epoch uint16

// EpochRing manages a fixed-capacity ring of UDP sessions indexed by epoch.
//
// SECURITY INVARIANT: ZeroizeAll MUST be called when the crypto instance is
// destroyed. This is enforced by the interface - implementations cannot omit it.
type EpochRing interface {
	Current() Epoch
	Resolve(epoch Epoch) (*DefaultUdpSession, bool)
	Insert(epoch Epoch, session *DefaultUdpSession)
	ResolveCurrent() (*DefaultUdpSession, bool)
	Oldest() (Epoch, bool)
	Len() int
	Capacity() int
	Remove(epoch Epoch) bool
	// ZeroizeAll zeros all sessions in the ring.
	// MUST be called during crypto teardown to ensure key material is cleared.
	// After this call, all sessions in the ring are unusable.
	ZeroizeAll()
}

type epochEntry struct {
	epoch   Epoch
	session *DefaultUdpSession
}

// defaultEpochRing is a fixed-capacity FIFO ring safe for concurrent Resolve calls.
// Insert may evict the oldest entry when capacity is exceeded.
type defaultEpochRing struct {
	mu       sync.RWMutex
	capacity int
	entries  []epochEntry
}

func NewEpochRing(capacity int, initialEpoch Epoch, initial *DefaultUdpSession) EpochRing {
	r := &defaultEpochRing{
		capacity: capacity,
	}
	if initial != nil {
		r.entries = append(r.entries, epochEntry{epoch: initialEpoch, session: initial})
	}
	return r
}

func (r *defaultEpochRing) Current() Epoch {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.entries) == 0 {
		return 0
	}
	return r.entries[len(r.entries)-1].epoch
}

func (r *defaultEpochRing) Resolve(epoch Epoch) (*DefaultUdpSession, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, e := range r.entries {
		if e.epoch == epoch {
			return e.session, true
		}
	}
	return nil, false
}

func (r *defaultEpochRing) Insert(epoch Epoch, session *DefaultUdpSession) {
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

func (r *defaultEpochRing) ResolveCurrent() (*DefaultUdpSession, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.entries) == 0 {
		return nil, false
	}
	return r.entries[len(r.entries)-1].session, true
}

func (r *defaultEpochRing) Oldest() (Epoch, bool) {
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

func (r *defaultEpochRing) Remove(epoch Epoch) bool {
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
