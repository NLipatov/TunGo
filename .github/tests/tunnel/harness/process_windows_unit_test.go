//go:build windows

package main

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestClientConsole(t *testing.T) {
	admin, _, _ := syscall.NewLazyDLL("shell32.dll").NewProc("IsUserAnAdmin").Call()
	if admin == 0 {
		t.Skip("the client scenario requires administrator privileges")
	}
	for _, test := range []struct {
		name  string
		flags uint32
	}{
		{"inherited", 0},
		{"detached", 0x00000008}, // DETACHED_PROCESS, as in a noninteractive SSH session.
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestClientConsoleHelper$", "-test.v")
			cmd.Env = append(os.Environ(), "TUNGO_E2E_CONSOLE_TEST=1")
			cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: test.flags}
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("client console and graceful shutdown: %v\n%s", err, output)
			}
		})
	}
}

func TestClientConsoleHelper(t *testing.T) {
	if os.Getenv("TUNGO_E2E_CONSOLE_TEST") != "1" {
		return
	}
	if err := prepareRunner(); err != nil {
		t.Fatal(err)
	}
	// Exercise the real Ctrl+Break path and require the child's cleanup output.
	TestChildGracefulShutdown(t)
}
