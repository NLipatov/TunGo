package systemd

import (
	"os"
	"os/exec"
)

func available(runtimeDir string) bool {
	if _, err := os.Stat(runtimeDir); err != nil {
		return false
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}
	return true
}
