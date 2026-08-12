package command

import "os/exec"

type Runner interface {
	CombinedOutput(name string, args ...string) ([]byte, error)
	Output(name string, args ...string) ([]byte, error)
	Run(name string, args ...string) error
}

type execRunner struct{}

func New() Runner {
	return &execRunner{}
}

func (r *execRunner) CombinedOutput(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

func (r *execRunner) Output(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

func (r *execRunner) Run(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}
