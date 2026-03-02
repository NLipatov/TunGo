package tui

import "testing"

func TestShowFatalError_ForwardsMessageToRunner(t *testing.T) {
	prev := runFatalErrorProgram
	t.Cleanup(func() { runFatalErrorProgram = prev })

	var got string
	runFatalErrorProgram = func(message string) {
		got = message
	}

	ShowFatalError("fatal boom")
	if got != "fatal boom" {
		t.Fatalf("expected message to be forwarded, got %q", got)
	}
}
