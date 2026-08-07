//go:build !windows

package signal

import (
	"os"
	"syscall"
)

var ShutdownSignals = [...]os.Signal{
	os.Interrupt,    // SIGINT (Ctrl-C)
	syscall.SIGTERM, // systemd/docker stop
	syscall.SIGHUP,  // terminal closed / reload
}
