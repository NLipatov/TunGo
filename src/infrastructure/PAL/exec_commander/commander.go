package exec_commander

import "os/exec"

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
