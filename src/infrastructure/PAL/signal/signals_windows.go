//go:build windows

package signal

import (
	"os"
	"syscall"
)

var ShutdownSignals = [...]os.Signal{
	os.Interrupt,    // Ctrl-C
	syscall.SIGTERM, // console close / Task Manager stop
}
