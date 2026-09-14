//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func shutdownSignals() []os.Signal { return []os.Signal{os.Interrupt, syscall.SIGTERM} }

func prepareRunner() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("root required")
	}
	return nil
}

func configureChild(_ *exec.Cmd) {}

func signalChild(p *os.Process) error { return p.Signal(syscall.SIGTERM) }
