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

func TestRuntimeLogBuffer_DefaultCapacityAndPartialLine(t *testing.T) {
	b := NewRuntimeLogBuffer(0)
	_, _ = b.Write([]byte("partial"))
	if tail := b.Tail(10); len(tail) != 0 {
		t.Fatalf("expected no complete lines yet, got %v", tail)
	}

	_, _ = b.Write([]byte(" line\n"))
	tail := b.Tail(10)
	if len(tail) != 1 || tail[0] != "partial line" {
		t.Fatalf("unexpected tail after newline flush: %v", tail)
	}
}

func TestRuntimeLogBuffer_TailEdgeCases(t *testing.T) {
	b := NewRuntimeLogBuffer(2)
	_, _ = b.Write([]byte("\n")) // empty line must be ignored
	if got := b.Tail(0); got != nil {
		t.Fatalf("expected nil for non-positive limit, got %v", got)
	}
	if got := b.Tail(-1); got != nil {
		t.Fatalf("expected nil for non-positive limit, got %v", got)
	}
}

func TestRedirectStandardLoggerToBuffer_NilBufferNoop(t *testing.T) {
	restore := RedirectStandardLoggerToBuffer(nil)
	if restore == nil {
		t.Fatal("expected non-nil restore func")
	}
	restore()
}

func TestRedirectStandardLoggerToBuffer_RestoresWriter(t *testing.T) {
	b := NewRuntimeLogBuffer(8)
	prevWriter := log.Writer()
	restore := RedirectStandardLoggerToBuffer(b)
	log.Printf("redirected line")
	restore()
	if log.Writer() != prevWriter {
		t.Fatal("expected logger writer to be restored")
	}
	if lines := b.Tail(8); len(lines) == 0 {
		t.Fatal("expected redirected log line in buffer")
	}
}

func TestRuntimeLogBuffer_TailInto_EmptyDst(t *testing.T) {
	b := NewRuntimeLogBuffer(4)
	_, _ = b.Write([]byte("one\ntwo\n"))

	dst := make([]string, 0)
	n := b.TailInto(dst, 10)
	if n != 0 {
		t.Fatalf("expected 0 for empty dst, got %d", n)
	}
}

func TestRuntimeLogBuffer_TailInto_LimitGreaterThanDst(t *testing.T) {
	b := NewRuntimeLogBuffer(8)
	_, _ = b.Write([]byte("one\ntwo\nthree\nfour\nfive\n"))

	dst := make([]string, 2)
	n := b.TailInto(dst, 100) // limit > len(dst) => clamp to len(dst)
	if n != 2 {
		t.Fatalf("expected 2 (clamped to dst length), got %d", n)
	}
	if dst[0] != "four" || dst[1] != "five" {
		t.Fatalf("expected last 2 lines, got %v", dst[:n])
	}
}

func TestEnableGlobalRuntimeLogCapture_IdempotentAndDisableSafe(t *testing.T) {
	DisableGlobalRuntimeLogCapture()
	DisableGlobalRuntimeLogCapture() // safe when already disabled

	EnableGlobalRuntimeLogCapture(4)
	first := GlobalRuntimeLogFeed()
	if first == nil {
		t.Fatal("expected initialized feed")
	}
	EnableGlobalRuntimeLogCapture(99) // should not replace existing buffer
	second := GlobalRuntimeLogFeed()
	if first != second {
		t.Fatal("expected global capture enable to be idempotent")
	}

	DisableGlobalRuntimeLogCapture()
	if GlobalRuntimeLogFeed() != nil {
		t.Fatal("expected nil feed after disable")
	}
}
