//go:build windows

package resume

import (
	"context"
	"time"
	"tungo/internal/client/stopwatch"
)

const (
	interval = time.Second
	maxGap   = time.Second
)

func Watch(ctx context.Context) <-chan struct{} {
	out := make(chan struct{})
	ticker := time.NewTicker(interval)
	sw := stopwatch.New()
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				gap := sw.Elapsed() - interval
				if gap > maxGap {
					close(out)
					return
				}
				sw.Reset()
			}
		}
	}()
	return out
}
