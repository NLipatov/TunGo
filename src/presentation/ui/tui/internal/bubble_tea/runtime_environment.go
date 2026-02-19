package bubble_tea

import "os"

var (
	getTermEnv = func() string { return os.Getenv("TERM") }
	stdinStat  = func() (os.FileInfo, error) { return os.Stdin.Stat() }
	stdoutStat = func() (os.FileInfo, error) { return os.Stdout.Stat() }
)

func IsInteractiveTerminal() bool {
	if term := getTermEnv(); term == "" || term == "dumb" {
		return false
	}
	stdinInfo, stdinErr := stdinStat()
	if stdinErr != nil {
		return false
	}
	stdoutInfo, stdoutErr := stdoutStat()
	if stdoutErr != nil {
		return false
	}
	return stdinInfo.Mode()&os.ModeCharDevice != 0 && stdoutInfo.Mode()&os.ModeCharDevice != 0
}
