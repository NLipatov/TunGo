package systemd

import (
	"os"
	"os/exec"
)

const (
	systemdRuntimeDir = "/run/systemd/system"
	systemdUnitPath   = "/etc/systemd/system/tungo.service"
	systemdUnitName   = "tungo.service"
	tungoBinaryPath   = "/usr/local/bin/tungo"
)

var (
	statPath      = os.Stat
	lstatPath     = os.Lstat
	lookPath      = exec.LookPath
	writeFilePath = os.WriteFile
	readFilePath  = os.ReadFile
	removePath    = os.Remove
	geteuid       = os.Geteuid
)
