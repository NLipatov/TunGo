package server

import "testing"

func TestDefaultLoggerFactory(t *testing.T) {
	f := newDefaultLoggerFactory()
	if f == nil {
		t.Fatal("expected non-nil factory")
	}
	logger := f.newLogger()
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}
