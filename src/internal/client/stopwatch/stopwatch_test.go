package stopwatch

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestStartsAtCreation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stopwatch := New()
		const elapsed = 1500 * time.Millisecond

		time.Sleep(elapsed)

		if got := stopwatch.Elapsed(); got != elapsed {
			t.Fatalf("Elapsed() = %s, want %s", got, elapsed)
		}
	})
}

func TestReset(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stopwatch := New()
		time.Sleep(1500 * time.Millisecond)

		stopwatch.Reset()

		if got := stopwatch.Elapsed(); got != 0 {
			t.Fatalf("Elapsed() after Reset() = %s, want 0", got)
		}
	})
}

func TestConcurrentResetAndElapsed(t *testing.T) {
	stopwatch := New()
	start := make(chan struct{})
	negativeElapsed := make(chan time.Duration, 1)

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
					stopwatch.Reset()
					continue
				}
				if elapsed := stopwatch.Elapsed(); elapsed < 0 {
					select {
					case negativeElapsed <- elapsed:
					default:
					}
				}
			}
		}()
	}

	close(start)
	wg.Wait()

	select {
	case elapsed := <-negativeElapsed:
		t.Fatalf("Elapsed() = %s during concurrent access, want non-negative duration", elapsed)
	default:
	}
}
