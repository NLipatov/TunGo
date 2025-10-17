//go:build !windows

package signal

import (
	"os"
	"syscall"
	"testing"
)

func TestShutdownSignals_Unix_ExactSetAndOrder(t *testing.T) {
	t.Parallel()

	got := NewDefaultProvider().ShutdownSignals()
	want := []os.Signal{
		os.Interrupt,    // SIGINT
		syscall.SIGTERM, // TERM
		syscall.SIGHUP,  // HUP
	}

	if len(got) != len(want) {
		t.Fatalf("unexpected length: got %d, want %d; got=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected element at %d: got %v, want %v; full got=%v", i, got[i], want[i], got)
		}
	}
}
