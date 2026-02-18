package bubble_tea

import (
	"log"
	"strings"
	"testing"
)

func TestRuntimeLogBuffer_Tail(t *testing.T) {
	b := NewRuntimeLogBuffer(3)
	_, _ = b.Write([]byte("one\n"))
	_, _ = b.Write([]byte("two\nthree\nfour\n"))

	got := b.Tail(2)
	if len(got) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(got))
	}
	if got[0] != "three" || got[1] != "four" {
		t.Fatalf("unexpected tail: %v", got)
	}
}

func TestGlobalRuntimeLogCapture_CapturesStandardLogger(t *testing.T) {
	DisableGlobalRuntimeLogCapture()
	t.Cleanup(DisableGlobalRuntimeLogCapture)

	EnableGlobalRuntimeLogCapture(8)
	log.Printf("runtime test line")

	feed := GlobalRuntimeLogFeed()
	if feed == nil {
		t.Fatal("expected global runtime log feed to be initialized")
	}

	lines := feed.Tail(8)
	found := false
	for _, line := range lines {
		if strings.Contains(line, "runtime test line") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected log line in runtime feed, got %v", lines)
	}
}
