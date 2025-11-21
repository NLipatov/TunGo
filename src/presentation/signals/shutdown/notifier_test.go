package shutdown

import (
	"os"
	"testing"
)

func TestShutdownNotifier_NotifyAndStop(t *testing.T) {
	t.Parallel()

	notifier := &Notifier{}
	ch := make(chan os.Signal, 1)

	// This ensures that calls are wired to os/signal without panicking.
	notifier.Notify(ch, os.Interrupt)
	notifier.Stop(ch)
}
