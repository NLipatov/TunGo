package command

import (
	"strings"
	"testing"
)

func TestRunnerOutput(t *testing.T) {
	c := New()
	out, err := c.Output("/bin/sh", "-c", "printf 'hello'")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "hello" {
		t.Fatalf("unexpected output: %q", string(out))
	}
}

func TestRunnerCombinedOutputError(t *testing.T) {
	c := New()
	out, err := c.CombinedOutput("/bin/sh", "-c", "printf out; printf err 1>&2; exit 7")
	if err == nil {
		t.Fatal("expected error from non-zero exit")
	}
	if !strings.Contains(string(out), "out") || !strings.Contains(string(out), "err") {
		t.Fatalf("expected combined output to contain both stdout and stderr, got %q", string(out))
	}
}

func TestRunnerCombinedOutputWithInput(t *testing.T) {
	c := New()
	out, err := c.CombinedOutputWithInput("/bin/sh", strings.NewReader("hello"), "-c", "read value; printf '%s' \"$value\"")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "hello" {
		t.Fatalf("unexpected output: %q", string(out))
	}
}

func TestRunnerRun(t *testing.T) {
	c := New()
	if err := c.Run("/bin/sh", "-c", "exit 0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := c.Run("/bin/sh", "-c", "exit 9"); err == nil {
		t.Fatal("expected error for non-zero exit")
	}
}
