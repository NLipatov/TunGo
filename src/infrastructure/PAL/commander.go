package PAL

import "os/exec"

// Commander abstracts platform-specific command execution (e.g., via exec.Command).
type Commander interface {
	CombinedOutput(name string, args ...string) ([]byte, error)
	Output(name string, args ...string) ([]byte, error)
	Run(name string, args ...string) error
}

type ExecCommander struct {
}

func NewExecCommander() Commander {
	return &ExecCommander{}
}

func (r *ExecCommander) CombinedOutput(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

func (r *ExecCommander) Output(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

func (r *ExecCommander) Run(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}
