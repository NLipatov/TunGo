package chacha20

import "sync"

// Epoch is encoded into the nonce prefix. We use uint16 for a compact, fixed width field.
type Epoch uint16

type EpochRing interface {
	Current() Epoch
	Resolve(epoch Epoch) (*DefaultUdpSession, bool)
	Insert(epoch Epoch, session *DefaultUdpSession)
	ResolveCurrent() (*DefaultUdpSession, bool)
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
		// Evict oldest.
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
