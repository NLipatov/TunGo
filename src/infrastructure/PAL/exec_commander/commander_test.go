package exec_commander

import (
	"strings"
	"testing"
)

func TestNewExecCommander(t *testing.T) {
	c := NewExecCommander()
	if c == nil {
		t.Fatal("expected non-nil commander")
	}
	if _, ok := c.(*ExecCommander); !ok {
		t.Fatalf("expected *ExecCommander, got %T", c)
	}
}

func TestExecCommander_Output(t *testing.T) {
	c := &ExecCommander{}
	out, err := c.Output("/bin/sh", "-c", "printf 'hello'")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "hello" {
		t.Fatalf("unexpected output: %q", string(out))
	}
}

func TestExecCommander_CombinedOutput_Error(t *testing.T) {
	c := &ExecCommander{}
	out, err := c.CombinedOutput("/bin/sh", "-c", "printf out; printf err 1>&2; exit 7")
	if err == nil {
		t.Fatal("expected error from non-zero exit")
	}
	if !strings.Contains(string(out), "out") || !strings.Contains(string(out), "err") {
		t.Fatalf("expected combined output to contain both stdout and stderr, got %q", string(out))
	}
}

func TestExecCommander_Run(t *testing.T) {
	c := &ExecCommander{}
	if err := c.Run("/bin/sh", "-c", "exit 0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := c.Run("/bin/sh", "-c", "exit 9"); err == nil {
		t.Fatal("expected error for non-zero exit")
	}
}
