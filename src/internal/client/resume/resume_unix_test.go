//go:build unix

package resume

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

func TestWatchDoesNotSignalWithoutSleepGap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		resumed := Watch(ctx)

		time.Sleep(3 * time.Second)
		synctest.Wait()
		assertNoResume(t, resumed)

		cancel()
		synctest.Wait()
	})
}

func TestWatchCancellationDoesNotSignalResume(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		resumed := Watch(ctx)
		synctest.Wait()

		cancel()
		synctest.Wait()

		assertNoResume(t, resumed)
	})
}

func assertNoResume(t *testing.T, resumed <-chan struct{}) {
	t.Helper()
	select {
	case <-resumed:
		t.Fatal("Watch signaled resume")
	default:
	}
}
