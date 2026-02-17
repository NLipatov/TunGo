package bubble_tea

import "os"

func IsInteractiveTerminal() bool {
	if term := os.Getenv("TERM"); term == "" || term == "dumb" {
		return false
	}
	stdinInfo, stdinErr := os.Stdin.Stat()
	if stdinErr != nil {
		return false
	}
	stdoutInfo, stdoutErr := os.Stdout.Stat()
	if stdoutErr != nil {
		return false
	}
	return stdinInfo.Mode()&os.ModeCharDevice != 0 && stdoutInfo.Mode()&os.ModeCharDevice != 0
}
