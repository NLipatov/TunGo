package bubble_tea

import "testing"

func TestClearTerminalAfterTUI_PrintsClearSequence(t *testing.T) {
	prevPrint := printToStdout
	t.Cleanup(func() {
		printToStdout = prevPrint
	})

	var got string
	printToStdout = func(s string) { got = s }

	clearTerminalAfterTUI()
	if got != "\x1b[2J\x1b[H" {
		t.Fatalf("expected clear sequence, got %q", got)
	}
}

func TestTerminalCleanup_DefaultPrintHook_IsCallable(t *testing.T) {
	printToStdout("")
}
