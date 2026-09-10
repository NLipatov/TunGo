package iolifecycle

import "sync/atomic"

// closingMask occupies the highest bit; the lower 63 bits count active I/O.
// Its bit pattern is 1 followed by 63 zeroes: 1000...0000.
const closingMask = int64(-1 << 63)

type Gate struct {
	state   atomic.Int64
	drained chan struct{}
}

func New() Gate {
	return Gate{drained: make(chan struct{})}
}

func (g *Gate) TryAcquire() bool {
	for {
		state := g.state.Load()
		if state&closingMask != 0 {
			return false
		}
		if g.state.CompareAndSwap(state, state+1) {
			return true
		}
	}
}

func (g *Gate) Release() {
	// The last release while closing leaves only closingMask in state.
	if g.state.Add(-1) == closingMask {
		close(g.drained)
	}
}

func (g *Gate) Closing() bool {
	return g.state.Load()&closingMask != 0
}

func (g *Gate) Drain(unblockIO func()) {
	// Or atomically sets the highest bit (the closing bit) and returns the previous state.
	// Zero means no active I/O; a negative value means draining has already started.
	if g.state.Or(closingMask) <= 0 {
		return
	}
	if unblockIO != nil {
		unblockIO()
	}
	<-g.drained
}
