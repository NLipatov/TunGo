package udp

import (
	"sync/atomic"
	"time"
)

type activityTracker struct {
	startedAt     time.Time
	lastElapsedMs atomic.Int64
}

func newActivityTracker() *activityTracker {
	return &activityTracker{
		startedAt: time.Now(),
	}
}

func (m *activityTracker) Touch() {
	m.lastElapsedMs.Store(time.Since(m.startedAt).Milliseconds())
}

func (m *activityTracker) IdleFor() time.Duration {
	last := m.lastElapsedMs.Load()
	now := time.Since(m.startedAt).Milliseconds()
	return time.Duration(now-last) * time.Millisecond
}
