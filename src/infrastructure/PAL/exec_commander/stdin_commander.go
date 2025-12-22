package exec_commander

import (
	"bytes"
	"os/exec"
)

// StdinCommander executes commands with predefined stdin.
// Intended for stdin-driven utilities like scutil on macOS.
type StdinCommander struct {
	stdin *bytes.Buffer
}

// NewStdinCommander creates a commander with given stdin content.
func NewStdinCommander(stdin string) *StdinCommander {
	return &StdinCommander{
		stdin: bytes.NewBufferString(stdin),
	}
}

func (c *StdinCommander) Run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = c.stdin
	return cmd.Run()
}

func (c *StdinCommander) Output(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Stdin = c.stdin
	return cmd.Output()
}

func (c *StdinCommander) CombinedOutput(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Stdin = c.stdin
	return cmd.CombinedOutput()
}
