package command

import (
	"io"
	"os/exec"
)

type runner struct{}

func New() *runner {
	return &runner{}
}

func (r *runner) CombinedOutput(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

func (r *runner) CombinedOutputWithInput(name string, input io.Reader, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Stdin = input
	return cmd.CombinedOutput()
}

func (r *runner) Output(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

func (r *runner) Run(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}
