//go:build unix

package resume

import (
	"context"
	"time"
)

func Watch(ctx context.Context) <-chan struct{} {
	out := make(chan struct{})
	ticker := time.NewTicker(time.Second)
	prev := time.Now()
	go func() {
		defer func() {
			ticker.Stop()
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cur := time.Now()
				monotonicElapsed := cur.Sub(prev)
				wallElapsed := time.Duration(
					cur.UnixNano() - prev.UnixNano(),
				)
				sleepGap := wallElapsed - monotonicElapsed
				if sleepGap > time.Second {
					close(out)
					return
				}
				prev = cur
			}
		}
	}()
	return out
}
