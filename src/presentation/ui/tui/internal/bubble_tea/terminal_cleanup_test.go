package bubble_tea

import "testing"

func TestClearTerminalAfterTUI_NonInteractive_NoOutput(t *testing.T) {
	prevInteractive := isInteractiveTerminal
	prevPrint := printToStdout
	t.Cleanup(func() {
		isInteractiveTerminal = prevInteractive
		printToStdout = prevPrint
	})

	called := false
	isInteractiveTerminal = func() bool { return false }
	printToStdout = func(string) { called = true }

	clearTerminalAfterTUI()
	if called {
		t.Fatal("expected no terminal clear output for non-interactive terminal")
	}
}

func TestClearTerminalAfterTUI_Interactive_PrintsClearSequence(t *testing.T) {
	prevInteractive := isInteractiveTerminal
	prevPrint := printToStdout
	t.Cleanup(func() {
		isInteractiveTerminal = prevInteractive
		printToStdout = prevPrint
	})

	var got string
	isInteractiveTerminal = func() bool { return true }
	printToStdout = func(s string) { got = s }

	clearTerminalAfterTUI()
	if got != "\x1b[2J\x1b[H" {
		t.Fatalf("expected clear sequence, got %q", got)
	}
}

func TestTerminalCleanup_DefaultPrintHook_IsCallable(t *testing.T) {
	printToStdout("")
}
