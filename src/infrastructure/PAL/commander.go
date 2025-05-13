package PAL

// Commander abstracts platform-specific command execution (e.g., via exec.Command).
type Commander interface {
	CombinedOutput(name string, args ...string) ([]byte, error)
	Output(name string, args ...string) ([]byte, error)
}
