package noise

import (
	"sync/atomic"
	"time"
)

const (
	// DefaultLoadThreshold is the default handshakes per second before triggering cookies.
	DefaultLoadThreshold = 1000
)

// LoadMonitor tracks handshake rate and determines if the server is under load.
// When under load, the server requires valid MAC2 (cookie) before performing DH.
type LoadMonitor struct {
	handshakesPerSecond atomic.Int64
	threshold           atomic.Int64
	counter             atomic.Int64
	lastResetTime       atomic.Int64
}

// NewLoadMonitor creates a new LoadMonitor with the given threshold.
func NewLoadMonitor(threshold int64) *LoadMonitor {
	if threshold <= 0 {
		threshold = DefaultLoadThreshold
	}
	lm := &LoadMonitor{}
	lm.threshold.Store(threshold)
	lm.lastResetTime.Store(time.Now().Unix())
	return lm
}

// RecordHandshake records a handshake attempt and updates the rate.
func (lm *LoadMonitor) RecordHandshake() {
	now := time.Now().Unix()
	lastReset := lm.lastResetTime.Load()

	// Reset counter every second
	if now > lastReset {
		if lm.lastResetTime.CompareAndSwap(lastReset, now) {
			rate := lm.counter.Swap(0)
			lm.handshakesPerSecond.Store(rate)
		}
	}

	lm.counter.Add(1)
}

// UnderLoad returns true if the server is receiving handshakes above the threshold.
func (lm *LoadMonitor) UnderLoad() bool {
	return lm.handshakesPerSecond.Load() > lm.threshold.Load()
}

// HandshakesPerSecond returns the current handshake rate.
func (lm *LoadMonitor) HandshakesPerSecond() int64 {
	return lm.handshakesPerSecond.Load()
}

// SetThreshold updates the load threshold.
func (lm *LoadMonitor) SetThreshold(threshold int64) {
	lm.threshold.Store(threshold)
}
