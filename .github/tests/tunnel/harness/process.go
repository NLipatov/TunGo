package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func startChild(logPath string, env []string, args ...string) (*child, error) {
	log, err := os.Create(logPath)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = log, log
	configureChild(cmd)
	if err := cmd.Start(); err != nil {
		return nil, errors.Join(err, log.Close())
	}
	p := &child{cmd: cmd, done: make(chan struct{})}
	go func() {
		p.err = errors.Join(cmd.Wait(), log.Close())
		close(p.done)
	}()
	return p, nil
}

// child owns its log and the single Wait call. Closing done publishes the exit
// result to the runner and controller without reading exec.Cmd concurrently.
type child struct {
	cmd  *exec.Cmd
	done chan struct{}
	err  error
}

func (p *child) stop() error {
	if !p.running() {
		return fmt.Errorf("process exited before shutdown: %v", p.err)
	}
	if err := signalChild(p.cmd.Process); err != nil {
		return err
	}
	return p.wait(45 * time.Second)
}

func (p *child) kill() {
	if p != nil && p.running() {
		_ = p.cmd.Process.Kill()
		_ = p.wait(10 * time.Second)
	}
}

func (p *child) running() bool {
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

func (p *child) wait(timeout time.Duration) error {
	select {
	case <-p.done:
		return p.err
	case <-time.After(timeout):
		return fmt.Errorf("process %d did not exit within %s", p.cmd.Process.Pid, timeout)
	}
}

func powershell(ctx context.Context, script string) (string, error) {
	return command(ctx, "pwsh", "-NoProfile", "-NonInteractive", "-Command",
		"$ErrorActionPreference='Stop'; "+script)
}

func command(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, args[0], args[1:]...).CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(output)), fmt.Errorf("%v: %w: %s", args, err, output)
	}
	return strings.TrimSpace(string(output)), nil
}
