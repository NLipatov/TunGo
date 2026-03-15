package logging

import (
	"bytes"
	"io"
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

func TestSetOutput_NilRestoresStderrAndCurrentOutput(t *testing.T) {
	var buf bytes.Buffer
	prev := SetOutput(&buf)
	t.Cleanup(func() { SetOutput(prev) })

	if CurrentOutput() != &buf {
		t.Fatalf("expected CurrentOutput to return configured writer")
	}

	gotPrev := SetOutput(nil)
	if gotPrev != &buf {
		t.Fatalf("expected SetOutput(nil) to return previous writer")
	}
	if CurrentOutput() == nil {
		t.Fatal("expected non-nil current output after nil reset")
	}
}

func TestWriter_ForwardsToCurrentOutput(t *testing.T) {
	var buf bytes.Buffer
	prev := SetOutput(&buf)
	t.Cleanup(func() { SetOutput(prev) })

	n, err := io.WriteString(Writer(), "forwarded")
	if err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
	if n != len("forwarded") {
		t.Fatalf("unexpected bytes written: %d", n)
	}
	if buf.String() != "forwarded" {
		t.Fatalf("expected forwarded content, got %q", buf.String())
	}
}
