package linux

import (
	"os/exec"
	"tungo/infrastructure/PAL"
)

type Commander struct {
}

func NewCommander() PAL.Commander {
	return &Commander{}
}

func (r *Commander) CombinedOutput(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

func (r *Commander) Output(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}
