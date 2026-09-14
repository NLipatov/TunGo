package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func shutdownSignals() []os.Signal { return []os.Signal{os.Interrupt} }

func prepareRunner() error {
	admin, _, _ := syscall.NewLazyDLL("shell32.dll").NewProc("IsUserAnAdmin").Call()
	if admin == 0 {
		return fmt.Errorf("administrator required")
	}
	kernel := syscall.NewLazyDLL("kernel32.dll")
	console, _, _ := kernel.NewProc("GetConsoleWindow").Call()
	if console == 0 {
		if ok, _, err := kernel.NewProc("AllocConsole").Call(); ok == 0 {
			return fmt.Errorf("allocate console for Ctrl+Break: %w", err)
		}
	}
	return nil
}

func configureChild(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

func signalChild(p *os.Process) error {
	// TerminateProcess would skip TunGo's cleanup. Go maps Ctrl+Break to Interrupt.
	ok, _, err := syscall.NewLazyDLL("kernel32.dll").NewProc("GenerateConsoleCtrlEvent").Call(1, uintptr(p.Pid))
	if ok == 0 {
		return fmt.Errorf("send Ctrl+Break: %w", err)
	}
	return nil
}
