package bubble_tea

import "testing"

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
