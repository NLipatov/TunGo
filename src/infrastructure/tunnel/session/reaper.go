package session

import (
	"context"
	"log/slog"
	"time"
)

// RunIdleReaperLoop periodically removes sessions that have been idle
// for longer than timeout. It blocks until ctx is cancelled.
func RunIdleReaperLoop(
	ctx context.Context,
	reap func(time.Duration) int,
	timeout, interval time.Duration,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n := reap(timeout); n > 0 {
				slog.Info("reaped idle sessions", "count", n)
			}
		}
	}
}
