package logging

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestNewLogLogger_ReturnsLogger(t *testing.T) {
	l := NewLogLogger()
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestLogLogger_Printf_WritesToStdLog(t *testing.T) {
	origOutput := log.Writer()
	origFlags := log.Flags()
	origPrefix := log.Prefix()
	defer func() {
		log.SetOutput(origOutput)
		log.SetFlags(origFlags)
		log.SetPrefix(origPrefix)
	}()

	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetFlags(0)
	log.SetPrefix("")

	LogLogger{}.Printf("hello %s", "world")
	if !strings.Contains(buf.String(), "hello world") {
		t.Fatalf("expected log output to contain formatted message, got %q", buf.String())
	}
}
