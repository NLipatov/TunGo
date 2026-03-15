package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestNewLogger_ReturnsLogger(t *testing.T) {
	l := NewLogger(slog.LevelInfo)
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestNewLogger_WritesToConfiguredOutput(t *testing.T) {
	var buf bytes.Buffer
	prev := SetOutput(&buf)
	t.Cleanup(func() { SetOutput(prev) })

	NewLogger(slog.LevelInfo).Info("hello world")
	if !strings.Contains(buf.String(), "hello world") {
		t.Fatalf("expected log output to contain message, got %q", buf.String())
	}
}

func TestNewStdLogger_WritesToConfiguredOutput(t *testing.T) {
	var buf bytes.Buffer
	prev := SetOutput(&buf)
	t.Cleanup(func() { SetOutput(prev) })

	NewStdLogger(slog.LevelInfo).Print("legacy hello")
	if !strings.Contains(buf.String(), "legacy hello") {
		t.Fatalf("expected legacy log output to contain message, got %q", buf.String())
	}
}
