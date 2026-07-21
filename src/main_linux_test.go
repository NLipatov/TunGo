//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

const (
	runMainClientEnv      = "TUNGO_TEST_RUN_MAIN_CLIENT"
	defaultConfigTestLock = "/tmp/tungo-default-configuration-test.lock"
)

func acquireDefaultConfigTestLock(t *testing.T) {
	t.Helper()
	lock, err := os.OpenFile(defaultConfigTestLock, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatalf("open default configuration test lock: %v", err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		_ = lock.Close()
		t.Fatalf("acquire default configuration test lock: %v", err)
	}
	t.Cleanup(func() {
		_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		_ = lock.Close()
	})
}

func requireCleanDefaultConfigDirectory(t *testing.T) string {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("default configuration paths require root access")
	}
	acquireDefaultConfigTestLock(t)
	directory := filepath.Join(string(filepath.Separator), "etc", "tungo")
	if _, err := os.Stat(directory); err == nil {
		t.Skip("preserving existing /etc/tungo")
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat default configuration directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(filepath.Join(directory, "client_configuration.json.1"))
		_ = os.Remove(filepath.Join(directory, "client_configuration.json"))
		_ = os.Remove(filepath.Join(directory, "server_configuration.json"))
		_ = os.Remove(filepath.Join(directory, "crash.log"))
		_ = os.Remove(directory)
	})
	return directory
}

func TestMain_ClientWithoutConfiguration(t *testing.T) {
	if os.Getenv(runMainClientEnv) == "1" {
		os.Args = []string{"tungo", "c"}
		main()
		return
	}
	requireCleanDefaultConfigDirectory(t)

	command := exec.Command(os.Args[0], "-test.run=^TestMain_ClientWithoutConfiguration$")
	command.Env = append(os.Environ(), runMainClientEnv+"=1")
	output, err := command.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("main process error = %v; output: %s", err, output)
	}
}

func TestRunCLI_GenerateServerConfigurationOnCleanSystem(t *testing.T) {
	directory := requireCleanDefaultConfigDirectory(t)
	t.Setenv("ServerIP", "127.0.0.1")
	setCommandLine(t, "s", "gen")

	var runErr error
	output := captureStdout(t, func() {
		runErr = runCLI(context.Background())
	})
	if runErr != nil {
		t.Fatalf("runCLI() error = %v", runErr)
	}
	if !strings.Contains(output, `"ClientID": 1`) {
		t.Fatalf("generated configuration output = %q", output)
	}
	if _, err := os.Stat(filepath.Join(directory, "client_configuration.json.1")); err != nil {
		t.Fatalf("stat generated client configuration: %v", err)
	}
}
