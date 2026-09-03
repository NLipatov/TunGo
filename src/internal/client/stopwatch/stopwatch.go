package stopwatch

import (
	"sync/atomic"
	"time"
)

// Stopwatch measures the time since its creation or last reset.
type Stopwatch struct {
	startedAt          time.Time
	lastResetElapsedMs atomic.Int64
}

func New() *Stopwatch {
	return &Stopwatch{
		startedAt: time.Now(),
	}
}

func (s *Stopwatch) Reset() {
	s.lastResetElapsedMs.Store(time.Since(s.startedAt).Milliseconds())
}

func (s *Stopwatch) Elapsed() time.Duration {
	lastReset := s.lastResetElapsedMs.Load()
	now := time.Since(s.startedAt).Milliseconds()
	return time.Duration(now-lastReset) * time.Millisecond
}
