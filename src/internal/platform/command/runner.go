package command

import (
	"context"
	"io"
	"os/exec"
	"time"
)

type runner struct{}

func New() *runner {
	return &runner{}
}

func (r *runner) CombinedOutput(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

func (r *runner) CombinedOutputContext(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	// Also bound pipe draining if a child inherited stdout or stderr.
	cmd.WaitDelay = time.Second
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return output, ctx.Err()
	}
	return output, err
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
