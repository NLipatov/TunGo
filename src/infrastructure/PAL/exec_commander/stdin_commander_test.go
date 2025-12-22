package exec_commander

import (
	"bytes"
	"testing"
)

func TestStdinCommander_Run_Success(t *testing.T) {
	c := NewStdinCommander("hello")

	err := c.Run("/bin/cat")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

func TestStdinCommander_Run_Error(t *testing.T) {
	c := NewStdinCommander("")

	// nonexistent command → guaranteed error
	err := c.Run("/definitely-not-exists")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestStdinCommander_Output(t *testing.T) {
	c := NewStdinCommander("hello\n")

	out, err := c.Output("/bin/cat")
	if err != nil {
		t.Fatalf("Output returned error: %v", err)
	}

	if string(out) != "hello\n" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestStdinCommander_Output_Error(t *testing.T) {
	c := NewStdinCommander("")

	_, err := c.Output("/definitely-not-exists")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestStdinCommander_CombinedOutput(t *testing.T) {
	c := NewStdinCommander("hello\n")

	out, err := c.CombinedOutput("/bin/cat")
	if err != nil {
		t.Fatalf("CombinedOutput returned error: %v", err)
	}

	if string(out) != "hello\n" {
		t.Fatalf("unexpected combined output: %q", out)
	}
}

func TestStdinCommander_CombinedOutput_Error(t *testing.T) {
	c := NewStdinCommander("")

	out, err := c.CombinedOutput("/definitely-not-exists")
	if err == nil {
		t.Fatalf("expected error, got nil (output=%q)", out)
	}
}

func TestStdinCommander_StdinConsumed(t *testing.T) {
	c := NewStdinCommander("once")

	out1, err := c.Output("/bin/cat")
	if err != nil {
		t.Fatalf("first output failed: %v", err)
	}

	out2, err := c.Output("/bin/cat")
	if err != nil {
		t.Fatalf("second output failed: %v", err)
	}

	if string(out1) != "once" {
		t.Fatalf("unexpected first output: %q", out1)
	}
	if len(bytes.TrimSpace(out2)) != 0 {
		t.Fatalf("expected empty stdin on second run, got %q", out2)
	}
}
