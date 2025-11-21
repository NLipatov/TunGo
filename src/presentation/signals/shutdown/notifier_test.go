package shutdown

import (
	"os"
	"testing"
)

func TestNewNotifier(t *testing.T) {
	t.Parallel()

	n := NewNotifier()
	if n == nil {
		t.Fatalf("NewNotifier must not return nil")
	}
}

func TestNotifier_NotifyAndStop(t *testing.T) {
	t.Parallel()

	notifier := NewNotifier()
	ch := make(chan os.Signal, 1)

	// This ensures that calls are wired to os/signal without panicking.
	notifier.Notify(ch, os.Interrupt)
	notifier.Stop(ch)
}
