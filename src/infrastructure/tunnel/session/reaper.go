package session

import (
	"context"
	"time"

	"tungo/application/logging"
)

// RunIdleReaperLoop periodically removes sessions that have been idle
// for longer than timeout. It blocks until ctx is cancelled.
func RunIdleReaperLoop(ctx context.Context, reaper IdleReaper, timeout, interval time.Duration, logger logging.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n := reaper.ReapIdle(timeout); n > 0 {
				logger.Printf("reaped %d idle session(s)", n)
			}
		}
	}
}
