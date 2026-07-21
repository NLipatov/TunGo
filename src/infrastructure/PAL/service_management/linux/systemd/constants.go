package systemd

import (
	"os"
	"os/exec"
)

var defaultSystemdConfig = DefaultConfig()

var (
	systemdRuntimeDir = defaultSystemdConfig.RuntimeDir
	systemdUnitPath   = defaultSystemdConfig.UnitPath
	systemdUnitName   = defaultSystemdConfig.UnitName
	tungoBinaryPath   = defaultSystemdConfig.BinaryPath
)

var (
	statPath      = os.Stat
	lstatPath     = os.Lstat
	lookPath      = exec.LookPath
	writeFilePath = os.WriteFile
	readFilePath  = os.ReadFile
	removePath    = os.Remove
)
