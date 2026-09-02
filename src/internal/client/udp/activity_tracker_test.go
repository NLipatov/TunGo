package udp

import (
	"sync"
	"testing"
	"time"
)

func setActivityTrackerIdleFor(tracker *activityTracker, idleFor time.Duration) {
	elapsedMs := time.Since(tracker.startedAt).Milliseconds()
	tracker.lastElapsedMs.Store(elapsedMs - idleFor.Milliseconds())
}

func TestActivityTrackerStartsAtCreation(t *testing.T) {
	tracker := newActivityTracker()

	idleFor := tracker.IdleFor()
	if idleFor < 0 || idleFor >= time.Minute {
		t.Fatalf("IdleFor() = %s, want duration since tracker creation", idleFor)
	}
}

func TestActivityTrackerTouchResetsIdleTime(t *testing.T) {
	tracker := newActivityTracker()
	const previousIdleTime = 1500 * time.Millisecond
	setActivityTrackerIdleFor(tracker, previousIdleTime)

	if idleFor := tracker.IdleFor(); idleFor < previousIdleTime {
		t.Fatalf("IdleFor() before Touch() = %s, want at least %s", idleFor, previousIdleTime)
	}

	tracker.Touch()
	if idleFor := tracker.IdleFor(); idleFor < 0 || idleFor >= time.Second {
		t.Fatalf("IdleFor() after Touch() = %s, want less than 1s", idleFor)
	}
}

func TestActivityTrackerConcurrentTouchAndIdleFor(t *testing.T) {
	tracker := newActivityTracker()
	start := make(chan struct{})
	negativeIdle := make(chan time.Duration, 1)

	const (
		workers    = 8
		iterations = 1000
	)
	var wg sync.WaitGroup
	wg.Add(workers)
	for worker := range workers {
		go func() {
			defer wg.Done()
			<-start
			for range iterations {
				if worker%2 == 0 {
					tracker.Touch()
					continue
				}
				if idleFor := tracker.IdleFor(); idleFor < 0 {
					select {
					case negativeIdle <- idleFor:
					default:
					}
				}
			}
		}()
	}

	close(start)
	wg.Wait()

	select {
	case idleFor := <-negativeIdle:
		t.Fatalf("IdleFor() = %s during concurrent access, want non-negative duration", idleFor)
	default:
	}
}
